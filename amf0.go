package main

import (
	"encoding/binary"
	"errors"
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
	AMF0Undefined    = 0x06
	AMF0ECMAArray    = 0x08
	AMF0ObjectEnd    = 0x09
)

type OnMetaData struct {
	duration        float64
	fileSize        float64
	width           float64
	height          float64
	videocodecid    float64
	videodatarate   float64
	framerate       float64
	audiocodecid    float64
	audiodatarate   float64
	audiosamplerate float64
	audiosamplesize float64
	audiochannels   float64
	stereo          float64
}

type AMF0Command struct {
	Name   string
	SeqNum float64
	Args   []any
}

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

// Every length in AMF0 comes from the client, so every read is bounds checked
var errAMF0Truncated = errors.New("amf0: data ends before value does")

// Objects can nest. Without a limit, a payload of nested objects could
// overflow the stack, which is a fatal error that recover can't catch.
const maxAMF0Depth = 32

func parseAMF0Command(data []byte) (*AMF0Command, error) {
	cmd := &AMF0Command{}
	offset := 0

	if len(data) < 1 || data[0] != AMF0String {
		return nil, errors.New("amf0: command name is not a string")
	}
	n, name, err := parseString(data[offset+1:]) // skip 0x02 type marker
	if err != nil {
		return nil, err
	}
	cmd.Name = name
	offset += 1 + n

	if len(data) <= offset || data[offset] != AMF0Number {
		return nil, errors.New("amf0: transaction id is not a number")
	}
	n, seqStr, err := parseNumber(data[offset+1:]) // skip 0x00 type marker
	if err != nil {
		return nil, err
	}
	cmd.SeqNum, _ = strconv.ParseFloat(seqStr, 64)
	offset += 1 + n

	for offset < len(data) {
		n, val, err := parseAMF0Value(data[offset:])
		if err != nil {
			return nil, err
		}
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
		numReadBytes, str, err := parseAMF0Value(data[readBytes:])
		if err != nil {
			return amfBody + "<" + err.Error() + ">"
		}
		readBytes += numReadBytes
		amfBody += str
	}
	//fmt.Printf("BODY: \n%s", amfBody)
	return amfBody
}

// Parses one AMF0 value, returns bytes consumed
func parseAMF0Value(data []byte) (int, string, error) {
	return parseAMF0ValueDepth(data, 0)
}

func parseAMF0ValueDepth(data []byte, depth int) (int, string, error) {
	if len(data) < 1 {
		return 0, "", errAMF0Truncated
	}
	readBytes := 0
	datatype := data[readBytes]
	readBytes++

	var consumed int
	var str string
	var err error

	switch datatype {
	case AMF0Number:
		consumed, str, err = parseNumber(data[readBytes:])
	case AMF0Boolean:
		consumed, str, err = parseBoolean(data[readBytes:])
	case AMF0String:
		consumed, str, err = parseString(data[readBytes:])
	case AMF0Object:
		consumed, str, err = parseObject(data[readBytes:], depth+1)
	case AMF0ECMAArray:
		consumed, str, err = parseEMCAArray(data[readBytes:], depth+1)
	case AMF0Null, AMF0Undefined:
		str = "null"
	default:
		// Unknown types have unknown sizes, so the rest can't be parsed reliably
		err = fmt.Errorf("amf0: unsupported type marker 0x%02x", datatype)
	}
	if err != nil {
		return 0, "", err
	}
	readBytes += consumed
	return readBytes, str + "\n", nil
}

func parseEMCAArray(data []byte, depth int) (int, string, error) {
	// 4 byte count (number of elements, can be ignored)
	if len(data) < 4 {
		return 0, "", errAMF0Truncated
	}
	n, str, err := parseProperties(data[4:], depth)
	if err != nil {
		return 0, "", err
	}
	return 4 + n, "[\n" + str + "]", nil
}

func parseObject(data []byte, depth int) (int, string, error) {
	n, str, err := parseProperties(data, depth)
	if err != nil {
		return 0, "", err
	}
	return n, "{\n" + str + "}", nil
}

