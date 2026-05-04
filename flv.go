package main

import "fmt"

func parseVideoPacket(payload []byte) {
	readerBytePointer := 0
	firstByte := payload[readerBytePointer]
	readerBytePointer++
	frameType := firstByte >> 4 // 4 LMB
	codecId := firstByte & 0x0F // 4 RMB

	fmt.Printf("Frame Type: %d\n CodecId: %d\n", frameType, codecId)

	switch codecId {
	case 7: // AVC
		avcPacketType := payload[readerBytePointer]
		readerBytePointer++
		switch avcPacketType {
		case 0: // AVC Sequence Header
			compositionTime := payload[readerBytePointer:3]
			fmt.Printf("Composition Time: %d\n", compositionTime)
		}
	}

	// TODO: Figure out what AVCDecoderConfigurationRecord is
	// Payload (
	// 17 // type codec
	// 00  // avc packet type
	// 00 00 00 // composition time
	// 01 42 c0 1e ff e1 00 19 67 42 c0 1e d9 01 e0 8f eb 01 10 00 00 03 00 10 00 00 03 03 c0 f1 62 e4 80 01 00 04 68 cb 8c b2): [23 0 0 0 0 1 66 192 30 255 225 0 25 103 66 192 30 217 1 224 143 235 1 16 0 0 3 0 16 0 0 3 3 192 241 98 228 128 1 0 4 104 203 140 178]
}
