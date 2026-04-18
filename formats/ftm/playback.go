package ftm

import (
	"fmt"
	"math"

	"github.com/StarHack/go-tracker-formats/formats/ftm/nes2a03"
)

type cellKey struct {
	ch, pat, row int
}

// chipPlayer clocks a real 2A03 APU from decoded FTM pattern data (track 0, first five channels).
// Expansion audio (VRC6, FDS, …) is not implemented; channels 5+ are ignored.
type chipPlayer struct {
	mod   *Module
	apu   *nes2a03.APU
	cells map[cellKey]PatternCell

	track         int
	frameIdx      int
	row           int
	rowAccum      float64
	samplesPerRow float64

	dmcRAM [65536]byte
}

func newChipPlayer(m *Module, sampleRate int) (*chipPlayer, error) {
	if m == nil || m.Params == nil || m.Frames == nil || len(m.Frames.Tracks) == 0 || m.Patterns == nil {
		return nil, fmt.Errorf("FTM chip playback: incomplete module (need PARAMS, FRAMES, PATTERNS)")
	}
	tr := m.Frames.Tracks[0]
	if tr.PatternLength <= 0 || tr.FrameCount <= 0 {
		return nil, fmt.Errorf("FTM chip playback: invalid track 0 frame/pattern length")
	}
	p := &chipPlayer{
		mod:           m,
		apu:           nes2a03.New(float64(sampleRate)),
		cells:         make(map[cellKey]PatternCell),
		track:         0,
		frameIdx:      0,
		row:           0,
		samplesPerRow: rowDurationSamples(m, tr, sampleRate),
	}
	p.apu.MemRead = func(addr uint16) byte { return p.dmcRAM[addr] }
	_ = m.Params.ExpansionChip // reserved for future routing

	for _, c := range m.Patterns.Rows {
		if c.Track != 0 {
			continue
		}
		p.cells[cellKey{c.Channel, c.Pattern, c.Row}] = c
	}
	// DPCM samples into bus map (C000-based bank not emulated; linear mirror).
	if m.DSamples != nil {
		for _, s := range m.DSamples.Samples {
			if s.Index < 0 || s.Index > 0x3F {
				continue
			}
			base := uint16(s.Index) * 256
			for i, b := range s.Data {
				addr := base + uint16(i)
				p.dmcRAM[addr] = b
			}
		}
	}

	p.apu.WriteRegister(0x4017, 0x40)
	p.apu.WriteRegister(0x4015, 0x0F)
	p.applyRow()
	return p, nil
}

func rowDurationSamples(m *Module, tr TrackFrames, sampleRate int) float64 {
	speed := tr.Speed
	if speed <= 0 {
		speed = defaultSpeed
	}
	if m.Params.Machine == 1 {
		return float64(sampleRate) * float64(speed) / 50.0
	}
	return float64(sampleRate) * float64(speed) / 60.0
}

// StereoSamplesOnePassTrack0 is the number of stereo sample pairs (one Player.Sample call each)
// for a single traversal of track 0: every frame in the frame list, each row 0..patternLength-1.
func StereoSamplesOnePassTrack0(m *Module, sampleRate int) int {
	if m == nil || m.Params == nil || m.Frames == nil || len(m.Frames.Tracks) == 0 {
		return 0
	}
	tr := m.Frames.Tracks[0]
	if tr.FrameCount <= 0 || tr.PatternLength <= 0 {
		return 0
	}
	spr := rowDurationSamples(m, tr, sampleRate)
	rows := int64(tr.FrameCount) * int64(tr.PatternLength)
	n := int(math.Ceil(float64(rows) * spr))
	return n
}

// midiFromCell maps FamiTracker pattern octave + chromatic note to MIDI note number (middle C = 60).
func midiFromCell(oct, note byte) (int, bool) {
	if note == 0 || note > 12 {
		return 0, false
	}
	if oct > 7 {
		oct = 7
	}
	// Oct 0 note 1 (C) -> MIDI 12; Oct 4 note 1 -> 60.
	m := (int(oct)+1)*12 + int(note) - 12
	if m < 1 {
		m = 1
	}
	return m, true
}

