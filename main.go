package main

import (
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"log"
	"log/slog"
	"net"
	"time"
)

const S1_C1_SIZE = 1536

var conn net.Conn

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
		switch chunk.ChunkHeader.ChunkMessageHeader.MessageTypeId {
		case 0x14:
			parseAMF0(chunk.Payload)
			buf := genWindowAckSizeMessage(2_500_000)
			sendBytes(buf)
			sendBytes(genPeerBandwidthMessage(2_500_000, 2))
			readBytes(12)
		}
	}

}

func readChunkBasicHeader() byte {
	header := readBytes(1)

	// CSID TYPE
	csIdType := (header[0] & 0x3F)
	csId := 0
	chunkType := (header[0] >> 6) & 0x3
	/*
		Read bit 2 & 3 of byte to get type.
	*/
	switch csIdType {
	case 0:
		csIdByte := readBytes(1)
		csId = int(csIdByte[0]) + 64
		fmt.Printf("cs id type 0, % x\n", csId)
	case 1:
		csIdByte := readBytes(2)
		csId = int(csIdByte[1])*256 + int(csIdByte[0]) + 64
		fmt.Printf("cs id type 1, % x\n", csId)
	default:
		fmt.Printf("cs id type 2, % x\n", csId)
	}

	fmt.Printf("Chunk Type, % x\n", chunkType)
	return chunkType
}

// func readChunkMessageHeader(chunkType byte) {
// 	// var timestamp []byte
// 	// var msgLen []byte
// 	// var msgTypeId byte
// 	// var msgStreamId []byte

// 	switch chunkType {
// 	case 0x0: // Type 0
// 		header := readBytes(11)
// 		msgLen, msgTypeId := parseType0Header(header)
// 		payload := readMessage(msgLen, 128)
// 		switch msgTypeId {
// 		case 0x14:
// 			parseAMF0(payload)
// 			buf := genWindowAckSizeMessage(2_500_000)
// 			sendBytes(buf)
// 			sendBytes(genPeerBandwidthMessage(2_500_000, 2))
// 			readBytes(12)

// 		}
// 	case 0x1: // Type 1
// 		header := readBytes(7)
// 		timestamp := header[0:3]
// 		fmt.Printf("message header type 1, % x\n\ttimestamp: % x\n", header, timestamp)
// 	case 0x2: // Type 2
// 		header := readBytes(3)
// 		timestamp := header[0:3]
// 		fmt.Printf("message header type 2, % x\n\ttimestamp: % x\n", header, timestamp)
// 	case 0x3: // Type 3
// 		// TODO: This is some special case
// 		fmt.Printf("message header type 3, no chunk message header")
// 	}
// }

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
	fmt.Printf("Received from client: % x\n", buf)

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

func generateRTMPMessage() {
	// generate chunk header

	// message header

	// payload
}

func genWindowAckSizeMessage(sizeInBytes int) []byte {

	buf := generateChunkHeader(0, 2, 4, 5, 0)
	payload := make([]byte, 4)
	binary.BigEndian.PutUint32(payload, uint32(sizeInBytes))

	buf = append(buf, payload...)
	fmt.Printf("header % x", buf)
	return buf
}

func genPeerBandwidthMessage(bandwidth int, limitType uint8) []byte {
	/*
		Limit Types
		0 (Hard): The client must stop sending if the limit is reached until an acknowledgement is received.
		1 (Soft): The client can ignore the limit if a previous "Hard" limit was already set to a higher value.
		2 (Dynamic): This is the most common. It acts as a "Hard" limit if the previous limit was Hard, or a "Soft" limit otherwise.
	*/

	buf := generateChunkHeader(0, 2, 5, 6, 0)
	payload := make([]byte, 5)
	binary.BigEndian.PutUint32(payload[:4], uint32(bandwidth))
	payload[4] = limitType

	buf = append(buf, payload...)
	fmt.Printf("[genPeerBandwidthMessage] header % x", buf)
	return buf
}

func generateChunkHeader(fmtType uint8, csId int, msgLength int, msgTypeId uint8, msgStreamId int) []byte {
	// csID of 2 means action command
	// Chunk header is either 1 2 or 3 byte
	// TODO: Format this to make it more generic. It only works for payload size of 4 bytes
	var buf []byte
	buf = append(buf, (fmtType<<6)|uint8(csId))
	switch fmtType {
	case 0:
		// timestamp 3 bytes
		// Setting to 0 for now

		// msgLength 3 bytes
		h := make([]byte, 11)
		h[3] = uint8(msgLength >> 16)
		h[4] = uint8(msgLength >> 8)
		h[5] = uint8(msgLength)

		// msgTypeId 1 byte,
		h[6] = uint8(msgTypeId)

		// msgStreamId 4 bytes
		h[7] = uint8(msgStreamId >> 24)
		h[8] = uint8(msgStreamId >> 16)
		h[9] = uint8(msgStreamId >> 8)
		h[10] = uint8(msgStreamId)
		buf = append(buf, h...)
	case 1:

	case 2:

	}

	return buf
}
