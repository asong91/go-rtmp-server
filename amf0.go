package main

import (
	"encoding/binary"
	"fmt"
	"math"
	"strconv"
)

const (
	AMF0_MSG_TYPE_ID = 0x14
	AMF0Number       = 0x00
	AMF0Boolean      = 0x01
	AMF0String       = 0x02
	AMF0Object       = 0x03
	AMF0Null         = 0x05
	AMF0ECMAArray    = 0x08
	AMF0ObjectEnd    = 0x09
)

// Loops until the whole chunk is consumed
func parseAMF0(data []byte) {
	readBytes := 0
	amfBody := ""
	for readBytes < len(data) {
		numReadBytes, str := parseAMF0Value(data[readBytes:])
		readBytes += numReadBytes
		amfBody += str
		// fmt.Printf("Parsed bytes %d so far out of %d\n", readBytes, len(data))
	}
	fmt.Printf("BODY: \n%s", amfBody)
}

// Parses one AMF0 value, returns bytes consumed
func parseAMF0Value(data []byte) (int, string) {
	readBytes := 0
	datatype := data[readBytes]
	readBytes++

	var consumed int
	var str string

	switch datatype {
	case AMF0Number:
		consumed, str = parseNumber(data[readBytes:])
	case AMF0Boolean:
		consumed, str = parseBoolean(data[readBytes:])
	case AMF0String:
		consumed, str = parseString(data[readBytes:])
	case AMF0Object:
		consumed, str = parseObject(data[readBytes:])
	}
	readBytes += consumed
	return readBytes, str + "\n"
}

func parseBoolean(data []byte) (int, string) {
	// TODO
	return 0, ""
}

func parseString(data []byte) (int, string) {
	// 2 bytes for len of string + len
	size := binary.BigEndian.Uint16(data[:2])
	// fmt.Printf("\t[parseString]: size % d\n", size)

	str := string(data[2 : 2+size])
	// fmt.Printf("\t[parseString]: %s\n", str)

	return 2 + int(size), str // type-length-string
}

func parseNumber(data []byte) (int, string) {
	// 8 bytes for the number. Its a float.
	bits := binary.BigEndian.Uint64(data[:8])
	num := math.Float64frombits(bits)
	// fmt.Printf("\t[parseNumber]: %f\n", num)
	return 8, strconv.FormatFloat(num, 'f', 15, 64) //type-8 byte number
}

func parseObject(data []byte) (int, string) {
	totalReadBytes := 0
	str := "{\n"
	for {
		// Check for object end marker: 00 00 09
		// fmt.Printf("%x %x %x", data[readBytes], data[readBytes+1], data[readBytes+2])
		if data[totalReadBytes] == 0x00 && data[totalReadBytes+1] == 0x00 && data[totalReadBytes+2] == 0x09 {
			totalReadBytes += 3
			break
		}

		// Key is always a raw string (no type marker)
		keyLen := int(binary.BigEndian.Uint16(data[totalReadBytes : totalReadBytes+2]))
		totalReadBytes += 2
		key := string(data[totalReadBytes : totalReadBytes+keyLen])
		totalReadBytes += keyLen
		// fmt.Printf("[parseObject]: key = %s\n", key)

		// Value is a full AMF0 value (type marker + data)
		numReadBytes, valueStr := parseAMF0Value(data[totalReadBytes:])
		totalReadBytes += numReadBytes
		str += "\t" + key + ": " + valueStr
	}

	return totalReadBytes, str + "}"
}
