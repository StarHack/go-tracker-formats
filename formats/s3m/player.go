// player.go - S3M player.
// Implements formats.PCMTracker.
// Supports core Scream Tracker 3 sample playback and common effects.
package s3m

import (
	"encoding/binary"
	"math"
	"rad2wav/formats"
	"strings"
)

type s3mSample struct {
	name      string
	length    int
	loopStart int
	loopEnd   int
	looped    bool
	c2spd     int
	volume    int
	data      []int16 // normalized to signed 16-bit
}

type s3mEvent struct {
	note   uint8 // 0..119, 254=cut, 255=none
	inst   uint8
	vol    uint8 // 0..64 or 255 none
	eff    uint8 // raw S3M command 1..26, 0 none
	param  uint8
}

type s3mPattern struct {
	rows   int
	events []s3mEvent // rows * channels
}

type s3mChannel struct {
	sample *s3mSample
	active bool
	note   int

	volume  int
	mixVol  int
	pan     int // 0..255
	globalP int

	pos     float64
	posStep float64

	baseFreq float64
	playFreq float64
	target   float64

	portUpMem   uint8
	portDownMem uint8
	volSlideMem uint8
	offMem      uint8
	toneMem     uint8
	vibSpd      uint8
	vibDep      uint8
	vibPos      int

	retrigMem  uint8
	retrigTick int
	cutTick    int
	delayTick  int
}

type Player struct {
	sampleRate int
	data       []byte

	title string
	l     layout

	samples  []s3mSample // 1-indexed (sample 0 unused)
	patterns []s3mPattern

	pos   int
	row   int
	tick  int
	speed int
	tempo int

	globalVol int

	samCnt     int
	samPerTick int

	channels []s3mChannel

	nextPos int
	nextRow int

	repeating   bool
	initialised bool
}

var _ formats.PCMTracker = (*Player)(nil)

