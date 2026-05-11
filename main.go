package main

import (
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"log"
	"log/slog"
	"net"
	"os"
	"time"
)

const S1_C1_SIZE = 1536

var conn net.Conn
var chunkSize = 128 // default

type RTMPStream struct {
	AudioSeqHeader *Chunk
	VideoSeqHeader *Chunk
	AudTimestamp   uint32
	VidTimestamp   uint32
	file           *os.File
	PrevFLVPacket  FLVPacket
}

var streamIdMap = make(map[uint32]*RTMPStream)

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
		conn, err = listener.Accept()
		if err != nil {
			log.Fatal("Error accepting connection:", err)
		}

		go rtmp()
	}
}

func rtmp() {
	println("Client connected:", conn.RemoteAddr().String())
	_, err := initializeHandshake()
	if err != nil {
		log.Fatal("Handshake failed:", err)
	}
	fmt.Println("---HANDSHAKE COMPLETE!---")

	readReq()
	defer conn.Close()
}

func readReq() {
	for {
		chunk := ReadChunk(conn)
		// chunk.Print()
		// chunk.Print() // Waits for the user to press the Enter key
		// fmt.Scanln()
		switch chunk.ChunkHeader.ChunkMessageHeader.MessageTypeId {
		case 0x01:
			chunkSize = chunk.readChunkSizeMessage()
		case 0x08: // Audio
			setTimestamp(chunk)
			packetType := chunk.Payload[1]
			msgStreamId := chunk.ChunkHeader.ChunkMessageHeader.MessageStreamId
			if packetType == 0 {
				// AAC Sequence Header
				storeSeqHeader(msgStreamId, true, chunk)
			} else if packetType == 1 {
				// AAC Raw
				streamIdMap[msgStreamId].writeToFile(chunk)
			}
			// parseAudioPacket(chunk.Payload)
		case 0x09: // Video
			setTimestamp(chunk)
			packetType := chunk.Payload[1]
			msgStreamId := chunk.ChunkHeader.ChunkMessageHeader.MessageStreamId
			if packetType == 0 {
				// AVC Sequence Header
				storeSeqHeader(msgStreamId, false, chunk)

			} else if packetType == 1 {
				// AVC NALU
				streamIdMap[msgStreamId].writeToFile(chunk)
			}
		case 0x12: // 18 Data Message
			fmt.Printf(parseAMF0(chunk.Payload))
			// fmt.Scanln()
		case 0x14: // 20 Command Message
			cmd, _ := parseAMF0Command(chunk.Payload)
			// cmd.Print()
			switch cmd.Name {
			case "connect":
				windowAckMsg := genProtocolControlMessage(5, 2_500_000)
				windowAckMsg.Send(conn)
				peerBandMsg := genProtocolControlMessage(6, 2_500_000)
				peerBandMsg.Send(conn)
				beginMsg := genUserControlMessage(0, 0, 0)
				beginMsg.Send(conn)
				chunkSize := genProtocolControlMessage(1, 4096)
				chunkSize.Send(conn)
				sendAMF0Message(genConnectSuccess(1.0), conn)
			case "releaseStream":
				sendAMF0Message(SerializeAMF0(
					"_result",
					cmd.SeqNum,
					nil,
				), conn)
			case "FCPublish":
				sendAMF0Message(SerializeAMF0(
					"onFCPublish",
					0.0,
					nil,
				), conn)
			case "FCUnpublish":
				fmt.Printf("FCUNPBLISH TODO: FIGURE OUT WHAT TO DO HERE\n")
				chunk.Print()
				cmd.Print()
			case "deleteStream":
				cmd.Print()
				// msgStreamId := chunk.ChunkHseader.ChunkMessageHeader.MessageStreamId
				streamIdMap[1].file.Close()
				break
			case "createStream":
				sendAMF0Message(SerializeAMF0(
					"_result",
					cmd.SeqNum,
					nil,
					1.0), conn)
			case "publish":
				streamBeginMsg := genUserControlMessage(0, 1, 0)
				streamBeginMsg.Send(conn)
				sendAMF0Message(SerializeAMF0(
					"onStatus",
					0.0,
					nil,
					[]AMF0Field{
						{"level", "status"},
						{"code", "NetStream.Publish.Start"},
						{"description", "Stream is now published."},
					},
				), conn)
				entry, exists := streamIdMap[chunk.ChunkHeader.ChunkMessageHeader.MessageStreamId]
				if !exists {
					// create entry in map
					entry = &RTMPStream{}
					streamIdMap[chunk.ChunkHeader.ChunkMessageHeader.MessageStreamId] = entry
				}
			}
		}
	}

	fmt.Printf("Stream over...")

}

