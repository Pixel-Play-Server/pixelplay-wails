package mcnbt

import (
	"encoding/binary"
	"fmt"
	"io"
	"math"
)

const (
	tagEnd       = 0
	tagByte      = 1
	tagShort     = 2
	tagInt       = 3
	tagLong      = 4
	tagFloat     = 5
	tagDouble    = 6
	tagByteArray = 7
	tagString    = 8
	tagList      = 9
	tagCompound  = 10
	tagIntArray  = 11
	tagLongArray = 12
)

// ReadNamed reads a named NBT tag (type + name + payload). Root of most MC files.
func ReadNamed(r io.Reader) (string, any, error) {
	typ, err := readByte(r)
	if err != nil {
		return "", nil, err
	}
	if typ == tagEnd {
		return "", nil, nil
	}
	name, err := readString(r)
	if err != nil {
		return "", nil, err
	}
	val, err := readPayload(r, typ)
	return name, val, err
}

func readPayload(r io.Reader, typ byte) (any, error) {
	switch typ {
	case tagByte:
		return readByte(r)
	case tagShort:
		return readI16(r)
	case tagInt:
		return readI32(r)
	case tagLong:
		return readI64(r)
	case tagFloat:
		u, err := readU32(r)
		return math.Float32frombits(u), err
	case tagDouble:
		u, err := readU64(r)
		return math.Float64frombits(u), err
	case tagByteArray:
		n, err := readI32(r)
		if err != nil {
			return nil, err
		}
		b := make([]byte, n)
		_, err = io.ReadFull(r, b)
		return b, err
	case tagString:
		return readString(r)
	case tagList:
		et, err := readByte(r)
		if err != nil {
			return nil, err
		}
		n, err := readI32(r)
		if err != nil {
			return nil, err
		}
		out := make([]any, 0, n)
		for i := int32(0); i < n; i++ {
			v, err := readPayload(r, et)
			if err != nil {
				return nil, err
			}
			out = append(out, v)
		}
		return out, nil
	case tagCompound:
		m := map[string]any{}
		for {
			t, err := readByte(r)
			if err != nil {
				return nil, err
			}
			if t == tagEnd {
				break
			}
			name, err := readString(r)
			if err != nil {
				return nil, err
			}
			v, err := readPayload(r, t)
			if err != nil {
				return nil, err
			}
			m[name] = v
		}
		return m, nil
	case tagIntArray:
		n, err := readI32(r)
		if err != nil {
			return nil, err
		}
		out := make([]int32, n)
		for i := range out {
			v, err := readI32(r)
			if err != nil {
				return nil, err
			}
			out[i] = v
		}
		return out, nil
	case tagLongArray:
		n, err := readI32(r)
		if err != nil {
			return nil, err
		}
		out := make([]int64, n)
		for i := range out {
			v, err := readI64(r)
			if err != nil {
				return nil, err
			}
			out[i] = v
		}
		return out, nil
	default:
		return nil, fmt.Errorf("nbt tag desconocido: %d", typ)
	}
}

func readByte(r io.Reader) (byte, error) {
	var b [1]byte
	_, err := io.ReadFull(r, b[:])
	return b[0], err
}

func readI16(r io.Reader) (int16, error) {
	var b [2]byte
	_, err := io.ReadFull(r, b[:])
	return int16(binary.BigEndian.Uint16(b[:])), err
}

func readI32(r io.Reader) (int32, error) {
	var b [4]byte
	_, err := io.ReadFull(r, b[:])
	return int32(binary.BigEndian.Uint32(b[:])), err
}

func readU32(r io.Reader) (uint32, error) {
	var b [4]byte
	_, err := io.ReadFull(r, b[:])
	return binary.BigEndian.Uint32(b[:]), err
}

func readI64(r io.Reader) (int64, error) {
	var b [8]byte
	_, err := io.ReadFull(r, b[:])
	return int64(binary.BigEndian.Uint64(b[:])), err
}

func readU64(r io.Reader) (uint64, error) {
	var b [8]byte
	_, err := io.ReadFull(r, b[:])
	return binary.BigEndian.Uint64(b[:]), err
}

func readString(r io.Reader) (string, error) {
	n, err := readI16(r)
	if err != nil {
		return "", err
	}
	if n < 0 {
		return "", fmt.Errorf("nbt string negativa")
	}
	b := make([]byte, n)
	if _, err := io.ReadFull(r, b); err != nil {
		return "", err
	}
	return string(b), nil
}

func AsMap(v any) map[string]any {
	m, _ := v.(map[string]any)
	return m
}

func AsString(v any) string {
	s, _ := v.(string)
	return s
}

func AsInt64(v any) int64 {
	switch t := v.(type) {
	case int64:
		return t
	case int32:
		return int64(t)
	case int16:
		return int64(t)
	case byte:
		return int64(t)
	default:
		return 0
	}
}

func AsInt32(v any) int32 {
	return int32(AsInt64(v))
}

func AsByte(v any) byte {
	switch t := v.(type) {
	case byte:
		return t
	case int8:
		return byte(t)
	case int32:
		return byte(t)
	case int64:
		return byte(t)
	default:
		return 0
	}
}