// Key/value pairs shared by objects and ECMA arrays, ending with 00 00 09
func parseProperties(data []byte, depth int) (int, string, error) {
	if depth > maxAMF0Depth {
		return 0, "", errors.New("amf0: objects nested too deeply")
	}
	totalReadBytes := 0
	str := ""
	for {
		if len(data)-totalReadBytes < 3 {
			return 0, "", errAMF0Truncated
		}
		// check for object end marker 00 00 09
		if data[totalReadBytes] == 0x00 && data[totalReadBytes+1] == 0x00 && data[totalReadBytes+2] == AMF0ObjectEnd {
			totalReadBytes += 3
			break
		}

		keyLen := int(binary.BigEndian.Uint16(data[totalReadBytes : totalReadBytes+2]))
		totalReadBytes += 2
		if len(data)-totalReadBytes < keyLen {
			return 0, "", errAMF0Truncated
		}
		key := string(data[totalReadBytes : totalReadBytes+keyLen])
		totalReadBytes += keyLen

		numReadBytes, valueStr, err := parseAMF0ValueDepth(data[totalReadBytes:], depth)
		if err != nil {
			return 0, "", err
		}
		totalReadBytes += numReadBytes
		str += "\t" + key + ": " + valueStr
	}

	return totalReadBytes, str, nil
}

func parseBoolean(data []byte) (int, string, error) {
	if len(data) < 1 {
		return 0, "", errAMF0Truncated
	}
	if data[0] == 0x01 {
		return 1, "true", nil
	}
	return 1, "false", nil
}

func parseString(data []byte) (int, string, error) {
	if len(data) < 2 {
		return 0, "", errAMF0Truncated
	}
	size := int(binary.BigEndian.Uint16(data[:2]))
	if len(data) < 2+size {
		return 0, "", errAMF0Truncated
	}
	str := string(data[2 : 2+size])
	//fmt.Print(str + "\n")
	return 2 + size, str, nil
}

func parseNumber(data []byte) (int, string, error) {
	if len(data) < 8 {
		return 0, "", errAMF0Truncated
	}
	bits := binary.BigEndian.Uint64(data[:8])
	num := math.Float64frombits(bits)
	return 8, strconv.FormatFloat(num, 'f', 15, 64), nil
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

func (s *Client) setOnMetaDataMessage(data []byte) error {
	offset := 0

	if len(data) < 1 || data[0] != AMF0String {
		return errors.New("amf0: data message name is not a string")
	}
	n, name, err := parseString(data[offset+1:]) // skip 0x02 type marker
	if err != nil {
		return err
	}
	offset += 1 + n
	if name == "@setDataFrame" {
		// TODO: If they send anything else this will break.
		// I'm assuming everything after @setDataFrame is onMetaData
		fmt.Println("---- SETTING DATA FRAME ----")
		s.genOnMetaData(data[offset:])

		relativeOffset := findDurationOffset(data[offset:])
		fmt.Printf("\tRelative Offset: %d\n", relativeOffset)
		if relativeOffset == -1 {
			// 0 means "no duration field", so the file output won't patch anything
			fmt.Println("Duration not found")
			s.durationIndex = 0
		} else {
			//fmt.Printf("OFFSET %d", 24+int(offset)+int(relativeOffset))
			s.durationIndex = 24 + int(relativeOffset)
		}
	}
	// TODO: I need to write flvheader to the top of the file
	// I plan on saving it it an obj and when the stream ends, replace the value at that specific byte
	// TODO: Look for @setDataFrame somehow
	// Write the whole onMetaData either at a specific byte after the stream ends or just set the time value fields
	return nil
}

func findDurationOffset(data []byte) int {
	key := "duration"
	// Room for the 2 byte length, the key, the 0x00 type marker and the 8 byte number
	for i := 0; i+2+len(key)+1+8 <= len(data); i++ {
		// AMF0 string key: 2-byte length prefix + string bytes
		keyLen := int(binary.BigEndian.Uint16(data[i : i+2]))
		if keyLen == len(key) && string(data[i+2:i+2+keyLen]) == key {
			// skip the 2-byte length + key bytes, then skip the 0x00 type marker
			return i + 2 + keyLen + 1
		}
	}
	return -1
}
