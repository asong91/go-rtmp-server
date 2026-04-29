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

type AMF0Payload struct {
	buf []byte
}

type AMF0Field struct {
	Key   string
	Value any
}

func (a *AMF0Payload) WriteString(s string) {
	a.buf = append(a.buf, 0x02)
	length := uint16(len(s))
	a.buf = append(a.buf, byte(length>>8), byte(length))
	a.buf = append(a.buf, []byte(s)...)
}

func (a *AMF0Payload) WriteNumber(n float64) {
	a.buf = append(a.buf, 0x00)
	bits := math.Float64bits(n)
	b := make([]byte, 8)
	binary.BigEndian.PutUint64(b, bits)
	a.buf = append(a.buf, b...)
}

func (a *AMF0Payload) WriteBoolean(b bool) {
	a.buf = append(a.buf, 0x01)
	if b == false {
		a.buf = append(a.buf, 0x00)
	} else {
		a.buf = append(a.buf, 0x01)
	}
}

func (a *AMF0Payload) WriteNull() {
	a.buf = append(a.buf, 0x05)
}

func (a *AMF0Payload) WriteObject(fields []AMF0Field) {
	a.buf = append(a.buf, 0x03)

	for _, f := range fields {
		a.WriteFieldName(f.Key)
		switch v := f.Value.(type) {
		case string:
			a.WriteString(v)
		case float64:
			a.WriteNumber(v)
		case int:
			a.WriteNumber(float64(v))
		case bool:
			a.WriteBoolean(v)
		case nil:
			a.WriteNull()
		}
	}
	// empty string key + object-end marker
	a.buf = append(a.buf, 0x00, 0x00, 0x09)
}

func (a *AMF0Payload) WriteFieldName(name string) {
	// field names inside objects have NO type marker, just length-prefixed string
	length := uint16(len(name))
	a.buf = append(a.buf, byte(length>>8), byte(length))
	a.buf = append(a.buf, []byte(name)...)
}

func genConnectSuccess(clientId float64) *AMF0Payload {
	payload := &AMF0Payload{}
	payload.WriteString("_result")
	payload.WriteNumber(clientId)
	payload.WriteObject([]AMF0Field{
		{"fmsVer", "FMS/3,0,1,123"},
		{"capabilities", 31},
	})
	payload.WriteObject([]AMF0Field{
		{"level", "status"},
		{"code", "NetConnection.Connect.Success"},
		{"description", "Connection succeeded."},
		{"objectEncoding", 0},
	})

	return payload
}

// Loops until the whole chunk is consumed
func parseAMF0(data []byte) string {
	readBytes := 0
	amfBody := ""
	for readBytes < len(data) {
		numReadBytes, str := parseAMF0Value(data[readBytes:])
		readBytes += numReadBytes
		amfBody += str
	}
	fmt.Printf("BODY: \n%s", amfBody)
	return amfBody
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
