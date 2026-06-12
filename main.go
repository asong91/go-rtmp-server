package main

import (
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"log"
	"log/slog"
	"math"
	"net"
	"os"
	"time"
)

const S1_C1_SIZE = 1536

var chunkSize = 4096

// var conn net.Conn
type Client struct {
	conn                 net.Conn
	chunkSize            int
	previousChunkHeaders map[uint32]ChunkMessageHeader

	// RTMP Stream Data
	AudioSeqHeader      *Chunk
	VideoSeqHeader      *Chunk
	AudTimestamp        uint32
	VidTimestamp        uint32
	file                *os.File
	PrevFLVPacket       FLVPacket
	AudPacket           int
	VidPacket           int
	FlvOnMetaDataPacket FLVPacket
	durationIndex       int
}

func main() {
	slog.SetLogLoggerLevel(slog.LevelDebug)
	println("Welcome to the TCP server!")
	listen()
}

func listen() {
	listener, err := net.Listen("tcp", "localhost:8080")
	if err != nil {
		panic(err)
	}
	defer listener.Close()

	for {
		c, err := listener.Accept()
		if err != nil {
			log.Fatal("Error accepting connection:", err)
		}
		client := Client{
			conn:                 c,
			chunkSize:            128,
			previousChunkHeaders: make(map[uint32]ChunkMessageHeader)}
		go client.rtmp()
	}
}

func (cl *Client) rtmp() {
	println("Client connected:", cl.conn.RemoteAddr().String())
	_, err := cl.initializeHandshake()
	if err != nil {
		log.Fatal("Handshake failed:", err)
	}
	fmt.Println("---HANDSHAKE COMPLETE!---")

	cl.readReq()
	// defer conn.Close()
}

