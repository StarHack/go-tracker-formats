package it

import (
	"encoding/binary"
	"fmt"
	"strings"
)

const (
	itHeaderSize = 0xC0
	itSmpHdrSize = 0x50
)

type itEnvelopeData struct {
	Valid      bool
	Flags      uint8
	Num        uint8
	LoopStart  uint8
	LoopEnd    uint8
	SusStart   uint8
	SusEnd     uint8
	NodeY      [25]uint8
	NodeTick   [25]uint16
}

type itInstrument struct {
	keyboard [120][2]uint8
	VolEnv   itEnvelopeData
	GVol     int
	FadeOut  int
	// FilterCutoff / FilterResonance (IT ifc/ifr, 0–127). Applied on new notes unless local filter mode clears them.
	FilterCutoff    uint8
	FilterResonance uint8
}

type itSample struct {
	Name         string
	Data         []int16
	Length       int
	Frames       int
	Stereo       bool
	LoopStart    int
	LoopEnd      int
	Looped       bool
	PingPong     bool
	C5Speed      int
	DefaultVol   int
	GlobalVol    int
	SampleExists bool
}

type Module struct {
	Title          string
	Flags          uint16
	Special        uint16
	Cmwt           uint16
	Cwtv           uint16
	GV             uint8
	MV             uint8
	Speed          uint8
	Tempo          uint8
	ChnPan         [64]byte
	ChnVol         [64]byte
	Orders         []byte
	InsPtrs        []uint32
	SmpPtrs        []uint32
	PatPtrs        []uint32
	Samples        []itSample
	Patterns       []itPattern
	Instruments    []itInstrument
	UseInstruments bool
	LinearSlides   bool
	ExtFilterRange bool // IT header flags bit 0x1000
	MidiMacros     ITMidiMacroConfig
}

