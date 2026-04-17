package s3m

import (
	"encoding/binary"
	"fmt"
)

type layout struct {
	Title         string
	OrderCount    int
	InstrumentCnt int
	PatternCnt    int
	GlobalVol     int
	InitialSpeed  int
	InitialTempo  int
	MasterVol     int
	SignedSamples bool
	Orders        []byte
	ChannelMap    [32]int
	ChannelCount  int
	InsPtrs       []uint16
	PatPtrs       []uint16
}

// Validate checks an S3M file for structural validity.
// Returns nil if valid, or an error.
func Validate(data []byte) error {
	l, ok := detectLayout(data)
	if !ok {
		if len(data) < 96 {
			return fmt.Errorf("not an S3M file: too short")
		}
		return fmt.Errorf("not a valid S3M file")
	}
	for _, o := range l.Orders {
		if o == 254 || o == 255 {
			continue
		}
		if int(o) >= l.PatternCnt {
			return fmt.Errorf("S3M order list references a non-existent pattern")
		}
	}
	for _, p := range l.InsPtrs {
		if err := validateInstrument(data, p); err != nil {
			return err
		}
	}
	for _, p := range l.PatPtrs {
		if err := validatePattern(data, p); err != nil {
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

func detectLayout(data []byte) (layout, bool) {
	var l layout
	if len(data) < 96 {
		return l, false
	}
	if data[0x1C] != 0x1A {
		return l, false
	}
	if string(data[0x2C:0x30]) != "SCRM" {
		return l, false
	}
	ord, ok1 := readU16LE(data, 0x20)
	ins, ok2 := readU16LE(data, 0x22)
	pat, ok3 := readU16LE(data, 0x24)
	if !ok1 || !ok2 || !ok3 {
		return l, false
	}
	if ord == 0 || ord > 256 {
		return l, false
	}
	l.OrderCount = int(ord)
	l.InstrumentCnt = int(ins)
	l.PatternCnt = int(pat)
	l.Title = string(data[:28])
	l.GlobalVol = int(data[0x30])
	l.InitialSpeed = int(data[0x31])
	l.InitialTempo = int(data[0x32])
	l.MasterVol = int(data[0x33])
	l.SignedSamples = data[0x2A] == 1
	for i := range l.ChannelMap {
		l.ChannelMap[i] = -1
	}
	mixCh := 0
	for ch := 0; ch < 32; ch++ {
		v := data[0x40+ch]
		if v < 16 {
			l.ChannelMap[ch] = mixCh
			mixCh++
		}
	}
	l.ChannelCount = mixCh
	tableOff := 0x60
	need := tableOff + l.OrderCount + (l.InstrumentCnt+l.PatternCnt)*2
	if need > len(data) {
		return l, false
	}
	l.Orders = make([]byte, l.OrderCount)
	copy(l.Orders, data[tableOff:tableOff+l.OrderCount])
	off := tableOff + l.OrderCount
	l.InsPtrs = make([]uint16, l.InstrumentCnt)
	for i := 0; i < l.InstrumentCnt; i++ {
		v, _ := readU16LE(data, off+i*2)
		l.InsPtrs[i] = v
	}
	off += l.InstrumentCnt * 2
	l.PatPtrs = make([]uint16, l.PatternCnt)
	for i := 0; i < l.PatternCnt; i++ {
		v, _ := readU16LE(data, off+i*2)
		l.PatPtrs[i] = v
	}
	return l, true
}

func validateInstrument(data []byte, para uint16) error {
	if para == 0 {
		return nil
	}
	off := int(para) << 4
	if off+0x50 > len(data) {
		return fmt.Errorf("S3M file is truncated in instrument header")
	}
	typ := data[off]
	if typ != 0 && typ != 1 {
		return fmt.Errorf("S3M contains unsupported instrument type")
	}
	if typ == 0 {
		return nil
	}
	if string(data[off+0x4C:off+0x50]) != "SCRS" {
		return fmt.Errorf("S3M instrument does not contain SCRS signature")
	}
	length, _ := readU32LE(data, off+0x10)
	loopBeg, _ := readU32LE(data, off+0x14)
	loopEnd, _ := readU32LE(data, off+0x18)
	if loopBeg > length || loopEnd > length || loopBeg > loopEnd {
		return fmt.Errorf("S3M instrument has invalid loop bounds")
	}
	memSeg := (uint32(data[off+0x0D]) << 16) | uint32(binary.LittleEndian.Uint16(data[off+0x0E:]))
	sampleOff := int(memSeg << 4)
	if sampleOff+int(length) > len(data) {
		return fmt.Errorf("S3M file is truncated in sample data")
	}
	return nil
}

func validatePattern(data []byte, para uint16) error {
	if para == 0 {
		return nil
	}
	off := int(para) << 4
	if off+2 > len(data) {
		return fmt.Errorf("S3M file is truncated in pattern header")
	}
	packed := int(binary.LittleEndian.Uint16(data[off:]))
	if packed < 2 || off+packed > len(data) {
		return fmt.Errorf("S3M file is truncated in pattern data")
	}
	return nil
}

