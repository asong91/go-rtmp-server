package main

import (
	"fmt"
	"log"
	"net"
)

func main() {
	println("Welcome to the TCP server!")
	listen()
}

func listen() {
	listener, err := net.Listen("tcp", ":8080")
	if err != nil {
		panic(err)
	}
	defer listener.Close()

	for {
		conn, err := listener.Accept()
		if err != nil {
			log.Fatal("Error accepting connection:", err)
		}

		go initHandshake(conn)
	}
}

func initHandshake(conn net.Conn) {
	println("Client connected:", conn.RemoteAddr().String())
	defer conn.Close()

	// Read the first byte (C0) from the client
	c0_buf := readBytes(conn, 1)
	if c0_buf == nil {
		log.Fatal("Failed to read C0 byte from client")
		return
	}
	// If the first byte is not 0x03, it's an invalid handshake
	if c0_buf[0] != 0x03 {
		log.Fatal("Invalid handshake byte received:", c0_buf[0])
		_, err := conn.Write([]byte{0x03})
		if err != nil {
			log.Fatal("Error writing to connection:", err)
		}
		panic(fmt.Sprintf("[HANDSHAKE] Expected 0x03, got %v", c0_buf[0]))
	}

	c1_buf := readBytes(conn, 1536)
	// _, err := conn.Read(c1_buf)
	if c1_buf == nil {
		log.Fatal("Failed to read C1 byte from client")
		return
	}

	sendBytes(conn, []byte{0x03})
	sendBytes(conn, c1_buf)

	c2_buf := readBytes(conn, 1536)
	if c2_buf == nil {
		log.Fatal("Failed to read C2 byte from client")
		return
	}
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
	_, err := conn.Write(data)
	if err != nil {
		log.Fatal("Error writing to connection:", err)
	}
}
