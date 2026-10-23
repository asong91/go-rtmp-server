package main

import (
	"crypto/rand"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"runtime/debug"
	"time"
)

func (cl *Client) rtmp() {
	addr := cl.conn.RemoteAddr().String()
	println("Client connected:", addr)

	// Deferred calls run last-in, first-out: closeOutput, then recover, then conn.Close.
	// They still run if readReq panics, and the recover keeps one bad client from crashing the server.
	defer cl.conn.Close()
	defer func() {
		if r := recover(); r != nil {
			fmt.Printf("Recovered from panic with client %s: %v\n%s", addr, r, debug.Stack())
		}
	}()
	defer cl.closeOutput()

	if err := cl.initializeHandshake(); err != nil {
		fmt.Println("Handshake failed:", err)
		return
	}
	fmt.Println("---HANDSHAKE COMPLETE!---")

	if err := cl.readReq(); err != nil {
		if errors.Is(err, io.EOF) {
			fmt.Println("Client disconnected")
		} else {
			fmt.Println("Error:", err)
		}
	}
	fmt.Printf("Stream over...")
}

func (cl *Client) closeOutput() {
	if cl.Output == nil {
		return
	}
	if err := cl.Output.closeStream(); err != nil {
		fmt.Println("Error closing output:", err)
	}
	cl.Output = nil
}

func (cl *Client) readReq() error {
	for {
		chunk, err := cl.ReadChunk()
		if err != nil {
			return err
		}
		cl.setTimestamp(chunk)
		// chunk.Print()
		// chunk.Print() // Waits for the user to press the Enter key
		// fmt.Scanln()
		switch chunk.ChunkHeader.ChunkMessageHeader.MessageTypeId {
		case 0x01:
			chunkSize, err := chunk.readChunkSizeMessage()
			if err != nil {
				return err
			}
			cl.chunkSize = chunkSize
			fmt.Printf("CLIENT SET CHUNK SIZE: %d\n", chunkSize)
		case 0x08: // Audio
			// Need the sound format byte and the AAC packet type byte
			if len(chunk.Payload) < 2 {
				fmt.Println("Skipping audio message that is too short")
				continue
			}
			packetType := chunk.Payload[1]
			msgStreamId := chunk.ChunkHeader.ChunkMessageHeader.MessageStreamId
			if packetType == 0 {
				// AAC Sequence Header
				cl.storeSeqHeader(msgStreamId, true, chunk)
			} else if packetType == 1 {
				// AAC Raw
				cl.writeMedia(chunk)
			}
			cl.AudPacket++
			// parseAudioPacket(chunk.Payload)
		case 0x09: // Video
			//chunk.ChunkHeader.Print()
			// fmt.Printf("video: fmt=%d ts=%d msgLen=%d\n",
			// 	chunk.ChunkHeader.ChunkBasicHeader.Fmt,
			// 	chunk.ChunkHeader.ChunkMessageHeader.Timestamp,
			// 	chunk.ChunkHeader.ChunkMessageHeader.MessageLength)
			//fmt.Printf("video fmt=%d rawDelta=%d vidTs=%d\n",
			//	chunk.ChunkHeader.ChunkBasicHeader.Fmt,
			//	chunk.ChunkHeader.ChunkMessageHeader.Timestamp,
			//	streamIdMap[1].VidTimestamp)
			// Need the frame/codec byte and the AVC packet type byte
			if len(chunk.Payload) < 2 {
				fmt.Println("Skipping video message that is too short")
				continue
			}
			packetType := chunk.Payload[1]
			msgStreamId := chunk.ChunkHeader.ChunkMessageHeader.MessageStreamId
			if packetType == 0 {
				// AVC Sequence Header
				cl.storeSeqHeader(msgStreamId, false, chunk)

			} else if packetType == 1 {
				// AVC NALU
				cl.writeMedia(chunk)
			}
			cl.VidPacket++
		case 0x12: // 18 Data Message
			//fmt.Printf("data:\n %s\n", parseAMF0(chunk.Payload))
			// fmt.Scanln()
			// fmt.Printf("%s", parseAMF0(chunk.Payload))
			if err := cl.setOnMetaDataMessage(chunk.Payload); err != nil {
				fmt.Println("Ignoring bad metadata:", err)
			}
		case 0x14: // 20 Command Message
			// chunk.Print()
			// fmt.Printf("%s", parseAMF0(chunk.Payload))
			cmd, err := parseAMF0Command(chunk.Payload)
			if err != nil {
				return fmt.Errorf("parsing command: %w", err)
			}
			// cmd.Print()
			switch cmd.Name {
			case "connect":
				fmt.Printf("connecting")
				//cmd.Print()
				err = cl.sendConnectResponse()
			case "releaseStream":
				err = sendAMF0CommandMessage(SerializeAMF0(
					"_result",
					cmd.SeqNum,
					nil,
				), cl.conn)
			case "FCPublish":
				err = sendAMF0CommandMessage(SerializeAMF0(
					"onFCPublish",
					0.0,
					nil,
				), cl.conn)
			case "FCUnpublish":
				fmt.Printf("FCUNPBLISH TODO: FIGURE OUT WHAT TO DO HERE\n")
				//chunk.Print()
				//cmd.Print()
			case "deleteStream":
				fmt.Printf("CLOSING TIMESTAMPS AUD: %d\nVID: %d\n %d %d", cl.AudTimestamp, cl.VidTimestamp, cl.AudPacket, cl.VidPacket)
				return nil
			case "createStream":
				err = sendAMF0CommandMessage(SerializeAMF0(
					"_result",
					cmd.SeqNum,
					nil,
					1.0), cl.conn)
			case "publish":
				streamBeginMsg := genUserControlMessage(0, 1, 0)
				if err = streamBeginMsg.Send(cl.conn); err != nil {
					break
				}
				err = sendAMF0CommandMessage(SerializeAMF0(
					"onStatus",
					0.0,
					nil,
					[]AMF0Field{
						{"level", "status"},
						{"code", "NetStream.Publish.Start"},
						{"description", "Stream is now published."},
					},
				), cl.conn)
				// entry, exists := cl.stream
				// if cl.stream == nil {
				// 	// create entry in map
				// 	entry = &RTMPStream{}
				// 	streamIdMap[chunk.ChunkHeader.ChunkMessageHeader.MessageStreamId] = entry
				// }
			}
			if err != nil {
				return fmt.Errorf("responding to %s: %w", cmd.Name, err)
			}
		}
	}
}

