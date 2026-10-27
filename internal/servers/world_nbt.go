package servers

import (
	"bytes"
	"compress/gzip"
	"encoding/binary"
	"errors"
	"io"
	"os"
	"strconv"
	"strings"
)

type worldMetadata struct {
	WorldData bool
	Seed      *string
	Version   string
}

// Bounded, allocation-light NBT traversal. Unknown tags are skipped, not decoded
// into arbitrary maps; corrupt sizes, excessive nesting and decompression fail.
func readWorldMetadata(filename string) (worldMetadata, error) {
	var result worldMetadata
	f, err := os.Open(filename)
	if err != nil {
		return result, err
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return result, err
	}
	defer gz.Close()
	data, err := io.ReadAll(io.LimitReader(gz, (16<<20)+1))
	if err != nil {
		return result, err
	}
	if len(data) > 16<<20 {
		return result, errors.New("NBT too large")
	}
	r := bytes.NewReader(data)
	readName := func() (string, error) {
		var n uint16
		if err := binary.Read(r, binary.BigEndian, &n); err != nil {
			return "", err
		}
		b := make([]byte, int(n))
		_, err := io.ReadFull(r, b)
		return string(b), err
	}
	skip := func(n int64) error {
		if n < 0 || n > int64(r.Len()) {
			return io.ErrUnexpectedEOF
		}
		_, err := r.Seek(n, io.SeekCurrent)
		return err
	}
	nodes := 0
	var payload func(byte, string, int) error
	payload = func(tag byte, key string, depth int) error {
		nodes++
		if depth > 64 || nodes > 1000000 {
			return errors.New("NBT complexity limit")
		}
		switch tag {
		case 1:
			return skip(1)
		case 2:
			return skip(2)
		case 3, 5:
			return skip(4)
		case 4:
			var value int64
			if err := binary.Read(r, binary.BigEndian, &value); err != nil {
				return err
			}
			if key == "Data/RandomSeed" || key == "Data/WorldGenSettings/seed" || key == "data/seed" || key == "seed" {
				seed := strconv.FormatInt(value, 10)
				result.Seed = &seed
			}
			return nil
		case 6:
			return skip(8)
		case 7, 11, 12:
			var n int32
			if err := binary.Read(r, binary.BigEndian, &n); err != nil {
				return err
			}
			if n < 0 {
				return errors.New("negative NBT array length")
			}
			size := int64(1)
			if tag == 11 {
				size = 4
			}
			if tag == 12 {
				size = 8
			}
			return skip(int64(n) * size)
		case 8:
			value, err := readName()
			if err == nil && key == "Data/Version/Name" {
				result.Version = value
			}
			return err
		case 9:
			kind, err := r.ReadByte()
			if err != nil {
				return err
			}
			var n int32
			if err = binary.Read(r, binary.BigEndian, &n); err != nil {
				return err
			}
			if n < 0 || n > 1000000 || (kind == 0 && n != 0) {
				return errors.New("invalid NBT list")
			}
			for i := int32(0); i < n; i++ {
				if err = payload(kind, key+"/[]", depth+1); err != nil {
					return err
				}
			}
			return nil
		case 10:
			if key == "Data" {
				result.WorldData = true
			}
			for {
				kind, err := r.ReadByte()
				if err != nil {
					return err
				}
				if kind == 0 {
					return nil
				}
				name, err := readName()
				if err != nil {
					return err
				}
				next := strings.TrimPrefix(key+"/"+name, "/")
				if err = payload(kind, next, depth+1); err != nil {
					return err
				}
			}
		default:
			return errors.New("invalid NBT tag")
		}
	}
	tag, err := r.ReadByte()
	if err != nil || tag != 10 {
		return result, errors.New("expected NBT compound")
	}
	if _, err = readName(); err != nil {
		return result, err
	}
	err = payload(tag, "", 0)
	return result, err
}