func (cl *Client) readReq() {
ReadLoop:
	for {
		chunk := cl.ReadChunk()
		// chunk.Print()
		// chunk.Print() // Waits for the user to press the Enter key
		// fmt.Scanln()
		switch chunk.ChunkHeader.ChunkMessageHeader.MessageTypeId {
		case 0x01:
			chunkSize := chunk.readChunkSizeMessage()
			cl.chunkSize = chunkSize
			fmt.Printf("CLIENT SET CHUNK SIZE: %d\n", chunkSize)
		case 0x08: // Audio
			cl.setTimestamp(chunk)
			packetType := chunk.Payload[1]
			msgStreamId := chunk.ChunkHeader.ChunkMessageHeader.MessageStreamId
			if packetType == 0 {
				// AAC Sequence Header
				cl.storeSeqHeader(msgStreamId, true, chunk)
			} else if packetType == 1 {
				// AAC Raw
				cl.writeToFile(chunk)
			}
			cl.AudPacket++
			// parseAudioPacket(chunk.Payload)
		case 0x09: // Video
			//chunk.ChunkHeader.Print()
			// fmt.Printf("video: fmt=%d ts=%d msgLen=%d\n",
			// 	chunk.ChunkHeader.ChunkBasicHeader.Fmt,
			// 	chunk.ChunkHeader.ChunkMessageHeader.Timestamp,
			// 	chunk.ChunkHeader.ChunkMessageHeader.MessageLength)
			cl.setTimestamp(chunk)
			//fmt.Printf("video fmt=%d rawDelta=%d vidTs=%d\n",
			//	chunk.ChunkHeader.ChunkBasicHeader.Fmt,
			//	chunk.ChunkHeader.ChunkMessageHeader.Timestamp,
			//	streamIdMap[1].VidTimestamp)
			packetType := chunk.Payload[1]
			msgStreamId := chunk.ChunkHeader.ChunkMessageHeader.MessageStreamId
			if packetType == 0 {
				// AVC Sequence Header
				cl.storeSeqHeader(msgStreamId, false, chunk)

			} else if packetType == 1 {
				// AVC NALU
				cl.writeToFile(chunk)
			}
			cl.VidPacket++
		case 0x12: // 18 Data Message
			//fmt.Printf("data:\n %s\n", parseAMF0(chunk.Payload))
			// fmt.Scanln()
			fmt.Printf("%s", parseAMF0(chunk.Payload))
			cl.setOnMetaDataMessage(chunk.Payload)
		case 0x14: // 20 Command Message
			cmd, _ := parseAMF0Command(chunk.Payload)
			// cmd.Print()
			switch cmd.Name {
			case "connect":
				//cmd.Print()
				windowAckMsg := genProtocolControlMessage(5, 2_500_000)
				windowAckMsg.Send(cl.conn)
				peerBandMsg := genProtocolControlMessage(6, 2_500_000)
				peerBandMsg.Send(cl.conn)
				beginMsg := genUserControlMessage(0, 0, 0)
				beginMsg.Send(cl.conn)
				chunkSize := genProtocolControlMessage(1, chunkSize)
				chunkSize.Send(cl.conn)
				sendAMF0CommandMessage(genConnectSuccess(1.0), cl.conn)
			case "releaseStream":
				sendAMF0CommandMessage(SerializeAMF0(
					"_result",
					cmd.SeqNum,
					nil,
				), cl.conn)
			case "FCPublish":
				sendAMF0CommandMessage(SerializeAMF0(
					"onFCPublish",
					0.0,
					nil,
				), cl.conn)
			case "FCUnpublish":
				fmt.Printf("FCUNPBLISH TODO: FIGURE OUT WHAT TO DO HERE\n")
				//chunk.Print()
				//cmd.Print()
			case "deleteStream":
				// msgStreamId := chunk.ChunkHseader.ChunkMessageHeader.MessageStreamId
				durationSeconds := float64(cl.AudTimestamp) / 1000.0
				bits := math.Float64bits(durationSeconds)
				var buf [8]byte
				binary.BigEndian.PutUint64(buf[:], bits)
				cl.file.WriteAt(buf[:], int64(cl.durationIndex))
				cl.file.Close()
				fmt.Printf("CLOSING TIMESTAMPS AUD: %d\nVID: %d\n %d %d", cl.AudTimestamp, cl.VidTimestamp, cl.AudPacket, cl.VidPacket)
				break ReadLoop
			case "createStream":
				sendAMF0CommandMessage(SerializeAMF0(
					"_result",
					cmd.SeqNum,
					nil,
					1.0), cl.conn)
			case "publish":
				streamBeginMsg := genUserControlMessage(0, 1, 0)
				streamBeginMsg.Send(cl.conn)
				sendAMF0CommandMessage(SerializeAMF0(
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
		}
	}

	fmt.Printf("Stream over...")

}

func (cl *Client) initializeHandshake() (int, error) {
	fmt.Printf("Starting RTMP handshake with client %s\n", cl.conn.RemoteAddr().String())

	// Read the first byte (C0) from the client
	c0_buf := cl.readBytes(1)
	if c0_buf == nil {
		log.Fatal("Failed to read C0 byte from client")
		return 0, fmt.Errorf("Failed to read C0 byte from client")
	}
	// If the first byte is not 0x03, it's an invalid handshake
	if c0_buf[0] != 0x03 {
		log.Fatal("Invalid handshake byte received:", c0_buf[0])
		return 0, fmt.Errorf("Invalid handshake byte received: %d", c0_buf[0])
	}

	c1_buf := cl.readBytes(S1_C1_SIZE) // Junk
	if c1_buf == nil {
		log.Fatal("Failed to read C1 byte from client")
		return 0, fmt.Errorf("Failed to read C1 byte from client")
	}

	cl.sendBytes([]byte{0x03}) // Send S0
	cl.sendBytes(makeS1())     // Send S1
	cl.sendBytes(makeS1())     // Send S2

	c2_buf := cl.readBytes(S1_C1_SIZE) // Junk
	if c2_buf == nil {
		log.Fatal("Failed to read C2 byte from client")
		return 0, fmt.Errorf("Failed to read C2 byte from client")
	}
	return 1, nil
}

func (cl *Client) readBytes(numBytes int) []byte {
	buf := make([]byte, numBytes)
	_, err := cl.conn.Read(buf)
	if err != nil {
		log.Fatal("Error reading from connection:", err)
		return nil
	}

	return buf
}

func (cl *Client) sendBytes(data []byte) {
	fmt.Printf("Sending bytes %d \n", len(data))
	n, err := cl.conn.Write(data)
	fmt.Printf("Sent %d bytes to client\n", n)
	if err != nil {
		log.Fatal("Error writing to connection:", err)
	}
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

func (cl *Client) storeSeqHeader(msgStreamId uint32, isAudio bool, chunk Chunk) {
	// stream := &cl.stream
	if isAudio {
		cl.AudioSeqHeader = &chunk
		//fmt.Println("audio chunk ")
		//chunk.Print()
	} else {
		cl.VideoSeqHeader = &chunk
		//fmt.Println("video chunk ")
		//chunk.Print()
	}
	t := time.Now()
	fileName := "output_vid" + t.Format(time.TimeOnly) + ".flv"
	_, err := os.Stat(fileName)
	if cl.HasBoth() && err != nil {
		file, err := cl.createFile(fileName)
		if err == nil {
			// streamIdMap[msgStreamId].FileName = fileName
			cl.file = file
			cl.writeToFile(*cl.AudioSeqHeader)
			cl.writeToFile(*cl.VideoSeqHeader)
		}
	}
}

func (h *Client) HasBoth() bool {
	return h.AudioSeqHeader != nil && h.VideoSeqHeader != nil
}

func (cl *Client) setTimestamp(c Chunk) {
	if c.isAudioData() {
		if c.ChunkHeader.ChunkBasicHeader.Fmt == 0 {
			cl.AudTimestamp = c.ChunkHeader.ChunkMessageHeader.Timestamp
		} else {
			cl.AudTimestamp += c.ChunkHeader.ChunkMessageHeader.Timestamp
		}
	} else if c.isVideoData() {
		// fmt.Printf("fmt=%d rawDelta=%d vidTs_before=%d vidTs_after=%d\n",
		// 	c.ChunkHeader.ChunkBasicHeader.Fmt,
		// 	c.ChunkHeader.ChunkMessageHeader.Timestamp,
		// 	stream.VidTimestamp,
		// 	stream.VidTimestamp+c.ChunkHeader.ChunkMessageHeader.Timestamp)
		if c.ChunkHeader.ChunkBasicHeader.Fmt == 0 {
			cl.VidTimestamp = c.ChunkHeader.ChunkMessageHeader.Timestamp
		} else {
			cl.VidTimestamp += c.ChunkHeader.ChunkMessageHeader.Timestamp
		}
	}
}