func loadModule(data []byte) (*Module, error) {
	if len(data) < itHeaderSize {
		return nil, fmt.Errorf("IT file too short")
	}
	if string(data[0:4]) != "IMPM" {
		return nil, fmt.Errorf("missing IMPM signature")
	}
	m := &Module{}
	m.Title = strings.TrimRight(string(data[4:30]), "\x00")
	m.Flags = binary.LittleEndian.Uint16(data[0x2C:])
	m.Special = binary.LittleEndian.Uint16(data[0x2E:])
	m.Cmwt = binary.LittleEndian.Uint16(data[0x2A:])
	m.Cwtv = binary.LittleEndian.Uint16(data[0x28:])
	m.GV = data[0x30]
	m.MV = data[0x31]
	m.Speed = data[0x32]
	m.Tempo = data[0x33]
	copy(m.ChnPan[:], data[0x40:0x80])
	copy(m.ChnVol[:], data[0x80:0xC0])
	ordNum := int(binary.LittleEndian.Uint16(data[0x20:]))
	insNum := int(binary.LittleEndian.Uint16(data[0x22:]))
	smpNum := int(binary.LittleEndian.Uint16(data[0x24:]))
	patNum := int(binary.LittleEndian.Uint16(data[0x26:]))
	if ordNum <= 0 || ordNum > 256 {
		return nil, fmt.Errorf("invalid IT ord count %d", ordNum)
	}
	if insNum < 0 || insNum > 256 || smpNum < 0 || smpNum > 256 || patNum < 0 || patNum > 256 {
		return nil, fmt.Errorf("invalid IT header counts")
	}
	ordOff := itHeaderSize
	if ordOff+ordNum > len(data) {
		return nil, fmt.Errorf("IT orders truncated")
	}
	m.Orders = append([]byte(nil), data[ordOff:ordOff+ordNum]...)
	ptrBase := ordOff + ordNum
	need := ptrBase + (insNum+smpNum+patNum)*4
	if need > len(data) {
		return nil, fmt.Errorf("IT pointer table truncated")
	}
	m.InsPtrs = make([]uint32, insNum)
	m.SmpPtrs = make([]uint32, smpNum)
	m.PatPtrs = make([]uint32, patNum)
	for i := 0; i < insNum; i++ {
		m.InsPtrs[i] = binary.LittleEndian.Uint32(data[ptrBase+i*4:])
	}
	base := ptrBase + insNum*4
	for i := 0; i < smpNum; i++ {
		m.SmpPtrs[i] = binary.LittleEndian.Uint32(data[base+i*4:])
	}
	base2 := base + smpNum*4
	for i := 0; i < patNum; i++ {
		m.PatPtrs[i] = binary.LittleEndian.Uint32(data[base2+i*4:])
	}
	ptrEnd := base2 + patNum*4
	msgLen := binary.LittleEndian.Uint16(data[0x36:])
	msgOff := binary.LittleEndian.Uint32(data[0x38:])
	midiCfg, _ := itTryLoadMidiConfig(data, ptrEnd, m.Flags, m.Special, m.InsPtrs, m.SmpPtrs, m.PatPtrs, msgOff, msgLen)
	m.MidiMacros = midiCfg

	m.UseInstruments = m.Flags&4 != 0
	m.LinearSlides = m.Flags&8 != 0
	m.ExtFilterRange = m.Flags&0x1000 != 0

	m.Instruments = make([]itInstrument, insNum+1)
	for i := 0; i < insNum; i++ {
		off := int(m.InsPtrs[i])
		if off <= 0 || off+4 > len(data) {
			continue
		}
		if string(data[off:off+4]) != "IMPI" {
			continue
		}
		kbOff := off + 0x40
		if kbOff+240 <= len(data) {
			for n := 0; n < 120; n++ {
				m.Instruments[i+1].keyboard[n][0] = data[kbOff+n*2]
				m.Instruments[i+1].keyboard[n][1] = data[kbOff+n*2+1]
			}
		}
		if off+60 <= len(data) {
			m.Instruments[i+1].FilterCutoff = data[off+58]
			m.Instruments[i+1].FilterResonance = data[off+59]
		}
		if off+0x1A <= len(data) {
			m.Instruments[i+1].GVol = int(data[off+0x18])
			if m.Instruments[i+1].GVol > 128 {
				m.Instruments[i+1].GVol = 128
			}
		}
		if off+0x16 <= len(data) {
			m.Instruments[i+1].FadeOut = int(binary.LittleEndian.Uint16(data[off+0x14:]))
			if m.Instruments[i+1].FadeOut > 255 {
				m.Instruments[i+1].FadeOut = 255
			}
		}
		envOff := off + 0x130
		if envOff+81 <= len(data) {
			m.Instruments[i+1].VolEnv = parseITEnvelope(data, envOff)
		}
	}

	m.Samples = make([]itSample, smpNum+1)
	for i := 0; i < smpNum; i++ {
		s, err := decodeSample(data, int(m.SmpPtrs[i]), m.Cmwt, m.Cwtv)
		if err != nil {
			return nil, fmt.Errorf("sample %d: %w", i+1, err)
		}
		m.Samples[i+1] = s
	}

	m.Patterns = make([]itPattern, patNum)
	for i := 0; i < patNum; i++ {
		pat, err := unpackPattern(data, int(m.PatPtrs[i]))
		if err != nil {
			return nil, fmt.Errorf("pattern %d: %w", i, err)
		}
		m.Patterns[i] = pat
	}
	return m, nil
}

func parseITEnvelope(data []byte, off int) itEnvelopeData {
	var e itEnvelopeData
	if off+6 > len(data) {
		return e
	}
	e.Flags = data[off]
	e.Num = data[off+1]
	if e.Num > 25 {
		e.Num = 25
	}
	e.LoopStart = data[off+2]
	e.LoopEnd = data[off+3]
	e.SusStart = data[off+4]
	e.SusEnd = data[off+5]
	n := int(e.Num)
	nodeOff := off + 6
	for i := 0; i < n; i++ {
		if nodeOff+3 > len(data) {
			break
		}
		e.NodeY[i] = data[nodeOff]
		e.NodeTick[i] = binary.LittleEndian.Uint16(data[nodeOff+1:])
		nodeOff += 3
	}
	e.Valid = e.Flags&1 != 0 && n >= 2
	return e
}

