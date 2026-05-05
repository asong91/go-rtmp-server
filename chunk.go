package main

import (
	"encoding/binary"
	"fmt"
	"io"
	"time"
)

var previousChunkHeaders = make(map[uint32]ChunkMessageHeader)

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
	ChunkStreamId uint32 // at least 6 bits can be up to 6bits + 2 bytes. 22 bits
}

func ReadChunk(r io.Reader) Chunk {
	header := ReadChunkHeader(r)
	return Chunk{ChunkHeader: header, Payload: readMessage(int(header.ChunkMessageHeader.MessageLength), chunkSize, int(header.ChunkBasicHeader.ChunkStreamId))}
}

func ReadChunkHeader(r io.Reader) ChunkHeader {
	chunkBasicHeader := ReadChunkBasicHeader(r)
	return ChunkHeader{ChunkBasicHeader: chunkBasicHeader, ChunkMessageHeader: ReadChunkMessagerHeader(r, chunkBasicHeader)}
}

func ReadChunkBasicHeader(r io.Reader) ChunkBasicHeader {
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
		csId = int(csIdType)
		fmt.Printf("cs id type 2, % x\n", csId)
	}

	fmt.Printf("Chunk Fmt Type, % x\n", chunkType)
	// cbh.Fmt = uint8(chunkType)
	// cbh.ChunkStreamID = uint32(csId)
	return ChunkBasicHeader{Fmt: uint8(chunkType), ChunkStreamId: uint32(csId)}
}

func ReadChunkMessagerHeader(r io.Reader, cbh ChunkBasicHeader) ChunkMessageHeader {
	chunkMessageHeader := ChunkMessageHeader{}
	switch cbh.Fmt {
	case 0x0: // Type 0
		header := readNumBytes(r, 11)
		chunkMessageHeader.parseHeaderType(0, header)
		previousChunkHeaders[cbh.ChunkStreamId] = chunkMessageHeader
	case 0x1: // Type 1
		header := readNumBytes(r, 7)
		chunkMessageHeader.parseHeaderType(1, header)
		// inherit stream ID from previous
		if prev, exists := previousChunkHeaders[cbh.ChunkStreamId]; exists {
			chunkMessageHeader.MessageStreamId = prev.MessageStreamId
		}
		previousChunkHeaders[cbh.ChunkStreamId] = chunkMessageHeader
	case 0x2: // Type 2
		header := readNumBytes(r, 3)
		chunkMessageHeader.parseHeaderType(2, header)
		// inherit message length, type, and stream ID from previous
		if prev, exists := previousChunkHeaders[cbh.ChunkStreamId]; exists {
			chunkMessageHeader.MessageLength = prev.MessageLength
			chunkMessageHeader.MessageTypeId = prev.MessageTypeId
			chunkMessageHeader.MessageStreamId = prev.MessageStreamId
		}
		previousChunkHeaders[cbh.ChunkStreamId] = chunkMessageHeader
	case 0x3: // Type 3
		if prev, exists := previousChunkHeaders[cbh.ChunkStreamId]; exists {
			chunkMessageHeader = prev
		} else {
			fmt.Printf("WARNING: No previous header found for chunk stream ID %d\n", cbh.ChunkStreamId)
		}
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
		cmh.MessageStreamId = binary.LittleEndian.Uint32(msgStreamId)
	}
}

func readNumBytes(r io.Reader, numBytes int) []byte {
	buf := make([]byte, numBytes)
	_, err := r.Read(buf)
	if err != nil {
		return nil
	}

	return buf
}

func (ch ChunkHeader) Print() {
	fmt.Printf("=== Chunk Header ===\n")
	fmt.Printf("Fmt:             %d\n", ch.ChunkBasicHeader.Fmt)
	fmt.Printf("ChunkStreamID:   %d\n", ch.ChunkBasicHeader.ChunkStreamId)
	fmt.Printf("Timestamp:       %d\n", ch.ChunkMessageHeader.Timestamp)
	fmt.Printf("MessageLength:   %d\n", ch.ChunkMessageHeader.MessageLength)
	fmt.Printf("MessageTypeId:   %d\n", ch.ChunkMessageHeader.MessageTypeId)
	fmt.Printf("MessageStreamId: %d\n", ch.ChunkMessageHeader.MessageStreamId)
}