func clampInt(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func clamp16(v int64) int16 {
	if v > 32767 {
		return 32767
	}
	if v < -32768 {
		return -32768
	}
	return int16(v)
}

func s3mNoteToFreq(note int, c2spd int) float64 {
	// note: 48 == C-4 in S3M-ish semantics
	return float64(c2spd) * math.Pow(2.0, float64(note-48)/12.0)
}

func (p *Player) calcSamPerTick() int {
	v := p.sampleRate * 5 / (p.tempo * 2)
	if v < 1 {
		v = 1
	}
	return v
}

func decodeSampleHeader(data []byte, para uint16, signedSamples bool) (s3mSample, bool) {
	var s s3mSample
	if para == 0 {
		return s, false
	}
	off := int(para) << 4
	if off+0x50 > len(data) {
		return s, false
	}
	if data[off] != 1 {
		return s, false
	}
	if string(data[off+0x4C:off+0x50]) != "SCRS" {
		return s, false
	}
	memSeg := (uint32(data[off+0x0D]) << 16) | uint32(binary.LittleEndian.Uint16(data[off+0x0E:off+0x10]))
	sampleOff := int(memSeg << 4)
	length := int(binary.LittleEndian.Uint32(data[off+0x10:]))
	loopBeg := int(binary.LittleEndian.Uint32(data[off+0x14:]))
	loopEnd := int(binary.LittleEndian.Uint32(data[off+0x18:]))
	vol := int(data[off+0x1C])
	flags := data[off+0x1F]
	c2spd := int(binary.LittleEndian.Uint32(data[off+0x20:]))
	name := strings.TrimRight(string(data[off+0x30:off+0x4C]), "\x00")
	if c2spd <= 0 {
		c2spd = 8363
	}
	if vol > 64 {
		vol = 64
	}
	if sampleOff < 0 || sampleOff+length > len(data) {
		return s, false
	}
	is16 := (flags & 0x04) != 0
	raw := data[sampleOff : sampleOff+length]
	var sampleData []int16
	if is16 {
		frames := len(raw) / 2
		sampleData = make([]int16, frames)
		for i := 0; i < frames; i++ {
			u := binary.LittleEndian.Uint16(raw[i*2:])
			if signedSamples {
				sampleData[i] = int16(u)
			} else {
				sampleData[i] = int16(int32(u) - 32768)
			}
		}
		length = frames
		loopBeg /= 2
		loopEnd /= 2
	} else {
		sampleData = make([]int16, len(raw))
		for i, b := range raw {
			if signedSamples {
				sampleData[i] = int16(int8(b)) << 8
			} else {
				sampleData[i] = int16(int(b)-128) << 8
			}
		}
		length = len(raw)
	}
	if loopBeg < 0 || loopBeg > length {
		loopBeg = length
	}
	if loopEnd < loopBeg || loopEnd > length {
		loopEnd = length
	}
	s = s3mSample{
		name:      name,
		length:    length,
		loopStart: loopBeg,
		loopEnd:   loopEnd,
		looped:    (flags&0x01) != 0 && loopEnd-loopBeg > 1,
		c2spd:     c2spd,
		volume:    vol,
		data:      sampleData,
	}
	return s, true
}

func decodePattern(data []byte, para uint16, numCh int, chMap [32]int) (s3mPattern, bool) {
	var p s3mPattern
	if para == 0 {
		p.rows = 64
		p.events = make([]s3mEvent, 64*numCh)
		for i := range p.events {
			p.events[i].vol = 255
			p.events[i].note = 255
		}
		return p, true
	}
	off := int(para) << 4
	if off+2 > len(data) {
		return p, false
	}
	packed := int(binary.LittleEndian.Uint16(data[off:]))
	if packed < 2 || off+packed > len(data) {
		return p, false
	}
	p.rows = 64
	p.events = make([]s3mEvent, p.rows*numCh)
	for i := range p.events {
		p.events[i].vol = 255
		p.events[i].note = 255
	}
	pos := off + 2
	row := 0
	for row < 64 && pos < off+packed {
		b := data[pos]
		pos++
		if b == 0 {
			row++
			continue
		}
		physCh := int(b & 31)
		logical := -1
		if physCh < len(chMap) {
			logical = chMap[physCh]
		}
		if b&32 != 0 {
			if pos+2 > off+packed {
				return p, false
			}
			n := data[pos]
			i := data[pos+1]
			pos += 2
			if logical >= 0 && logical < numCh {
				ev := &p.events[row*numCh+logical]
				ev.inst = i
				if n == 255 {
					ev.note = 255
				} else if n == 254 {
					ev.note = 254
				} else {
					oct := int((n >> 4) & 0x0F)
					nn := int(n & 0x0F)
					if nn < 12 {
						ev.note = uint8(oct*12 + nn)
					} else {
						ev.note = 255
					}
				}
			}
		}
		if b&64 != 0 {
			if pos >= off+packed {
				return p, false
			}
			v := data[pos]
			pos++
			if logical >= 0 && logical < numCh {
				ev := &p.events[row*numCh+logical]
				if v <= 64 {
					ev.vol = v
				}
			}
		}
		if b&128 != 0 {
			if pos+2 > off+packed {
				return p, false
			}
			e := data[pos]
			pr := data[pos+1]
			pos += 2
			if logical >= 0 && logical < numCh {
				ev := &p.events[row*numCh+logical]
				ev.eff = e
				ev.param = pr
			}
		}
	}
	return p, true
}

func (p *Player) Init(tune []byte, sampleRate int) string {
	p.initialised = false
	p.data = tune
	if err := Validate(tune); err != "" {
		return err
	}
	l, ok := detectLayout(tune)
	if !ok {
		return "Not a valid S3M file."
	}
	p.l = l
	p.title = strings.TrimRight(l.Title, "\x00")
	p.sampleRate = sampleRate

	p.samples = make([]s3mSample, l.InstrumentCnt+1)
	for i := 0; i < l.InstrumentCnt; i++ {
		if s, ok := decodeSampleHeader(tune, l.InsPtrs[i], l.SignedSamples); ok {
			p.samples[i+1] = s
		}
	}
	p.patterns = make([]s3mPattern, l.PatternCnt)
	for i := 0; i < l.PatternCnt; i++ {
		pat, ok := decodePattern(tune, l.PatPtrs[i], l.ChannelCount, l.ChannelMap)
		if !ok {
			return "S3M pattern parse failed."
		}
		p.patterns[i] = pat
	}

	p.channels = make([]s3mChannel, l.ChannelCount)
	if len(p.channels) == 0 {
		return "S3M has no PCM channels enabled (likely OPL-only module)."
	}
	for phys := 0; phys < 32; phys++ {
		logical := l.ChannelMap[phys]
		if logical < 0 {
			continue
		}
		// ST3 default channel panning: 0..7 left, 8..15 right.
		pan := 32
		if phys >= 8 {
			pan = 224
		}
		p.channels[logical].pan = pan
		p.channels[logical].globalP = pan
		p.channels[logical].volume = 64
		p.channels[logical].mixVol = 64
		p.channels[logical].cutTick = -1
		p.channels[logical].delayTick = -1
	}

	p.Stop()
	p.initialised = true
	return ""
}

func (p *Player) Stop() {
	if p.channels != nil {
		for i := range p.channels {
			pan := p.channels[i].pan
			gpan := p.channels[i].globalP
			p.channels[i] = s3mChannel{pan: pan, globalP: gpan, volume: 64, mixVol: 64, cutTick: -1, delayTick: -1}
		}
	}
	p.globalVol = clampInt(p.l.GlobalVol, 0, 64)
	if p.globalVol == 0 {
		p.globalVol = 64
	}
	p.speed = clampInt(p.l.InitialSpeed, 1, 31)
	if p.speed == 0 {
		p.speed = 6
	}
	p.tempo = clampInt(p.l.InitialTempo, 32, 255)
	if p.tempo == 0 {
		p.tempo = 125
	}
	p.pos, p.row, p.tick = 0, 0, 0
	p.samCnt = 0
	p.samPerTick = p.calcSamPerTick()
	p.nextPos, p.nextRow = -1, -1
	p.repeating = false
}

func (p *Player) GetDescription() []byte {
	if p.title == "" {
		return nil
	}
	return []byte(p.title)
}

func (p *Player) trigger(ch *s3mChannel, ev s3mEvent, tick0 bool) {
	if ev.inst > 0 && int(ev.inst) < len(p.samples) {
		s := &p.samples[int(ev.inst)]
		if len(s.data) > 0 {
			ch.sample = s
			ch.volume = s.volume
			ch.mixVol = ch.volume
		}
	}
	if ev.note == 255 {
		return
	}
	if ev.note == 254 {
		ch.active = false
		return
	}
	if ch.sample == nil || len(ch.sample.data) == 0 {
		return
	}
	n := int(ev.note)
	ch.note = n
	ch.baseFreq = s3mNoteToFreq(n, ch.sample.c2spd)
	ch.playFreq = ch.baseFreq
	ch.target = ch.baseFreq
	if tick0 && ev.eff == 15 { // Oxx sample offset
		if ev.param != 0 {
			ch.offMem = ev.param
		}
		ch.pos = float64(int(ch.offMem) * 256)
	} else {
		ch.pos = 0
	}
	ch.active = true
}

func (p *Player) doVolSlide(ch *s3mChannel, param uint8) {
	if param != 0 {
		ch.volSlideMem = param
	} else {
		param = ch.volSlideMem
	}
	up := int(param >> 4)
	down := int(param & 0x0F)
	if up > 0 && down == 0 {
		ch.volume = clampInt(ch.volume+up, 0, 64)
	} else if down > 0 && up == 0 {
		ch.volume = clampInt(ch.volume-down, 0, 64)
	}
	ch.mixVol = ch.volume
}

func (p *Player) applyEffect(ch *s3mChannel, ev s3mEvent, tick0 bool) {
	e := ev.eff
	x := ev.param >> 4
	y := ev.param & 0x0F
	switch e {
	case 1: // Axx speed
		if tick0 && ev.param > 0 {
			p.speed = int(ev.param)
		}
	case 2: // Bxx jump
		if tick0 {
			p.nextPos = int(ev.param)
			p.nextRow = 0
		}
	case 3: // Cxx break (BCD)
		if tick0 {
			r := int(x)*10 + int(y)
			if r > 63 {
				r = 0
			}
			p.nextRow = r
		}
	case 4: // Dxy vol slide
		if !tick0 {
			p.doVolSlide(ch, ev.param)
		}
	case 5: // Exx porta down
		if ev.param != 0 {
			ch.portDownMem = ev.param
		}
		if !tick0 {
			ch.baseFreq /= math.Pow(2.0, float64(ch.portDownMem)/768.0)
			ch.playFreq = ch.baseFreq
		}
	case 6: // Fxx porta up
		if ev.param != 0 {
			ch.portUpMem = ev.param
		}
		if !tick0 {
			ch.baseFreq *= math.Pow(2.0, float64(ch.portUpMem)/768.0)
			ch.playFreq = ch.baseFreq
		}
	case 7: // Gxx tone porta
		if ev.param != 0 {
			ch.toneMem = ev.param
		}
		if !tick0 {
			step := math.Pow(2.0, float64(ch.toneMem)/768.0)
			if ch.baseFreq < ch.target {
				ch.baseFreq *= step
				if ch.baseFreq > ch.target {
					ch.baseFreq = ch.target
				}
			} else if ch.baseFreq > ch.target {
				ch.baseFreq /= step
				if ch.baseFreq < ch.target {
					ch.baseFreq = ch.target
				}
			}
			ch.playFreq = ch.baseFreq
		}
	case 8: // Hxy vibrato
		if tick0 {
			if x != 0 {
				ch.vibSpd = x
			}
			if y != 0 {
				ch.vibDep = y
			}
		} else {
			delta := math.Sin(float64(ch.vibPos)*2.0*math.Pi/64.0) * float64(ch.vibDep) / 64.0
			ch.playFreq = ch.baseFreq * math.Pow(2.0, delta/12.0)
			ch.vibPos = (ch.vibPos + int(ch.vibSpd)) & 63
		}
	case 10: // Jxy arpeggio
		if ev.param != 0 && !tick0 {
			switch p.tick % 3 {
			case 1:
				ch.playFreq = ch.baseFreq * math.Pow(2.0, float64(x)/12.0)
			case 2:
				ch.playFreq = ch.baseFreq * math.Pow(2.0, float64(y)/12.0)
			default:
				ch.playFreq = ch.baseFreq
			}
		}
	case 11: // Kxy vib + vol
		if !tick0 {
			p.applyEffect(ch, s3mEvent{eff: 8, param: ev.param}, false)
			p.doVolSlide(ch, ev.param)
		}
	case 12: // Lxy tone + vol
		if !tick0 {
			p.applyEffect(ch, s3mEvent{eff: 7, param: ev.param}, false)
			p.doVolSlide(ch, ev.param)
		}
	case 15: // Oxx sample offset memory
		if tick0 && ev.param != 0 {
			ch.offMem = ev.param
		}
	case 17: // Qxy retrig
		if tick0 {
			if ev.param != 0 {
				ch.retrigMem = ev.param
			}
			ch.retrigTick = 0
		} else {
			if ch.retrigMem&0x0F > 0 {
				ch.retrigTick++
				if ch.retrigTick >= int(ch.retrigMem&0x0F) {
					ch.retrigTick = 0
					ch.pos = 0
					ch.active = ch.sample != nil
				}
			}
		}
	case 19: // Sxx
		switch x {
		case 8: // S8x pan
			if tick0 {
				ch.pan = int(y) * 17
			}
		case 12: // SCx note cut
			if tick0 {
				ch.cutTick = int(y)
				if y == 0 {
					ch.volume = 0
					ch.mixVol = 0
				}
			} else if ch.cutTick >= 0 && p.tick == ch.cutTick {
				ch.volume = 0
				ch.mixVol = 0
			}
		case 13: // SDx note delay
			if tick0 {
				ch.delayTick = int(y)
			} else if ch.delayTick >= 0 && p.tick == ch.delayTick {
				ch.pos = 0
				ch.active = ch.sample != nil
			}
		}
	case 20: // Txx tempo
		if tick0 && ev.param >= 32 {
			p.tempo = int(ev.param)
			p.samPerTick = p.calcSamPerTick()
		}
	case 22: // Vxx global vol
		if tick0 {
			p.globalVol = clampInt(int(ev.param), 0, 64)
		}
	}
}

func (p *Player) processRow() {
	if p.pos >= len(p.l.Orders) {
		return
	}
	ord := p.l.Orders[p.pos]
	if ord == 255 {
		p.repeating = true
		return
	}
	if ord == 254 {
		p.pos++
		if p.pos >= len(p.l.Orders) {
			p.repeating = true
			return
		}
		ord = p.l.Orders[p.pos]
	}
	if int(ord) >= len(p.patterns) {
		return
	}
	pat := p.patterns[int(ord)]
	if p.row >= pat.rows {
		return
	}
	for ci := range p.channels {
		ch := &p.channels[ci]
		ev := pat.events[p.row*len(p.channels)+ci]
		ch.playFreq = ch.baseFreq
		ch.mixVol = ch.volume
		ch.cutTick = -1
		ch.delayTick = -1
		if ev.vol <= 64 {
			ch.volume = int(ev.vol)
			ch.mixVol = ch.volume
		}
		// Tone porta keeps running sample; note sets target only.
		if ev.eff == 7 && ev.note != 255 && ev.note != 254 && ch.sample != nil {
			ch.target = s3mNoteToFreq(int(ev.note), ch.sample.c2spd)
		} else {
			p.trigger(ch, ev, true)
		}
		p.applyEffect(ch, ev, true)
	}
}

func (p *Player) processTick() {
	if p.pos >= len(p.l.Orders) {
		return
	}
	ord := p.l.Orders[p.pos]
	if ord == 254 || ord == 255 || int(ord) >= len(p.patterns) {
		return
	}
	pat := p.patterns[int(ord)]
	if p.row >= pat.rows {
		return
	}
	for ci := range p.channels {
		ch := &p.channels[ci]
		ch.playFreq = ch.baseFreq
		ch.mixVol = ch.volume
		ev := pat.events[p.row*len(p.channels)+ci]
		p.applyEffect(ch, ev, false)
	}
}

func (p *Player) advanceRow() {
	newPos := p.pos
	newRow := p.row + 1
	if p.nextPos >= 0 {
		newPos = p.nextPos
		newRow = 0
	}
	if p.nextRow >= 0 {
		newRow = p.nextRow
		if p.nextPos < 0 {
			newPos++
		}
	}
	p.nextPos, p.nextRow = -1, -1
	if newPos >= p.l.OrderCount {
		newPos = 0
		p.repeating = true
	}
	for {
		if newPos >= p.l.OrderCount {
			newPos = 0
			p.repeating = true
		}
		if p.l.Orders[newPos] == 254 {
			newPos++
			continue
		}
		break
	}
	ord := p.l.Orders[newPos]
	if ord != 255 && int(ord) < len(p.patterns) && newRow >= p.patterns[int(ord)].rows {
		newRow = 0
		newPos++
		if newPos >= p.l.OrderCount {
			newPos = 0
			p.repeating = true
		}
	}
	p.pos, p.row = newPos, newRow
}

func (p *Player) Sample(left, right *int16) bool {
	if !p.initialised {
		*left, *right = 0, 0
		return false
	}
	if p.samCnt == 0 {
		if p.tick == 0 {
			p.processRow()
		} else {
			p.processTick()
		}
	}
	var lAcc, rAcc int64
	for i := range p.channels {
		ch := &p.channels[i]
		if !ch.active || ch.sample == nil || len(ch.sample.data) == 0 || ch.mixVol <= 0 {
			continue
		}
		freq := ch.playFreq
		if freq <= 0 {
			freq = ch.baseFreq
		}
		if freq <= 0 {
			continue
		}
		ch.posStep = freq / float64(p.sampleRate)
		s := ch.sample
		i0 := int(ch.pos)
		if i0 < 0 || i0 >= s.length {
			ch.active = false
			continue
		}
		i1 := i0 + 1
		if s.looped {
			if i1 >= s.loopEnd {
				i1 = s.loopStart
			}
		} else if i1 >= s.length {
			i1 = i0
		}
		frac := ch.pos - float64(i0)
		v0 := float64(s.data[i0])
		v1 := float64(s.data[i1])
		smp := v0*(1-frac) + v1*frac

		g := (float64(ch.mixVol) / 64.0) * (float64(p.globalVol) / 64.0)
		smp *= g
		pan := 128 + (ch.pan-128)/2 // soften hard pan
		if pan < 0 {
			pan = 0
		}
		if pan > 255 {
			pan = 255
		}
		lAcc += int64(smp * float64(255-pan) / 255.0)
		rAcc += int64(smp * float64(pan) / 255.0)

		ch.pos += ch.posStep
		if s.looped {
			for ch.pos >= float64(s.loopEnd) {
				ch.pos -= float64(s.loopEnd - s.loopStart)
			}
		} else if int(ch.pos) >= s.length {
			ch.active = false
		}
	}

	div := int64(len(p.channels))
	if div < 1 {
		div = 1
	}
	*left = clamp16(lAcc / div)
	*right = clamp16(rAcc / div)

	p.samCnt++
	if p.samCnt >= p.samPerTick {
		p.samCnt = 0
		p.tick++
		if p.tick >= p.speed {
			p.tick = 0
			p.advanceRow()
		}
	}
	return p.repeating
}
