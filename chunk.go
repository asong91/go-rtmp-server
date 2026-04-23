package main

import (
	"encoding/binary"
	"fmt"
	"io"
)

const ChunkSize = 128

type Chunk struct {
	ChunkHeader ChunkHeader
	Payload     []byte
}

type ChunkHeader struct {
	ChunkBasicHeader   ChunkBasicHeader
	ChunkMessageHeader ChunkMessageHeader
}

type ChunkMessageHeader struct {
	/*
		Type 0 - 11 bytes
		Type 1 - 7 bytes
		Type 2 - 3 bytes
		Type 3 - None
	*/
	Timestamp       uint32 // 3 bytes
	MessageLength   uint32 // 3 bytes
	MessageTypeId   uint8  // 1 byte
	MessageStreamId uint32 // 4 bytes
}

type ChunkBasicHeader struct {
	Fmt           uint8  // only 2 bits
	ChunkStreamID uint32 // at least 6 bits can be up to 6bits + 2 bytes. 22 bits
}

func ReadChunk(r io.Reader) Chunk {
	header := NewChunkHeader(r)
	return Chunk{ChunkHeader: header, Payload: readMessage(int(header.ChunkMessageHeader.MessageLength), 128)}
}

func NewChunkHeader(r io.Reader) ChunkHeader {
	chunkBasicHeader := NewChunkBasicHeader(r)
	return ChunkHeader{ChunkBasicHeader: chunkBasicHeader, ChunkMessageHeader: NewChunkMessagerHeader(r, chunkBasicHeader)}
}

func NewChunkBasicHeader(r io.Reader) ChunkBasicHeader {
	/*
		First byte is fmt(2 bits) + cs-id (6 bits)
		0 1 2 3 4 5 6 7
		+-+-+-+-+-+-+-+-+
		|fmt|   cs id   |
		+-+-+-+-+-+-+-+-+

		Based on cs-id, cs-id is 6 bits, 2 bytes or 3 bytes
		cs-id = 0. 2 bytes
		cs-id = 1. 3 bytes
	*/
	firstByte := readNumBytes(r, 1)

	csIdType := (firstByte[0] & 0x3F)
	csId := 0
	chunkType := (firstByte[0] >> 6) & 0x3
	/*
		Read bit 2 & 3 of byte to get type.
	*/
	switch csIdType {
	case 0:
		csIdByte := readNumBytes(r, 1)
		csId = int(csIdByte[0]) + 64
		fmt.Printf("cs id type 0, % x\n", csId)
	case 1:
		csIdByte := readNumBytes(r, 2)
		csId = int(csIdByte[1])*256 + int(csIdByte[0]) + 64
		fmt.Printf("cs id type 1, % x\n", csId)
	default:
		fmt.Printf("cs id type 2, % x\n", csId)
	}

	fmt.Printf("Chunk Type, % x\n", chunkType)
	// cbh.Fmt = uint8(chunkType)
	// cbh.ChunkStreamID = uint32(csId)
	return ChunkBasicHeader{Fmt: uint8(chunkType), ChunkStreamID: uint32(csId)}
}

func NewChunkMessagerHeader(r io.Reader, cbh ChunkBasicHeader) ChunkMessageHeader {
	chunkMessageHeader := ChunkMessageHeader{}
	switch cbh.Fmt {
	case 0x0: // Type 0
		header := readNumBytes(r, 11)
		chunkMessageHeader.parseHeaderType(0, header)
	case 0x1: // Type 1
		header := readBytes(7)
		chunkMessageHeader.parseHeaderType(1, header)
	case 0x2: // Type 2
		header := readBytes(3)
		chunkMessageHeader.parseHeaderType(2, header)
	case 0x3: // Type 3
		// TODO: This is some special case
		fmt.Printf("message header type 3, no chunk message header")
	}
	return chunkMessageHeader
}

func (cmh *ChunkMessageHeader) parseHeaderType(chunkType uint8, data []byte) {
	if chunkType < 3 {
		timestamp := data[0:3]
		if timestamp[0] == 0xFF && timestamp[1] == 0xFF && timestamp[2] == 0xFF {
			fmt.Printf("WARNING: [readChunkMessageHeader] type 0 has extended timestamp")
		}
		cmh.Timestamp = binary.BigEndian.Uint32(append([]byte{0}, timestamp...))
	}
	if chunkType < 2 {
		msgLen := data[3:6]
		msgTypeId := data[6] // 20 is AMF0 17 is AMF3
		cmh.MessageLength = binary.BigEndian.Uint32(append([]byte{0}, msgLen...))
		cmh.MessageTypeId = msgTypeId
	}
	if chunkType < 1 {
		msgStreamId := data[7:11]
		cmh.MessageStreamId = binary.BigEndian.Uint32(msgStreamId)
	}
}

func readNumBytes(r io.Reader, numBytes int) []byte {
	buf := make([]byte, numBytes)
	_, err := r.Read(buf)
	if err != nil {
		return nil
	}
	fmt.Printf("Received from client: % x\n", buf)

	return buf
}

func (ch ChunkHeader) Print() {
	fmt.Printf("=== Chunk Header ===\n")
	fmt.Printf("Fmt:             %d\n", ch.ChunkBasicHeader.Fmt)
	fmt.Printf("ChunkStreamID:   %d\n", ch.ChunkBasicHeader.ChunkStreamID)
	fmt.Printf("Timestamp:       %d\n", ch.ChunkMessageHeader.Timestamp)
	fmt.Printf("MessageLength:   %d\n", ch.ChunkMessageHeader.MessageLength)
	fmt.Printf("MessageTypeId:   %d\n", ch.ChunkMessageHeader.MessageTypeId)
	fmt.Printf("MessageStreamId: %d\n", ch.ChunkMessageHeader.MessageStreamId)
}

func readMessage(msgLen int, chunkSize int) []byte {
	// fmt.Printf("[readMessage]: msgLen: %d chunkSize: %d\n", msgLen, chunkSize)
	buf := make([]byte, 0, msgLen)
	remaining := msgLen

	for remaining > 0 {
		toRead := chunkSize
		if remaining < chunkSize {
			toRead = remaining
		}

		chunk := readBytes(toRead)
		buf = append(buf, chunk...)
		remaining -= toRead

		// if more data remains, consume the 0xC3 continuation header
		if remaining > 0 {
			header := readBytes(1)
			if header[0] != 0xC3 {
				fmt.Printf("WARNING: expected 0xC3 continuation header, got %02x\n", header[0])
			}
		}
	}

	return buf
}
