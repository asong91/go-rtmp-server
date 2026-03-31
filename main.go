package main

import (
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

		go handleConnection(conn)
	}
}

func handleConnection(conn net.Conn) {
	println("Client connected:", conn.RemoteAddr().String())
	defer conn.Close()

	buf := make([]byte, 1024)
	_, err := conn.Read(buf)
	if err != nil {
		log.Fatal("Error reading from connection:", err)
		panic(err)
	}

	_, err = conn.Write([]byte("Hello, Client!\n"))
	if err != nil {
		log.Fatal("Error writing to connection:", err)
		panic(err)
	}
}
