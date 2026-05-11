package main

import (
	"encoding/binary"
	"fmt"
	"os"
)

type FLVPacket struct {
	tag             FLVTag
	Payload         []byte
	PreviousTagSize uint32
}

type FLVTag struct {
	/*
		[1 byte type]       ← 8=audio, 9=video, 18=script
		[3 bytes data size] ← size of the payload
		[3 bytes timestamp] ← milliseconds
		[1 byte timestamp extended] ← upper 8 bits of timestamp
		[3 bytes stream ID] ← always 0
	*/
	Type        uint8  // 1 byte
	PayloadSize uint32 // 3 bytes
	Timestamp   uint32 // 3 bytes + 1 optional byte extended
	StreamId    uint32 // 3 bytes Always 0 apparently
}

type AVCDecoderConfigurationRecord struct {
	ConfigurationVersion  uint8
	AVCProfileIndication  uint8
	ProfileCompatability  uint8
	AVCLevelIndication    uint8
	LengthSizeMinusOne    uint8
	SequenceParameterSets [][]byte
	PictureParameterSets  [][]byte
}

func parseVideoPacket(data []byte) {
	readerBytePointer := 0
	firstByte := data[readerBytePointer]
	readerBytePointer++
	frameType := firstByte >> 4 // 4 LMB
	codecId := firstByte & 0x0F // 4 RMB

	fmt.Printf("Frame Type: %d\n CodecId: %d\n", frameType, codecId)

	switch codecId {
	case 7: // AVC
		avcPacketType := data[readerBytePointer]
		readerBytePointer++
		switch avcPacketType {
		case 0: // AVC Sequence Header
			// compositionTime := data[readerBytePointer:3]
			readerBytePointer += 3
			parseAVCDecoderConfigurationRecord(data[readerBytePointer:])
			// aRec.Print()
			// fmt.Printf("Composition Time: %d\n", composition?Time)
		}
	}
}

func (r AVCDecoderConfigurationRecord) Print() {
	fmt.Printf("=== AVCDecoderConfigurationRecord ===\n")
	fmt.Printf("ConfigurationVersion: %d\n", r.ConfigurationVersion)
	fmt.Printf("AVCProfileIndication: %d\n", r.AVCProfileIndication)
	fmt.Printf("ProfileCompatibility: %d\n", r.ProfileCompatability)
	fmt.Printf("AVCLevelIndication:   %d\n", r.AVCLevelIndication)
	fmt.Printf("LengthSizeMinusOne:   %d\n", r.LengthSizeMinusOne)
	fmt.Printf("SPS count: %d\n", len(r.SequenceParameterSets))
	for i, sps := range r.SequenceParameterSets {
		fmt.Printf("  SPS[%d]: % x\n", i, sps)
	}
	fmt.Printf("PPS count: %d\n", len(r.PictureParameterSets))
	for i, pps := range r.PictureParameterSets {
		fmt.Printf("  PPS[%d]: % x\n", i, pps)
	}
}

func parseAVCDecoderConfigurationRecord(data []byte) AVCDecoderConfigurationRecord {
	r := AVCDecoderConfigurationRecord{}
	r.ConfigurationVersion = data[0]
	r.AVCProfileIndication = data[1]
	r.ProfileCompatability = data[2]
	r.AVCLevelIndication = data[3]
	r.LengthSizeMinusOne = data[4] & 0x03

	numSps := data[5] & 0x1F
	offset := 6
	for i := 0; i < int(numSps); i++ {
		spsLen := int(binary.BigEndian.Uint16(data[offset : offset+2]))
		offset += 2
		r.SequenceParameterSets = append(r.SequenceParameterSets, data[offset:offset+spsLen])
		offset += spsLen
	}
	numPPS := data[offset]
	offset++
	for i := 0; i < int(numPPS); i++ {
		ppsLen := int(binary.BigEndian.Uint16(data[offset : offset+2]))
		offset += 2
		r.PictureParameterSets = append(r.PictureParameterSets, data[offset:offset+ppsLen])
		offset += ppsLen
	}

	return r
}

func createFile(fileName string) (*os.File, error) {
	file, err := os.Create(fileName)
	if err != nil {
		fmt.Print("ERROR CREATING FILE")
		return nil, err
	}
	_, err = file.Write(createHeader())
	if err != nil {
		// file.Close()
		return nil, err
	}
	return file, nil
}

func (s *RTMPStream) Open(filename string) error {
	f, err := os.Create(filename)
	if err != nil {
		return err
	}
	s.file = f
	return nil
}

func (s *RTMPStream) Write(data []byte) error {
	_, err := s.file.Write(data)
	return err
}

func (stream *RTMPStream) writeToFile(chunk Chunk) {

	flvTag := FLVTag{}
	flvTag.Type = chunk.ChunkHeader.ChunkMessageHeader.MessageTypeId
	flvTag.PayloadSize = uint32(len(chunk.Payload))
	if chunk.isAudioData() {
		flvTag.Timestamp = stream.AudTimestamp
	} else {
		flvTag.Timestamp = stream.VidTimestamp
	}

	flvTag.StreamId = 0

	flvPacket := FLVPacket{}
	flvPacket.tag = flvTag
	flvPacket.Payload = chunk.Payload
	if stream.PrevFLVPacket.tag.Type == 0 {
		flvPacket.PreviousTagSize = 0
	} else {
		flvPacket.PreviousTagSize = uint32(11 + stream.PrevFLVPacket.tag.PayloadSize)
	}
	stream.PrevFLVPacket = flvPacket

	encodedByte := encodeFLVPacket(flvPacket)
	// fmt.Printf("WRITING BYTES: %x %x %x %x\n", encodedByte[4], encodedByte[5], encodedByte[6], encodedByte[7])
	stream.file.Write(encodedByte)
}

func encodeFLVPacket(packet FLVPacket) []byte {
	res := []byte{}
	res = append(res, byte(packet.PreviousTagSize>>24), byte(packet.PreviousTagSize>>16), byte(packet.PreviousTagSize>>8), byte(packet.PreviousTagSize))
	res = append(res, encodeFLVTag(packet.tag)...)
	res = append(res, packet.Payload...)
	return res
}

func encodeFLVTag(tag FLVTag) []byte {
	res := []byte{}
	res = append(res, tag.Type)
	res = append(res, byte(tag.PayloadSize>>16), byte(tag.PayloadSize>>8), byte(tag.PayloadSize))
	res = append(res, byte(tag.Timestamp>>16), byte(tag.Timestamp>>8), byte(tag.Timestamp))
	res = append(res, byte(tag.Timestamp>>24)) // timestamp extended
	res = append(res, 0, 0, 0)
	return res
}

func createHeader() []byte {
	header := make([]byte, 9)
	header[0] = 0x46
	header[1] = 0x4C
	header[2] = 0x56
	header[3] = 0x01
	header[4] = 0x05
	header[8] = 0x09

	// header 9 bytes + prevtagsize 4bytes
	return header
}
