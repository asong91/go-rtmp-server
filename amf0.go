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

type AMF0Command struct {
	Name   string
	SeqNum float64
	Args   []any
}

// for objects within commands
type AMF0Field struct {
	Key   string
	Value any
}

func SerializeAMF0(values ...any) []byte {
	buf := []byte{}
	for _, v := range values {
		buf = append(buf, serializeAMF0Value(v)...)
	}
	return buf
}

func serializeAMF0Value(v any) []byte {
	buf := []byte{}
	switch val := v.(type) {
	case string:
		buf = append(buf, 0x02)
		length := uint16(len(val))
		buf = append(buf, byte(length>>8), byte(length))
		buf = append(buf, []byte(val)...)
	case float64:
		buf = append(buf, 0x00)
		b := make([]byte, 8)
		binary.BigEndian.PutUint64(b, math.Float64bits(val))
		buf = append(buf, b...)
	case int:
		buf = append(buf, 0x00)
		b := make([]byte, 8)
		binary.BigEndian.PutUint64(b, math.Float64bits(float64(val)))
		buf = append(buf, b...)
	case bool:
		buf = append(buf, 0x01)
		if val {
			buf = append(buf, 0x01)
		} else {
			buf = append(buf, 0x00)
		}
	case nil:
		buf = append(buf, 0x05)
	case []AMF0Field:
		buf = append(buf, 0x03)
		for _, f := range val {
			length := uint16(len(f.Key))
			buf = append(buf, byte(length>>8), byte(length))
			buf = append(buf, []byte(f.Key)...)
			buf = append(buf, serializeAMF0Value(f.Value)...)
		}
		buf = append(buf, 0x00, 0x00, 0x09)
	}
	return buf
}

func genConnectSuccess(seqnum float64) []byte {
	return SerializeAMF0(
		"_result",
		seqnum,
		[]AMF0Field{
			{"fmsVer", "FMS/3,0,1,123"},
			{"capabilities", 31},
		},
		[]AMF0Field{
			{"level", "status"},
			{"code", "NetConnection.Connect.Success"},
			{"description", "Connection succeeded."},
			{"objectEncoding", 0},
		},
	)
}

func parseAMF0Command(data []byte) (*AMF0Command, error) {
	cmd := &AMF0Command{}
	offset := 0

	n, name := parseString(data[offset+1:]) // skip 0x02 type marker
	cmd.Name = name
	offset += 1 + n

	n, seqStr := parseNumber(data[offset+1:]) // skip 0x00 type marker
	cmd.SeqNum, _ = strconv.ParseFloat(seqStr, 64)
	offset += 1 + n

	for offset < len(data) {
		n, val := parseAMF0Value(data[offset:])
		cmd.Args = append(cmd.Args, val)
		offset += n
	}

	return cmd, nil
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
	case AMF0ECMAArray:
		consumed, str = parseEMCAArray(data[readBytes:])
	}
	readBytes += consumed
	return readBytes, str + "\n"
}

func parseEMCAArray(data []byte) (int, string) {
	totalReadBytes := 0
	// 4 byte count (number of elements, can be ignored)
	totalReadBytes += 4

	str := "[\n"
	for {
		// check for object end marker 00 00 09
		if data[totalReadBytes] == 0x00 && data[totalReadBytes+1] == 0x00 && data[totalReadBytes+2] == 0x09 {
			totalReadBytes += 3
			break
		}

		keyLen := int(binary.BigEndian.Uint16(data[totalReadBytes : totalReadBytes+2]))
		totalReadBytes += 2
		key := string(data[totalReadBytes : totalReadBytes+keyLen])
		totalReadBytes += keyLen

		numReadBytes, valueStr := parseAMF0Value(data[totalReadBytes:])
		totalReadBytes += numReadBytes
		str += "\t" + key + ": " + valueStr
	}

	return totalReadBytes, str + "]"
}

func parseBoolean(data []byte) (int, string) {
	if data[0] == 0x01 {
		return 1, "true"
	}
	return 1, "false"
}

func parseString(data []byte) (int, string) {
	size := binary.BigEndian.Uint16(data[:2])
	str := string(data[2 : 2+size])
	fmt.Print(str + "\n")
	return 2 + int(size), str
}

func parseNumber(data []byte) (int, string) {
	bits := binary.BigEndian.Uint64(data[:8])
	num := math.Float64frombits(bits)
	return 8, strconv.FormatFloat(num, 'f', 15, 64)
}

func parseObject(data []byte) (int, string) {
	totalReadBytes := 0
	str := "{\n"
	for {
		if data[totalReadBytes] == 0x00 && data[totalReadBytes+1] == 0x00 && data[totalReadBytes+2] == 0x09 {
			totalReadBytes += 3
			break
		}

		keyLen := int(binary.BigEndian.Uint16(data[totalReadBytes : totalReadBytes+2]))
		totalReadBytes += 2
		key := string(data[totalReadBytes : totalReadBytes+keyLen])
		totalReadBytes += keyLen

		numReadBytes, valueStr := parseAMF0Value(data[totalReadBytes:])
		totalReadBytes += numReadBytes
		str += "\t" + key + ": " + valueStr
	}

	return totalReadBytes, str + "}"
}

func (a *AMF0Command) Print() {
	fmt.Printf("\t=== AMF0 Command ===\n")
	fmt.Printf("\tName:     %s\n", a.Name)
	fmt.Printf("\tSeqNum:   %f\n", a.SeqNum)
	for _, v := range a.Args {
		switch val := v.(type) {
		case AMF0Field:
			fmt.Printf("\t\t%s: %v", val.Key, val.Value)
		default:
			fmt.Printf("\t%v\n", v)
		}
	}
}
