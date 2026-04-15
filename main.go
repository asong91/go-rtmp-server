package main

import (
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"log"
	"net"
	"time"
)

const S1_C1_SIZE = 1536

func main() {
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
		conn, err := listener.Accept()
		if err != nil {
			log.Fatal("Error accepting connection:", err)
		}

		go rtmp(conn)
	}
}

func rtmp(conn net.Conn) {
	println("Client connected:", conn.RemoteAddr().String())
	_, err := initializeHandshake(conn)
	if err != nil {
		log.Fatal("Handshake failed:", err)
	}

	readHeader(conn)
	defer conn.Close()
}

func readHeader(conn net.Conn) {
	chunkType := readChunkBasicHeader(conn)
	readChunkMessageHeader(conn, chunkType)
}

func readChunkBasicHeader(conn net.Conn) byte {
	header := readBytes(conn, 1)

	// CSID TYPE
	csIdType := (header[0] & 0x3F)
	csId := 0
	chunkType := (header[0] >> 6) & 0x3
	/*
		Read bit 2 & 3 of byte to get type.
	*/
	switch csIdType {
	case 0:
		csIdByte := readBytes(conn, 1)
		csId = int(csIdByte[0]) + 64
		fmt.Printf("cs id type 0, % x\n", csId)
	case 1:
		csIdByte := readBytes(conn, 2)
		csId = int(csIdByte[1])*256 + int(csIdByte[0]) + 64
		fmt.Printf("cs id type 1, % x\n", csId)
	default:
		fmt.Printf("cs id type 2, % x\n", csId)
	}

	fmt.Printf("Chunk Type, % x\n", chunkType)
	return chunkType
}

func readChunkMessageHeader(conn net.Conn, chunkType byte) {

	switch chunkType {
	case 0x0:
		header := readBytes(conn, 11)
		fmt.Printf("message header 0, % x\n", header)
	case 0x1:
		header := readBytes(conn, 7)
		fmt.Printf("message header 1, % x\n", header)
	case 0x2:
		header := readBytes(conn, 3)
		fmt.Printf("message header 2, % x\n", header)
	case 0x3:
		// TODO: This is some special case
		header := readBytes(conn, 11)
		fmt.Printf("message header 3, % x\n", header)
	}
}

// func getCsType(conn net.Conn, input []byte) (int) {
// 	return
// }

// func readHeader(conn net.Conn) (int, error) {
// 	header := make([]byte, 1)
// 	n, err := conn.Read(header)
// 	if err != nil {
// 		log.Fatal("Error reading header from connection:", err)
// 		return 0, err
// 	}
// 	fmt.Printf("Received header: %v\n", header[:n])

// 	// Read fmt type by checking first 2 bits of the header
// 	fmtType := header[0] & 0xC0
// 	fmt.Printf("Chunk Header Type Is: %d", fmtType)
// 	return int(fmtType), nil
// }

func initializeHandshake(conn net.Conn) (int, error) {
	fmt.Printf("Starting RTMP handshake with client %s\n", conn.RemoteAddr().String())

	// Read the first byte (C0) from the client
	c0_buf := readBytes(conn, 1)
	if c0_buf == nil {
		log.Fatal("Failed to read C0 byte from client")
		return 0, fmt.Errorf("Failed to read C0 byte from client")
	}
	// If the first byte is not 0x03, it's an invalid handshake
	if c0_buf[0] != 0x03 {
		log.Fatal("Invalid handshake byte received:", c0_buf[0])
		return 0, fmt.Errorf("Invalid handshake byte received: %d", c0_buf[0])
	}

	c1_buf := readBytes(conn, S1_C1_SIZE) // Junk
	if c1_buf == nil {
		log.Fatal("Failed to read C1 byte from client")
		return 0, fmt.Errorf("Failed to read C1 byte from client")
	}

	sendBytes(conn, []byte{0x03}) // Send S0
	sendBytes(conn, makeS1())     // Send S1
	sendBytes(conn, makeS1())     // Send S2

	c2_buf := readBytes(conn, S1_C1_SIZE) // Junk
	if c2_buf == nil {
		log.Fatal("Failed to read C2 byte from client")
		return 0, fmt.Errorf("Failed to read C2 byte from client")
	}
	return 1, nil
}

func readBytes(conn net.Conn, numBytes int) []byte {
	buf := make([]byte, numBytes)
	_, err := conn.Read(buf)
	if err != nil {
		log.Fatal("Error reading from connection:", err)
		return nil
	}
	fmt.Printf("Received from client: %v\n", buf)

	return buf
}

func sendBytes(conn net.Conn, data []byte) {
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