func decodeSample(data []byte, off int, cmwt, cwtv uint16) (itSample, error) {
	var s itSample
	if off <= 0 || off+itSmpHdrSize > len(data) {
		return s, nil
	}
	if string(data[off:off+4]) != "IMPS" {
		return s, fmt.Errorf("missing IMPS signature")
	}
	s.Name = strings.TrimRight(string(data[off+0x14:off+0x2E]), "\x00")
	flg := data[off+0x12]
	cvt := data[off+0x2E]
	length := int(binary.LittleEndian.Uint32(data[off+0x30:]))
	loopStart := int(binary.LittleEndian.Uint32(data[off+0x34:]))
	loopEnd := int(binary.LittleEndian.Uint32(data[off+0x38:]))
	c5 := int(binary.LittleEndian.Uint32(data[off+0x3C:]))
	sampPtr := int(binary.LittleEndian.Uint32(data[off+0x44:]))
	s.DefaultVol = int(data[off+0x13])
	s.GlobalVol = int(data[off+0x11])
	if s.DefaultVol > 64 {
		s.DefaultVol = 64
	}
	if s.GlobalVol > 64 {
		s.GlobalVol = 64
	}
	if c5 <= 0 {
		c5 = 8363
	}
	s.C5Speed = c5
	s.SampleExists = flg&1 != 0
	if !s.SampleExists || length <= 0 {
		return s, nil
	}
	stereo := flg&4 != 0
	is16 := flg&2 != 0
	compressed := flg&8 != 0
	signed := (cvt&1 != 0) || (cmwt >= 0x0202)
	delta := cvt&4 != 0
	it215 := cwtv >= 0x0215

	if sampPtr <= 0 || sampPtr >= len(data) {
		return s, fmt.Errorf("invalid sample data pointer")
	}
	raw := data[sampPtr:]
	frames := length
	if stereo {
		if frames < 1 {
			return s, fmt.Errorf("invalid stereo sample length")
		}
	}
	if !compressed {
		var byteLen int
		if stereo {
			if is16 {
				byteLen = frames * 2 * 2
			} else {
				byteLen = frames * 2
			}
		} else {
			byteLen = frames
			if is16 {
				byteLen = frames * 2
			}
		}
		if byteLen > len(raw) {
			return s, fmt.Errorf("sample data truncated")
		}
		raw = raw[:byteLen]
		if stereo {
			s.Data = pcmStereoToInt16(raw, is16, signed, delta, frames)
		} else {
			s.Data = pcmToInt16(raw, is16, signed, delta)
		}
	} else {
		if stereo {
			var err error
			if is16 {
				s.Data, err = decompressIT16Stereo(raw, frames, it215)
			} else {
				s.Data, err = decompressIT8Stereo(raw, frames, it215)
			}
			if err != nil {
				return s, err
			}
		} else {
			if is16 {
				d, err := decompressIT16(raw, frames, it215)
				if err != nil {
					return s, err
				}
				s.Data = make([]int16, len(d))
				copy(s.Data, d)
			} else {
				d, err := decompressIT8(raw, frames, it215)
				if err != nil {
					return s, err
				}
				s.Data = make([]int16, len(d))
				for i := range d {
					s.Data[i] = int16(int32(d[i]) << 8)
				}
			}
		}
	}
	s.Stereo = stereo
	s.Length = len(s.Data)
	if stereo {
		s.Frames = s.Length / 2
	} else {
		s.Frames = s.Length
	}
	s.Looped = flg&0x10 != 0
	s.PingPong = flg&0x40 != 0
	if s.Looped {
		if stereo {
			s.LoopStart = loopStart
			s.LoopEnd = loopEnd
			if s.LoopEnd > s.Frames {
				s.LoopEnd = s.Frames
			}
			if s.LoopStart > s.Frames {
				s.LoopStart = s.Frames
			}
		} else {
			s.LoopStart = loopStart
			s.LoopEnd = loopEnd
			if s.LoopEnd > s.Length {
				s.LoopEnd = s.Length
			}
			if s.LoopStart > s.Length {
				s.LoopStart = s.Length
			}
		}
	}
	return s, nil
}

