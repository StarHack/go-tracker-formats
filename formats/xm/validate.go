package xm

import (
	"encoding/binary"
	"fmt"
)

const (
	xmMagic            = "Extended Module: "
	xmMinHeaderSize    = 60
	xmMinPatternHeader = 9
	xmMinInstrHeader   = 29
	xmSampleHeaderSize = 40
)

type moduleLayout struct {
	HeaderSize      int
	SongLength      int
	Restart         int
	Channels        int
	PatternCount    int
	InstrumentCount int
	Flags           uint16
	Tempo           int
	BPM             int
	Orders          []byte
}

// Validate checks a FastTracker 2 XM file for structural validity.
// Returns nil if valid, or an error otherwise.
func Validate(data []byte) error {
	layout, ok := detectHeader(data)
	if !ok {
		if len(data) < xmMinHeaderSize {
			return fmt.Errorf("not an XM file: too short")
		}
		return fmt.Errorf("not a valid FastTracker 2 XM file")
	}
	for _, order := range layout.Orders {
		if order >= byte(layout.PatternCount) && order != 0xFF {
			return fmt.Errorf("XM order list references a non-existent pattern")
		}
	}
	offset := 60 + layout.HeaderSize
	for i := 0; i < layout.PatternCount; i++ {
		var err error
		offset, err = validatePattern(data, offset, layout.Channels)
		if err != nil {
			return err
		}
	}
	for i := 0; i < layout.InstrumentCount; i++ {
		var err error
		offset, err = validateInstrument(data, offset)
		if err != nil {
			return err
		}
	}
	return nil
}

func readU16LE(data []byte, off int) (uint16, bool) {
	if off+2 > len(data) {
		return 0, false
	}
	return binary.LittleEndian.Uint16(data[off:]), true
}

func readU32LE(data []byte, off int) (uint32, bool) {
	if off+4 > len(data) {
		return 0, false
	}
	return binary.LittleEndian.Uint32(data[off:]), true
}

func detectHeader(data []byte) (moduleLayout, bool) {
	if len(data) < xmMinHeaderSize {
		return moduleLayout{}, false
	}
	if string(data[:17]) != xmMagic {
		return moduleLayout{}, false
	}
	if data[37] != 0x1A {
		return moduleLayout{}, false
	}
	hdrSize32, ok := readU32LE(data, 60)
	if !ok {
		return moduleLayout{}, false
	}
	hdrSize := int(hdrSize32)
	if hdrSize < 20 || 60+hdrSize > len(data) {
		return moduleLayout{}, false
	}
	songLen, ok := readU16LE(data, 64)
	if !ok {
		return moduleLayout{}, false
	}
	restart, _ := readU16LE(data, 66)
	channels, _ := readU16LE(data, 68)
	patterns, _ := readU16LE(data, 70)
	instruments, _ := readU16LE(data, 72)
	flags, _ := readU16LE(data, 74)
	tempo, _ := readU16LE(data, 76)
	bpm, _ := readU16LE(data, 78)
	if songLen == 0 || songLen > 256 || channels == 0 {
		return moduleLayout{}, false
	}
	ordersEnd := 80 + int(songLen)
	if ordersEnd > 60+hdrSize || ordersEnd > len(data) {
		return moduleLayout{}, false
	}
	orders := make([]byte, songLen)
	copy(orders, data[80:ordersEnd])
	return moduleLayout{
		HeaderSize:      hdrSize,
		SongLength:      int(songLen),
		Restart:         int(restart),
		Channels:        int(channels),
		PatternCount:    int(patterns),
		InstrumentCount: int(instruments),
		Flags:           flags,
		Tempo:           int(tempo),
		BPM:             int(bpm),
		Orders:          orders,
	}, true
}

