package main

type ChunkHeader struct {
	Type            uint8
	Timestamp       uint32
	MessageLength   uint32
	MessageTypeID   uint8
	MessageStreamID uint32
}