func (cl *Client) sendConnectResponse() error {
	windowAckMsg := genProtocolControlMessage(5, 2_500_000)
	if err := windowAckMsg.Send(cl.conn); err != nil {
		return err
	}
	peerBandMsg := genProtocolControlMessage(6, 2_500_000)
	if err := peerBandMsg.Send(cl.conn); err != nil {
		return err
	}
	beginMsg := genUserControlMessage(0, 0, 0)
	if err := beginMsg.Send(cl.conn); err != nil {
		return err
	}
	chunkSizeMsg := genProtocolControlMessage(1, chunkSize)
	if err := chunkSizeMsg.Send(cl.conn); err != nil {
		return err
	}
	return sendAMF0CommandMessage(genConnectSuccess(1.0), cl.conn)
}

// io.ReadFull keeps reading until buf is full; a single conn.Read can return fewer bytes
func readNumBytes(r io.Reader, numBytes int) ([]byte, error) {
	buf := make([]byte, numBytes)
	if _, err := io.ReadFull(r, buf); err != nil {
		return nil, err
	}
	return buf, nil
}

func (cl *Client) sendBytes(data []byte) error {
	_, err := cl.conn.Write(data)
	return err
}

func (cl *Client) initializeHandshake() error {
	fmt.Printf("Starting RTMP handshake with client %s\n", cl.conn.RemoteAddr().String())
	// Read the first byte (C0) from the client
	c0Buf, err := readNumBytes(cl.conn, 1)
	if err != nil {
		return fmt.Errorf("reading C0: %w", err)
	}
	// If the first byte is not 0x03, it's an invalid handshake
	if c0Buf[0] != 0x03 {
		return fmt.Errorf("invalid handshake byte received: %d", c0Buf[0])
	}

	c1Buf, err := readNumBytes(cl.conn, S1_C1_SIZE)
	if err != nil {
		return fmt.Errorf("reading C1: %w", err)
	}

	if err := cl.sendBytes([]byte{0x03}); err != nil {
		return fmt.Errorf("sending S0: %w", err)
	}
	if err := cl.sendBytes(makeS1()); err != nil {
		return fmt.Errorf("sending S1: %w", err)
	}
	// S2 echoes C1 back to the client
	if err := cl.sendBytes(c1Buf); err != nil {
		return fmt.Errorf("sending S2: %w", err)
	}

	if _, err := readNumBytes(cl.conn, S1_C1_SIZE); err != nil {
		return fmt.Errorf("reading C2: %w", err)
	}
	return nil
}

func makeS1() []byte {
	s1 := make([]byte, 1536)

	// first 4 bytes: timestamp
	binary.BigEndian.PutUint32(s1[0:4], uint32(time.Now().UnixMilli()))

	// next 4 bytes: zeros
	// (already zero from make())

	// remaining 1528 bytes: random
	rand.Read(s1[8:])

	return s1
}

func byteSliceToInt(data []byte) uint32 {
	var res uint32
	for _, v := range data {
		res = (res << 8) | uint32(v)
	}

	return res
}

func (h *Client) HasBoth() bool {
	return h.AudioSeqHeader != nil && h.VideoSeqHeader != nil
}

// The only place that looks at OutputType
func (cl *Client) createStreamOutput() StreamOutput {
	switch cl.StreamConfig.OutputType {
	case "file":
		fileName := cl.StreamConfig.OutputDest
		if fileName == "" {
			fileName = "output_vid" + time.Now().Format("2006-01-02_15-04-05") + ".flv"
		}
		return &FileOutput{fileName: fileName, durationIndex: cl.durationIndex}
	case "hls":
		return &HlsOutput{outDir: "streams/" + cl.StreamConfig.OutputDest}
	}
	return nil
}

func (cl *Client) storeSeqHeader(msgStreamId uint32, isAudio bool, chunk Chunk) {
	if isAudio {
		cl.AudioSeqHeader = &chunk
	} else {
		cl.VideoSeqHeader = &chunk
	}
	if !cl.HasBoth() || cl.Output != nil {
		return
	}

	output := cl.createStreamOutput()
	if output == nil {
		fmt.Println("Unknown output type:", cl.StreamConfig.OutputType)
		return
	}
	if err := output.createOutput(cl.FlvOnMetaDataPacket, *cl.AudioSeqHeader, *cl.VideoSeqHeader); err != nil {
		fmt.Println("Error creating output:", err)
		return
	}
	// Only store once the output is actually running
	cl.Output = output
}

func (cl *Client) writeMedia(chunk Chunk) {
	if cl.Output == nil {
		return
	}
	timestamp := cl.VidTimestamp
	if chunk.isAudioData() {
		timestamp = cl.AudTimestamp
	}
	if err := cl.Output.writeMedia(chunk, timestamp); err != nil {
		fmt.Println("Error writing media:", err)
	}
}
