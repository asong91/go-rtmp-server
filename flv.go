package main

import (
	"encoding/binary"
	"fmt"
	"os"
)

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

func createFile(fileName string) {
	file, err := os.Create(fileName)
	if err != nil {
		fmt.Print("ERROR CREATING FILE")
	}
	file.Write(createHeader())
	defer file.Close()
}

func createHeader() []byte {
	header := make([]byte, 13)
	header[0] = 0x46
	header[1] = 0x4C
	header[2] = 0x56
	header[3] = 0x01
	header[4] = 0x05
	header[8] = 0x09

	// header 9 bytes + prevtagsize 4bytes
	return header
}