func (c *Chunk) Print() {
	fmt.Printf("=== Chunk Request ===\n")
	fmt.Printf("\t=== Chunk Header ===\n")
	fmt.Printf("\tFmt:             %d\n", c.ChunkHeader.ChunkBasicHeader.Fmt)
	fmt.Printf("\tChunkStreamID:   %d\n", c.ChunkHeader.ChunkBasicHeader.ChunkStreamId)
	fmt.Printf("\tTimestamp:       %d\n", c.ChunkHeader.ChunkMessageHeader.Timestamp)
	fmt.Printf("\tMessageLength:   %d\n", c.ChunkHeader.ChunkMessageHeader.MessageLength)
	fmt.Printf("\tMessageTypeId:   %d\n", c.ChunkHeader.ChunkMessageHeader.MessageTypeId)
	fmt.Printf("\tMessageStreamId: %d\n", c.ChunkHeader.ChunkMessageHeader.MessageStreamId)
	fmt.Printf("\t\t=== Payload ===\n")
	fmt.Printf("\t\tPayload (% x): %v\n", c.Payload, c.Payload)
	fmt.Printf("\t\tPayloadLength:  %d\n", len(c.Payload))
}

func readMessage(msgLen int, chunkSize int, csId int) []byte {
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
			expected := byte(0xC0) | byte(csId)
			if header[0] != expected {
				fmt.Printf("WARNING: expected %02x continuation header, got %02x\n", expected, header[0])
			}
		}
	}

	return buf
}

func genProtocolControlMessage(msgTypeId uint8, payload int) Chunk {
	/*
		// TODO: Change this to Enum
		Message Type Can Be
		1 - Set Chunk Size
		2 - Abort Message
		3 - Acknowledgement
		4 - User Control
		5 - Window Acknowledgement Size
		6-  Set Peer Bandwidth

		Limit Type can be
		0 - Hard: The peer SHOULD limit its output bandwidth to the indicated window size.
		1 - Soft: The peer SHOULD limit its output bandwidth to the the window indicated in this message or the limit already in effect, whichever is smaller.
		2 - Dynamic: If the previous Limit Type was Hard, treat this message as though it was marked Hard, otherwise ignore this message.
	*/

	chunkBasicHeader := ChunkBasicHeader{Fmt: 0, ChunkStreamId: 2} // This is always 0 and 2
	chunkMessageHeader := ChunkMessageHeader{MessageStreamId: 0, MessageTypeId: msgTypeId, Timestamp: 0}
	var chunk Chunk

	switch msgTypeId {
	case 1:
		// 32 bits. bit 0 is 0 and 31 bits is the chunk size
		buf := make([]byte, 4)
		binary.BigEndian.PutUint32(buf, uint32(payload)&0x7FFFFFFF)
		chunk.Payload = buf
	case 2:
		// 32 bits. Chunk Stream Id
		buf := make([]byte, 4)
		binary.BigEndian.PutUint32(buf, uint32(payload))
		chunk.Payload = buf
	case 3:
		// 32 bits. 4 bytes
		buf := make([]byte, 4)
		binary.BigEndian.PutUint32(buf, uint32(payload))
		chunk.Payload = buf
	case 4:
		// 2 bytes event type. 4 bytes? Event data?
		// handled in genUserControlMessage()
	case 5:
		// 32 bits. 4 bytes
		buf := make([]byte, 4)
		binary.BigEndian.PutUint32(buf, uint32(payload))
		chunk.Payload = buf
	case 6:
		// 40 bits. 5 bytes
		// Ack Window Size 4bytes + Limit Type 1byte
		buf := make([]byte, 5)
		binary.BigEndian.PutUint32(buf[:4], uint32(payload))
		buf[4] = 2 // Limit type. FFMPEG is always 2
		chunk.Payload = buf
	case 8:

	case 9:

	}

	chunkHeader := ChunkHeader{ChunkBasicHeader: chunkBasicHeader, ChunkMessageHeader: chunkMessageHeader}
	chunk.ChunkHeader = chunkHeader
	return chunk
}

func genUserControlMessage(eventTypeId uint8, streamId int, bufferLen int) Chunk {

	chunkBasicHeader := ChunkBasicHeader{Fmt: 0, ChunkStreamId: 2} // This is always 0 and 2
	chunkMessageHeader := ChunkMessageHeader{MessageStreamId: 0, MessageTypeId: 4, Timestamp: 0}
	var chunk Chunk

	var buf []byte

	switch eventTypeId {
	case 0, 1, 2, 4:
		// Stream Begin, Stream EOF, Stream Dry - 6 bytes
		buf = make([]byte, 6)
		binary.BigEndian.PutUint16(buf[0:2], uint16(eventTypeId))
		binary.BigEndian.PutUint32(buf[2:6], uint32(streamId))
		chunk.Payload = buf

	case 3:
		// SetBufferLength - 10 bytes (event type + stream ID + buffer length)
		buf = make([]byte, 10)
		binary.BigEndian.PutUint16(buf[0:2], uint16(eventTypeId))
		binary.BigEndian.PutUint32(buf[2:6], uint32(streamId))
		binary.BigEndian.PutUint32(buf[6:10], uint32(bufferLen))
		chunk.Payload = buf

	case 6, 7:
		// Ping Request, Ping Response - 6 bytes (event type + timestamp)
		buf = make([]byte, 6)
		binary.BigEndian.PutUint16(buf[0:2], uint16(eventTypeId))
		timestamp := uint32(time.Now().UnixMilli())
		binary.BigEndian.PutUint32(buf[2:6], timestamp)
		chunk.Payload = buf
	}
	chunkHeader := ChunkHeader{ChunkBasicHeader: chunkBasicHeader, ChunkMessageHeader: chunkMessageHeader}
	chunk.ChunkHeader = chunkHeader
	return chunk
}