func validatePattern(data []byte, offset int, channels int) (int, error) {
	if offset+xmMinPatternHeader > len(data) {
		return offset, fmt.Errorf("XM file is truncated in pattern header")
	}
	hdrLen32, _ := readU32LE(data, offset)
	hdrLen := int(hdrLen32)
	if hdrLen < xmMinPatternHeader || offset+hdrLen > len(data) {
		return offset, fmt.Errorf("XM file contains an invalid pattern header")
	}
	if data[offset+4] != 0 {
		return offset, fmt.Errorf("XM file uses unsupported pattern packing type")
	}
	rows, _ := readU16LE(data, offset+5)
	packedSize, _ := readU16LE(data, offset+7)
	if rows == 0 {
		rows = 256
	}
	patDataOff := offset + hdrLen
	patDataEnd := patDataOff + int(packedSize)
	if patDataEnd > len(data) {
		return offset, fmt.Errorf("XM file is truncated in pattern data")
	}
	cells := int(rows) * channels
	pos := patDataOff
	for i := 0; i < cells; i++ {
		if int(packedSize) == 0 {
			break
		}
		if pos >= patDataEnd {
			return offset, fmt.Errorf("XM pattern data ends before all rows/channels are described")
		}
		b := data[pos]
		pos++
		if b&0x80 != 0 {
			need := 0
			if b&0x01 != 0 {
				need++
			}
			if b&0x02 != 0 {
				need++
			}
			if b&0x04 != 0 {
				need++
			}
			if b&0x08 != 0 {
				need++
			}
			if b&0x10 != 0 {
				need++
			}
			if pos+need > patDataEnd {
				return offset, fmt.Errorf("XM pattern contains a truncated packed event")
			}
			pos += need
		} else {
			if pos+4 > patDataEnd {
				return offset, fmt.Errorf("XM pattern contains a truncated event")
			}
			pos += 4
		}
	}
	return patDataEnd, nil
}

func validateInstrument(data []byte, offset int) (int, error) {
	if offset+xmMinInstrHeader > len(data) {
		return offset, fmt.Errorf("XM file is truncated in instrument header")
	}
	instrSize32, _ := readU32LE(data, offset)
	instrSize := int(instrSize32)
	if instrSize < xmMinInstrHeader || offset+instrSize > len(data) {
		return offset, fmt.Errorf("XM file contains an invalid instrument header")
	}
	numSamples, _ := readU16LE(data, offset+27)
	sampleHdrSize := 0
	if numSamples > 0 {
		v, ok := readU32LE(data, offset+29)
		if !ok {
			return offset, fmt.Errorf("XM file is truncated in instrument sample-header size")
		}
		sampleHdrSize = int(v)
		if sampleHdrSize < xmSampleHeaderSize {
			return offset, fmt.Errorf("XM instrument contains an invalid sample header size")
		}
	}
	sampleHeadersOff := offset + instrSize
	sampleDataOff := sampleHeadersOff + int(numSamples)*sampleHdrSize
	if sampleDataOff > len(data) {
		return offset, fmt.Errorf("XM file is truncated in sample headers")
	}
	cursor := sampleHeadersOff
	for i := 0; i < int(numSamples); i++ {
		if cursor+sampleHdrSize > len(data) {
			return offset, fmt.Errorf("XM file is truncated in sample header table")
		}
		length32, _ := readU32LE(data, cursor)
		loopStart32, _ := readU32LE(data, cursor+4)
		loopLen32, _ := readU32LE(data, cursor+8)
		typ := data[cursor+14]
		is16 := (typ & 0x10) != 0
		length := int(length32)
		loopStart := int(loopStart32)
		loopLen := int(loopLen32)
		if is16 {
			if length%2 != 0 || loopStart%2 != 0 || loopLen%2 != 0 {
				return offset, fmt.Errorf("XM sample %d uses invalid 16-bit byte counts", i+1)
			}
		}
		if loopStart > length || loopStart+loopLen > length {
			return offset, fmt.Errorf("XM sample %d has invalid loop bounds", i+1)
		}
		sampleDataOff += length
		if sampleDataOff > len(data) {
			return offset, fmt.Errorf("XM file is truncated in sample %d data", i+1)
		}
		cursor += sampleHdrSize
	}
	return sampleDataOff, nil
}