func initializeHandshake() (int, error) {
	fmt.Printf("Starting RTMP handshake with client %s\n", conn.RemoteAddr().String())

	// Read the first byte (C0) from the client
	c0_buf := readBytes(1)
	if c0_buf == nil {
		log.Fatal("Failed to read C0 byte from client")
		return 0, fmt.Errorf("Failed to read C0 byte from client")
	}
	// If the first byte is not 0x03, it's an invalid handshake
	if c0_buf[0] != 0x03 {
		log.Fatal("Invalid handshake byte received:", c0_buf[0])
		return 0, fmt.Errorf("Invalid handshake byte received: %d", c0_buf[0])
	}

	c1_buf := readBytes(S1_C1_SIZE) // Junk
	if c1_buf == nil {
		log.Fatal("Failed to read C1 byte from client")
		return 0, fmt.Errorf("Failed to read C1 byte from client")
	}

	sendBytes([]byte{0x03}) // Send S0
	sendBytes(makeS1())     // Send S1
	sendBytes(makeS1())     // Send S2

	c2_buf := readBytes(S1_C1_SIZE) // Junk
	if c2_buf == nil {
		log.Fatal("Failed to read C2 byte from client")
		return 0, fmt.Errorf("Failed to read C2 byte from client")
	}
	return 1, nil
}

func readBytes(numBytes int) []byte {
	buf := make([]byte, numBytes)
	_, err := conn.Read(buf)
	if err != nil {
		log.Fatal("Error reading from connection:", err)
		return nil
	}

	return buf
}

func sendBytes(data []byte) {
	fmt.Printf("Sending bytes %d \n", len(data))
	n, err := conn.Write(data)
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

func storeSeqHeader(msgStreamId uint32, isAudio bool, chunk Chunk) {
	stream := streamIdMap[msgStreamId]
	if isAudio {
		stream.AudioSeqHeader = &chunk
	} else {
		stream.VideoSeqHeader = &chunk
	}
	t := time.Now()
	fileName := "output_vid" + t.Format(time.Kitchen) + ".flv"
	_, err := os.Stat(fileName)
	if stream.HasBoth() && err != nil {
		file, err := createFile(fileName)
		if err == nil {
			// streamIdMap[msgStreamId].FileName = fileName
			stream.file = file
			stream.writeToFile(*stream.AudioSeqHeader)
			stream.writeToFile(*stream.VideoSeqHeader)
		}
	}
}

func (h *RTMPStream) HasBoth() bool {
	return h.AudioSeqHeader != nil && h.VideoSeqHeader != nil
}

func setTimestamp(c Chunk) {
	stream := streamIdMap[c.ChunkHeader.ChunkMessageHeader.MessageStreamId]
	if c.isAudioData() {
		if c.ChunkHeader.ChunkBasicHeader.Fmt == 0 {
			stream.AudTimestamp = c.ChunkHeader.ChunkMessageHeader.Timestamp
		} else {
			stream.AudTimestamp += c.ChunkHeader.ChunkMessageHeader.Timestamp
		}
	} else if c.isVideoData() {
		if c.ChunkHeader.ChunkBasicHeader.Fmt == 0 {
			stream.VidTimestamp = c.ChunkHeader.ChunkMessageHeader.Timestamp
		} else {
			stream.VidTimestamp += c.ChunkHeader.ChunkMessageHeader.Timestamp
		}
	}
}