func pulsePeriodFromMIDI(midi int) uint16 {
	const cpu = float64(nes2a03.CPUFrequency)
	f := 440.0 * math.Pow(2, float64(midi-69)/12.0)
	if f < 1 {
		f = 1
	}
	p := int(math.Round(cpu/(16.0*f) - 1))
	if p < 8 {
		p = 8
	}
	if p > 0x7ff {
		p = 0x7ff
	}
	return uint16(p)
}

func trianglePeriodFromMIDI(midi int) uint16 {
	const cpu = float64(nes2a03.CPUFrequency)
	f := 440.0 * math.Pow(2, float64(midi-69)/12.0)
	if f < 1 {
		f = 1
	}
	p := int(math.Round(cpu/(32.0*f) - 1))
	if p < 2 {
		p = 2
	}
	if p > 0x7ff {
		p = 0x7ff
	}
	return uint16(p)
}

func (p *chipPlayer) applyRow() {
	m := p.mod
	tf := m.Frames.Tracks[0]
	nCh := m.Params.Channels
	if nCh > 5 {
		nCh = 5 // only classic 2A03 front‑end
	}
	frame := tf.Patterns[p.frameIdx]
	for ch := 0; ch < nCh && ch < len(frame); ch++ {
		idx := int(frame[ch])
		ck := cellKey{ch, idx, p.row}
		cell, ok := p.cells[ck]
		if !ok || cell.Note == 0 || cell.Note > 12 {
			p.silenceChannel(ch)
			continue
		}
		midi, ok2 := midiFromCell(cell.Octave, cell.Note)
		if !ok2 {
			p.silenceChannel(ch)
			continue
		}
		per := pulsePeriodFromMIDI(midi)
		tper := trianglePeriodFromMIDI(midi)
		vol := int(cell.Vol)
		if vol > 0x10 {
			vol = 0x10
		}
		vn := byte(vol & 0x0f)
		if vol == 0 || vol == 0x10 {
			vn = 0x0f
		}
		ctrl := byte(0x30) | (3 << 6) | vn // 75% duty, constant volume
		switch ch {
		case 0:
			p.apu.WriteRegister(0x4002, byte(per))
			p.apu.WriteRegister(0x4003, byte(per>>8))
			p.apu.WriteRegister(0x4000, ctrl)
			p.apu.WriteRegister(0x4001, 0x08)
		case 1:
			p.apu.WriteRegister(0x4006, byte(per))
			p.apu.WriteRegister(0x4007, byte(per>>8))
			p.apu.WriteRegister(0x4004, ctrl)
			p.apu.WriteRegister(0x4005, 0x08)
		case 2:
			p.apu.WriteRegister(0x400A, byte(tper))
			p.apu.WriteRegister(0x400B, byte(tper>>8))
			p.apu.WriteRegister(0x4008, 0x7F)
		case 3:
			p.apu.WriteRegister(0x400E, 0x04)
			p.apu.WriteRegister(0x400C, ctrl)
			p.apu.WriteRegister(0x400F, 0xF8)
		default:
			p.silenceChannel(ch)
		}
	}
	p.apu.WriteRegister(0x4015, 0x0F)
}

func (p *chipPlayer) silenceChannel(ch int) {
	switch ch {
	case 0:
		p.apu.WriteRegister(0x4000, 0x30)
		p.apu.WriteRegister(0x4003, 0)
	case 1:
		p.apu.WriteRegister(0x4004, 0x30)
		p.apu.WriteRegister(0x4007, 0)
	case 2:
		p.apu.WriteRegister(0x400B, 0)
	case 3:
		p.apu.WriteRegister(0x400C, 0x30)
		p.apu.WriteRegister(0x400F, 0)
	}
}

func (p *chipPlayer) advanceRow() {
	tf := p.mod.Frames.Tracks[0]
	p.row++
	if p.row >= tf.PatternLength {
		p.row = 0
		p.frameIdx++
		if p.frameIdx >= tf.FrameCount {
			p.frameIdx = 0
		}
	}
	p.applyRow()
}

func (p *chipPlayer) nextSample() int16 {
	p.rowAccum += 1
	for p.rowAccum >= p.samplesPerRow {
		p.rowAccum -= p.samplesPerRow
		p.advanceRow()
	}
	for {
		ok, v := p.apu.Step()
		if ok {
			return nes2a03.FloatToPCM(v)
		}
	}
}