func pcmStereoToInt16(raw []byte, is16, signed, delta bool, frames int) []int16 {
	out := make([]int16, frames*2)
	if is16 {
		if delta {
			var accL, accR int32
			for f := 0; f < frames; f++ {
				i := f * 4
				if i+4 > len(raw) {
					break
				}
				dl := int32(int16(binary.LittleEndian.Uint16(raw[i:])))
				dr := int32(int16(binary.LittleEndian.Uint16(raw[i+2:])))
				accL += dl
				accR += dr
				if accL > 32767 {
					accL = 32767
				}
				if accL < -32768 {
					accL = -32768
				}
				if accR > 32767 {
					accR = 32767
				}
				if accR < -32768 {
					accR = -32768
				}
				out[f*2] = int16(accL)
				out[f*2+1] = int16(accR)
			}
			return out
		}
		for f := 0; f < frames; f++ {
			i := f * 4
			if i+4 > len(raw) {
				break
			}
			u0 := binary.LittleEndian.Uint16(raw[i:])
			u1 := binary.LittleEndian.Uint16(raw[i+2:])
			if signed {
				out[f*2] = int16(u0)
				out[f*2+1] = int16(u1)
			} else {
				out[f*2] = int16(int32(u0) - 32768)
				out[f*2+1] = int16(int32(u1) - 32768)
			}
		}
		return out
	}
	if delta {
		var accL, accR int32
		for f := 0; f < frames; f++ {
			i := f * 2
			if i+2 > len(raw) {
				break
			}
			accL += int32(int8(raw[i]))
			accR += int32(int8(raw[i+1]))
			if accL > 127 {
				accL = 127
			}
			if accL < -128 {
				accL = -128
			}
			if accR > 127 {
				accR = 127
			}
			if accR < -128 {
				accR = -128
			}
			out[f*2] = int16(accL << 8)
			out[f*2+1] = int16(accR << 8)
		}
		return out
	}
	for f := 0; f < frames; f++ {
		i := f * 2
		if i+2 > len(raw) {
			break
		}
		b0, b1 := raw[i], raw[i+1]
		if signed {
			out[f*2] = int16(int32(int8(b0)) << 8)
			out[f*2+1] = int16(int32(int8(b1)) << 8)
		} else {
			out[f*2] = int16(int32(b0)-128) << 8
			out[f*2+1] = int16(int32(b1)-128) << 8
		}
	}
	return out
}

func pcmToInt16(raw []byte, is16, signed, delta bool) []int16 {
	if is16 {
		n := len(raw) / 2
		out := make([]int16, n)
		if delta {
			var acc int32
			for i := 0; i < n; i++ {
				u := binary.LittleEndian.Uint16(raw[i*2:])
				acc += int32(int16(u))
				if acc > 32767 {
					acc = 32767
				}
				if acc < -32768 {
					acc = -32768
				}
				out[i] = int16(acc)
			}
			return out
		}
		for i := 0; i < n; i++ {
			u := binary.LittleEndian.Uint16(raw[i*2:])
			if signed {
				out[i] = int16(u)
			} else {
				out[i] = int16(int32(u) - 32768)
			}
		}
		return out
	}
	out := make([]int16, len(raw))
	if delta {
		var acc int32
		for i, b := range raw {
			acc += int32(int8(b))
			if acc > 127 {
				acc = 127
			}
			if acc < -128 {
				acc = -128
			}
			out[i] = int16(acc << 8)
		}
		return out
	}
	for i, b := range raw {
		if signed {
			out[i] = int16(int32(int8(b)) << 8)
		} else {
			out[i] = int16(int32(b)-128) << 8
		}
	}
	return out
}