func sendAMF0Message(payload []byte, w io.Writer) {
	chunkBasicHeader := ChunkBasicHeader{Fmt: 0, ChunkStreamId: 3} // TODO: This 3 might change
	chunkMessageHeader := ChunkMessageHeader{Timestamp: 0, MessageTypeId: 20, MessageStreamId: 0}
	chunkHeader := ChunkHeader{ChunkBasicHeader: chunkBasicHeader, ChunkMessageHeader: chunkMessageHeader}
	chunk := Chunk{ChunkHeader: chunkHeader, Payload: payload}
	chunk.Send(w)
}

func (c *Chunk) Send(w io.Writer) error {
	// Set message length based on payload size
	c.ChunkHeader.ChunkMessageHeader.MessageLength = uint32(len(c.Payload))
	fmt.Println("SENDING CHUNK")
	// c.Print()

	// Create a buffer to hold all serialized data
	buf := make([]byte, 0)

	// Serialize the basic header
	basicHeader := c.serializeBasicHeaderBuffer()
	buf = append(buf, basicHeader...)

	// Serialize the message header
	messageHeader := c.serializeMessageHeaderBuffer()
	buf = append(buf, messageHeader...)

	// Append the payload
	buf = append(buf, c.Payload...)

	// Write everything at once
	_, err := w.Write(buf)
	return err
}

func (c *Chunk) serializeBasicHeaderBuffer() []byte {
	cbh := c.ChunkHeader.ChunkBasicHeader
	csID := cbh.ChunkStreamId

	var header []byte

	if csID < 64 {
		header = []byte{(cbh.Fmt << 6) | uint8(csID)}
	} else if csID < 320 {
		header = make([]byte, 2)
		header[0] = cbh.Fmt << 6
		header[1] = uint8(csID - 64)
	} else {
		header = make([]byte, 3)
		header[0] = (cbh.Fmt << 6) | 1
		id := csID - 64
		header[1] = uint8(id & 0xFF)
		header[2] = uint8((id >> 8) & 0xFF)
	}

	return header
}

func (c *Chunk) serializeMessageHeaderBuffer() []byte {
	cmh := c.ChunkHeader.ChunkMessageHeader
	fmt := c.ChunkHeader.ChunkBasicHeader.Fmt

	switch fmt {
	case 0:
		header := make([]byte, 11)
		ts := cmh.Timestamp
		header[0] = uint8((ts >> 16) & 0xFF)
		header[1] = uint8((ts >> 8) & 0xFF)
		header[2] = uint8(ts & 0xFF)
		len := cmh.MessageLength
		header[3] = uint8((len >> 16) & 0xFF)
		header[4] = uint8((len >> 8) & 0xFF)
		header[5] = uint8(len & 0xFF)
		header[6] = cmh.MessageTypeId
		binary.LittleEndian.PutUint32(header[7:11], cmh.MessageStreamId)
		return header

	case 1:
		header := make([]byte, 7)
		ts := cmh.Timestamp
		header[0] = uint8((ts >> 16) & 0xFF)
		header[1] = uint8((ts >> 8) & 0xFF)
		header[2] = uint8(ts & 0xFF)
		len := cmh.MessageLength
		header[3] = uint8((len >> 16) & 0xFF)
		header[4] = uint8((len >> 8) & 0xFF)
		header[5] = uint8(len & 0xFF)
		header[6] = cmh.MessageTypeId
		return header

	case 2:
		header := make([]byte, 3)
		ts := cmh.Timestamp
		header[0] = uint8((ts >> 16) & 0xFF)
		header[1] = uint8((ts >> 8) & 0xFF)
		header[2] = uint8(ts & 0xFF)
		return header

	case 3:
		return []byte{}
	}

	return []byte{}
}

func (c *Chunk) readChunkSizeMessage() int {
	if c.Payload[0]>>7 != 0 {
		fmt.Printf("ERROR BYTE PACKET IS INVALID FOR CHUNK SIZE")
	}
	value := binary.BigEndian.Uint32(c.Payload) & 0x7FFFFFFF
	return int(value)
}
