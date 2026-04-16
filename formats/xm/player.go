// player.go - FastTracker 2 XM player.
// Implements formats.PCMTracker.
package xm

import (
	"encoding/binary"
	"math"
	"rad2wav/formats"
	"strings"
)

type xmEvent struct {
	note   uint8
	inst   uint8
	vol    uint8
	effect uint8
	param  uint8
}

type xmPattern struct {
	rows   int
	events []xmEvent
}

type xmEnvPoint struct {
	frame int
	value int
}

type xmEnvelope struct {
	points         []xmEnvPoint
	enabled        bool
	sustainEnabled bool
	loopEnabled    bool
	sustainPoint   int
	loopStart      int
	loopEnd        int
}

type xmSample struct {
	length    int
	loopStart int
	loopLen   int
	loopType  int
	is16      bool
	volume    int
	finetune  int8
	panning   int
	relNote   int8
	name      string
	data      []int16
}

type xmInstrument struct {
	name         string
	sampleMap    [96]uint8
	volEnv       xmEnvelope
	panEnv       xmEnvelope
	vibratoType  uint8
	vibratoSweep uint8
	vibratoDepth uint8
	vibratoRate  uint8
	fadeout      int
	samples      []xmSample
}

type delayedEvent struct {
	active bool
	event  xmEvent
}

type xmChannel struct {
	instIndex   int
	inst        *xmInstrument
	sampleIndex int
	sample      *xmSample
	keyOn       bool
	active      bool
	note        int
	basePitch   float64
	playPitch   float64
	targetPitch float64
	samplePos   float64
	sampleDir   float64
	baseVolume  int
	volume      int
	pan         int
	fadeoutVol  int
	volEnvPos   int
	panEnvPos   int

	arpX uint8
	arpY uint8

	portaUpMem   uint8
	portaDownMem uint8
	tonePortaMem uint8
	volSlideMem  uint8
	panSlideMem  uint8
	sampleOffMem uint8
	globVolMem   int
	vibratoSpd   uint8
	vibratoDepth uint8
	vibratoPhase int
	vibratoWave  uint8
	tremoloSpd   uint8
	tremoloDepth uint8
	tremoloPhase int
	tremoloWave  uint8
	tremorOn     int
	tremorOff    int
	tremorPos    int
	retrigTicks  int
	retrigCount  int
	cutTick      int
	keyoffTick   int
	delayTick    int
	delayed      delayedEvent
	autoVibPos   int
}

type Player struct {
	sampleRate  int
	data        []byte
	title       string
	tracker     string
	module      moduleLayout
	linearFreq  bool
	patterns    []xmPattern
	instruments []xmInstrument
	channels    []xmChannel
	globalVol   int

	pos         int
	row         int
	speed       int
	bpm         int
	tick        int
	samCnt      int
	samPerTick  int
	nextPos     int
	nextRow     int
	nextRowSame bool
	patDelay    int
	orderMap    [8]uint32
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

func evalWaveform(kind uint8, phase int) float64 {
	p := phase & 63
	switch kind & 3 {
	case 1:
		return 1.0 - float64(p)/32.0
	case 2:
		if p < 32 {
			return 1
		}
		return -1
	default:
		return math.Sin(float64(p) * 2 * math.Pi / 64)
	}
}

func envelopeValue(env xmEnvelope, pos int) int {
	if !env.enabled || len(env.points) == 0 {
		return 64
	}
	if pos <= env.points[0].frame {
		return env.points[0].value
	}
	for i := 1; i < len(env.points); i++ {
		a := env.points[i-1]
		b := env.points[i]
		if pos <= b.frame {
			if b.frame == a.frame {
				return b.value
			}
			t := float64(pos-a.frame) / float64(b.frame-a.frame)
			return int(math.Round(float64(a.value) + (float64(b.value-a.value) * t)))
		}
	}
	return env.points[len(env.points)-1].value
}

func advanceEnvelope(env xmEnvelope, pos int, keyOn bool) int {
	if !env.enabled || len(env.points) == 0 {
		return pos
	}
	if keyOn && env.sustainEnabled && env.sustainPoint >= 0 && env.sustainPoint < len(env.points) {
		sf := env.points[env.sustainPoint].frame
		if pos >= sf {
			return pos
		}
	}
	pos++
	if env.loopEnabled && env.loopStart >= 0 && env.loopEnd >= env.loopStart && env.loopEnd < len(env.points) {
		endFrame := env.points[env.loopEnd].frame
		if pos > endFrame {
			pos = env.points[env.loopStart].frame
		}
	}
	return pos
}

func pitchToStep(pitch float64, sampleRate int) float64 {
	// Reference: C-4 (noteIdx 48, 0-based) = 8363 Hz (FT2 / libxm standard)
	freq := 8363.0 * math.Pow(2.0, (pitch-48.0)/12.0)
	return freq / float64(sampleRate)
}

func decodeDelta8(data []byte) []int16 {
	out := make([]int16, len(data))
	var acc int16
	for i, b := range data {
		acc += int16(int8(b))
		out[i] = acc << 8
	}
	return out
}

func decodeDelta16(data []byte) []int16 {
	count := len(data) / 2
	out := make([]int16, count)
	var acc int32
	for i := 0; i < count; i++ {
		d := int16(binary.LittleEndian.Uint16(data[i*2:]))
		acc += int32(d)
		if acc > 32767 {
			acc = 32767
		} else if acc < -32768 {
			acc = -32768
		}
		out[i] = int16(acc)
	}
	return out
}

func parseEnvelope(pointsRaw []byte, count int, typ, sustain, loopStart, loopEnd uint8) xmEnvelope {
	env := xmEnvelope{
		enabled:        typ&1 != 0,
		sustainEnabled: typ&2 != 0,
		loopEnabled:    typ&4 != 0,
		sustainPoint:   int(sustain),
		loopStart:      int(loopStart),
		loopEnd:        int(loopEnd),
	}
	if count > 12 {
		count = 12
	}
	env.points = make([]xmEnvPoint, 0, count)
	for i := 0; i < count; i++ {
		off := i * 4
		frame := int(binary.LittleEndian.Uint16(pointsRaw[off:]))
		value := int(binary.LittleEndian.Uint16(pointsRaw[off+2:]))
		if value > 64 {
			value = 64
		}
		env.points = append(env.points, xmEnvPoint{frame: frame, value: value})
	}
	return env
}

func unpackPatterns(data []byte, layout moduleLayout) ([]xmPattern, int, string) {
	offset := 60 + layout.HeaderSize
	patterns := make([]xmPattern, layout.PatternCount)
	for pi := 0; pi < layout.PatternCount; pi++ {
		hdrLen := int(binary.LittleEndian.Uint32(data[offset:]))
		rows := int(binary.LittleEndian.Uint16(data[offset+5:]))
		if rows == 0 {
			rows = 256
		}
		packedSize := int(binary.LittleEndian.Uint16(data[offset+7:]))
		patDataOff := offset + hdrLen
		patDataEnd := patDataOff + packedSize
		pat := xmPattern{rows: rows, events: make([]xmEvent, rows*layout.Channels)}
		pos := patDataOff
		for i := 0; i < len(pat.events) && pos < patDataEnd; i++ {
			b := data[pos]
			pos++
			var ev xmEvent
			if b&0x80 != 0 {
				if b&0x01 != 0 {
					ev.note = data[pos]
					pos++
				}
				if b&0x02 != 0 {
					ev.inst = data[pos]
					pos++
				}
				if b&0x04 != 0 {
					ev.vol = data[pos]
					pos++
				}
				if b&0x08 != 0 {
					ev.effect = data[pos]
					pos++
				}
				if b&0x10 != 0 {
					ev.param = data[pos]
					pos++
				}
			} else {
				ev.note = b
				ev.inst = data[pos]
				ev.vol = data[pos+1]
				ev.effect = data[pos+2]
				ev.param = data[pos+3]
				pos += 4
			}
			pat.events[i] = ev
		}
		patterns[pi] = pat
		offset = patDataEnd
	}
	return patterns, offset, ""
}

func parseInstruments(data []byte, offset int, count int) ([]xmInstrument, string) {
	insts := make([]xmInstrument, count+1)
	for ii := 1; ii <= count; ii++ {
		instrSize := int(binary.LittleEndian.Uint32(data[offset:]))
		inst := xmInstrument{name: strings.TrimRight(string(data[offset+4:offset+26]), "\x00")}
		numSamples := int(binary.LittleEndian.Uint16(data[offset+27:]))
		sampleHdrSize := 0
		if numSamples > 0 {
			sampleHdrSize = int(binary.LittleEndian.Uint32(data[offset+29:]))
			copy(inst.sampleMap[:], data[offset+33:offset+129])
			inst.volEnv = parseEnvelope(data[offset+129:offset+177], int(data[offset+225]), data[offset+233], data[offset+227], data[offset+228], data[offset+229])
			inst.panEnv = parseEnvelope(data[offset+177:offset+225], int(data[offset+226]), data[offset+234], data[offset+230], data[offset+231], data[offset+232])
			inst.vibratoType = data[offset+235]
			inst.vibratoSweep = data[offset+236]
			inst.vibratoDepth = data[offset+237]
			inst.vibratoRate = data[offset+238]
			inst.fadeout = int(binary.LittleEndian.Uint16(data[offset+239:]))
		}
		sampleHeadersOff := offset + instrSize
		dataOff := sampleHeadersOff + numSamples*sampleHdrSize
		inst.samples = make([]xmSample, numSamples)
		for si := 0; si < numSamples; si++ {
			sh := sampleHeadersOff + si*sampleHdrSize
			length := int(binary.LittleEndian.Uint32(data[sh:]))
			loopStart := int(binary.LittleEndian.Uint32(data[sh+4:]))
			loopLen := int(binary.LittleEndian.Uint32(data[sh+8:]))
			volume := int(data[sh+12])
			finetune := int8(data[sh+13])
			typ := data[sh+14]
			pan := int(data[sh+15])
			rel := int8(data[sh+16])
			name := strings.TrimRight(string(data[sh+18:sh+40]), "\x00")
			smp := xmSample{
				length:    length,
				loopStart: loopStart,
				loopLen:   loopLen,
				loopType:  int(typ & 0x03),
				is16:      typ&0x10 != 0,
				volume:    clampInt(volume, 0, 64),
				finetune:  finetune,
				panning:   clampInt(pan, 0, 255),
				relNote:   rel,
				name:      name,
			}
			raw := data[dataOff : dataOff+length]
			if smp.is16 {
				smp.data = decodeDelta16(raw)
				smp.length = len(smp.data)
				smp.loopStart /= 2
				smp.loopLen /= 2
			} else {
				smp.data = decodeDelta8(raw)
			}
			if smp.loopStart > smp.length {
				smp.loopStart = smp.length
			}
			if smp.loopStart+smp.loopLen > smp.length {
				smp.loopLen = smp.length - smp.loopStart
			}
			inst.samples[si] = smp
			dataOff += length
		}
		insts[ii] = inst
		offset = dataOff
	}
	return insts, ""
}

func (p *Player) Init(tune []byte, sampleRate int) string {
	p.initialised = false
	p.data = tune
	if err := Validate(tune); err != "" {
		return err
	}
	layout, ok := detectHeader(tune)
	if !ok {
		return "Not a valid FastTracker 2 XM file."
	}
	p.sampleRate = sampleRate
	p.module = layout
	p.linearFreq = layout.Flags&1 != 0
	p.title = strings.TrimRight(string(tune[17:37]), "\x00")
	p.tracker = strings.TrimRight(string(tune[38:58]), "\x00")
	patterns, offset, err := unpackPatterns(tune, layout)
	if err != "" {
		return err
	}
	insts, err := parseInstruments(tune, offset, layout.InstrumentCount)
	if err != "" {
		return err
	}
	p.patterns = patterns
	p.instruments = insts
	p.channels = make([]xmChannel, layout.Channels)
	for i := range p.channels {
		p.channels[i].pan = 128
		p.channels[i].fadeoutVol = 32768
		p.channels[i].sampleDir = 1
		p.channels[i].cutTick = -1
		p.channels[i].keyoffTick = -1
		p.channels[i].delayTick = -1
	}
	p.Stop()
	p.initialised = true
	return ""
}

func (p *Player) Stop() {
	for i := range p.channels {
		pan := p.channels[i].pan
		p.channels[i] = xmChannel{pan: pan, fadeoutVol: 65536, sampleDir: 1, cutTick: -1, keyoffTick: -1, delayTick: -1}
	}
	p.globalVol = 64
	p.pos = 0
	p.row = 0
	p.speed = p.module.Tempo
	if p.speed <= 0 {
		p.speed = 6
	}
	p.bpm = p.module.BPM
	if p.bpm <= 0 {
		p.bpm = 125
	}
	p.tick = 0
	p.samCnt = -1
	p.samPerTick = p.calcSamPerTick()
	p.nextPos = -1
	p.nextRow = -1
	p.nextRowSame = false
	p.patDelay = 0
	p.repeating = false
	for i := range p.orderMap {
		p.orderMap[i] = 0
	}
	p.orderMap[0] = 1
}

func (p *Player) GetDescription() []byte {
	if p.title == "" {
		return nil
	}
	return []byte(p.title)
}

func (p *Player) calcSamPerTick() int {
	v := p.sampleRate * 5 / (p.bpm * 2)
	if v < 1 {
		v = 1
	}
	return v
}

func (p *Player) Sample(left, right *int16) bool {
	if p.samCnt < 0 {
		if p.tick == 0 {
			p.processRow()
		} else {
			p.processTick()
		}
		for i := range p.channels {
			p.advanceChannelTick(&p.channels[i])
		}
		p.samPerTick = p.calcSamPerTick()
		p.samCnt = 0
		*left = 0
		*right = 0
		return p.repeating
	}
	if p.samCnt == 0 {
		if p.tick == 0 {
			p.processRow()
		} else {
			p.processTick()
		}
		for i := range p.channels {
			p.advanceChannelTick(&p.channels[i])
		}
		p.samPerTick = p.calcSamPerTick()
	}

	var lAcc, rAcc int64
	for i := range p.channels {
		ch := &p.channels[i]
		if !ch.active || ch.sample == nil || len(ch.sample.data) == 0 {
			continue
		}
		step := pitchToStep(ch.playPitch, p.sampleRate)
		if step <= 0 {
			continue
		}
		s := ch.sample
		pos := ch.samplePos
		i0 := int(pos)
		if i0 < 0 || i0 >= s.length {
			ch.active = false
			continue
		}
		i1 := i0 + int(ch.sampleDir)
		if s.loopType == 2 {
			if i1 >= s.loopStart+s.loopLen {
				i1 = s.loopStart + s.loopLen - 2
			}
			if i1 < s.loopStart {
				i1 = s.loopStart + 1
			}
		} else if s.loopType == 1 {
			if i1 >= s.loopStart+s.loopLen {
				i1 = s.loopStart
			}
		} else if i1 >= s.length {
			i1 = i0
		}
		frac := pos - float64(i0)
		s0 := float64(s.data[i0])
		s1 := s0
		if i1 >= 0 && i1 < s.length {
			s1 = float64(s.data[i1])
		}
		mix := s0*(1-frac) + s1*frac
		volMul := float64(ch.volume) / 64.0
		if ch.inst != nil {
			volMul *= float64(envelopeValue(ch.inst.volEnv, ch.volEnvPos)) / 64.0
		}
		volMul *= float64(ch.fadeoutVol) / 32768.0
		volMul *= float64(p.globalVol) / 64.0
		pan := float64(ch.pan)
		if ch.inst != nil && ch.inst.panEnv.enabled {
			ep := envelopeValue(ch.inst.panEnv, ch.panEnvPos)
			pan = clampPan(ch.pan + (ep-32)*4)
		}
		// Equal-power panning matching libxm: sqrt((MAX_PANNING-pan)/MAX_PANNING), MAX_PANNING=256
		lVol := math.Sqrt((256.0 - pan) / 256.0)
		rVol := math.Sqrt(pan / 256.0)
		// AMPLIFICATION = 0.25 (matches libxm), also normalize 8-bit-shifted
		// samples from int16 range to float: divide by 32768
		scaledF := mix * volMul * 0.25 / 32768.0
		lAcc += int64(scaledF * lVol * 32767.0)
		rAcc += int64(scaledF * rVol * 32767.0)
		ch.samplePos += step * ch.sampleDir
		p.wrapSample(ch)
	}
	l := clamp32(lAcc)
	r := clamp32(rAcc)
	*left = int16(l)
	*right = int16(r)
	p.samCnt++
	if p.samCnt >= p.samPerTick {
		p.samCnt = 0
		p.tick++
		if p.tick >= p.speed+p.patDelay*p.speed {
			p.tick = 0
			p.patDelay = 0
			p.advanceRow()
		}
	}
	return p.repeating
}

func clampPan(v int) float64 {
	if v < 0 {
		return 0
	}
	if v > 255 {
		return 255
	}
	return float64(v)
}

func clamp32(v int64) int32 {
	if v > 32767 {
		return 32767
	}
	if v < -32768 {
		return -32768
	}
	return int32(v)
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func (p *Player) wrapSample(ch *xmChannel) {
	if ch.sample == nil {
		return
	}
	s := ch.sample
	if s.loopType == 0 {
		if ch.samplePos >= float64(s.length) || ch.samplePos < 0 {
			ch.active = false
		}
		return
	}
	if s.loopLen <= 1 {
		return
	}
	start := float64(s.loopStart)
	end := float64(s.loopStart + s.loopLen)
	if s.loopType == 1 {
		for ch.samplePos >= end {
			ch.samplePos -= float64(s.loopLen)
		}
	} else if s.loopType == 2 {
		if ch.sampleDir > 0 && ch.samplePos >= end {
			ch.sampleDir = -1
			ch.samplePos = end - 1
		} else if ch.sampleDir < 0 && ch.samplePos < start {
			ch.sampleDir = 1
			ch.samplePos = start
		}
	}
}

func (p *Player) processRow() {
	if p.pos >= p.module.SongLength {
		return
	}
	ord := p.module.Orders[p.pos]
	if ord == 0xFF || int(ord) >= len(p.patterns) {
		return
	}
	pat := p.patterns[ord]
	if p.row >= pat.rows {
		return
	}
	for ci := range p.channels {
		ch := &p.channels[ci]
		ev := pat.events[p.row*p.module.Channels+ci]
		ch.playPitch = ch.basePitch
		ch.cutTick = -1
		ch.keyoffTick = -1
		ch.delayTick = -1
		ch.delayed.active = false
		p.applyVolumeColumn(ch, ev.vol, true)
		if ev.note == 97 {
			p.keyOff(ch)
		}
		if ev.effect == 0x0E && (ev.param>>4) == 0x0D && (ev.param&0x0F) > 0 {
			ch.delayTick = int(ev.param & 0x0F)
			ch.delayed = delayedEvent{active: true, event: ev}
			p.applyEffect(ch, ev, true, true)
			continue
		}
		p.triggerEvent(ch, ev, false)
		p.applyEffect(ch, ev, true, false)
	}
}

func (p *Player) processTick() {
	if p.pos >= p.module.SongLength {
		return
	}
	ord := p.module.Orders[p.pos]
	if ord == 0xFF || int(ord) >= len(p.patterns) {
		return
	}
	pat := p.patterns[ord]
	if p.row >= pat.rows {
		return
	}
	for ci := range p.channels {
		ch := &p.channels[ci]
		ev := pat.events[p.row*p.module.Channels+ci]
		ch.playPitch = ch.basePitch
		p.applyVolumeColumn(ch, ev.vol, false)
		p.applyEffect(ch, ev, false, false)
		if ch.delayTick >= 0 && p.tick == ch.delayTick && ch.delayed.active {
			p.triggerEvent(ch, ch.delayed.event, true)
		}
	}
}

func (p *Player) triggerEvent(ch *xmChannel, ev xmEvent, delayed bool) {
	if ev.inst > 0 && int(ev.inst) < len(p.instruments) {
		ch.instIndex = int(ev.inst)
		ch.inst = &p.instruments[ch.instIndex]
	}
	if ev.note == 0 || ev.note == 97 {
		if ev.inst > 0 && ch.sample != nil {
			ch.baseVolume = ch.sample.volume
			ch.volume = ch.baseVolume
			ch.pan = ch.sample.panning
		}
		return
	}
	if ch.inst == nil {
		return
	}
	noteIdx := int(ev.note) - 1
	if noteIdx < 0 {
		return
	}
	mapIdx := noteIdx
	if mapIdx > 95 {
		mapIdx = 95
	}
	smpIdx := int(ch.inst.sampleMap[mapIdx])
	if smpIdx >= len(ch.inst.samples) {
		smpIdx = len(ch.inst.samples) - 1
	}
	if smpIdx < 0 || len(ch.inst.samples) == 0 {
		return
	}
	smp := &ch.inst.samples[smpIdx]
	if ev.effect == 0x03 || ev.effect == 0x05 || ev.vol >= 0xF0 {
		ch.targetPitch = float64(noteIdx) + float64(smp.relNote) + float64(smp.finetune)/128.0
		if ev.effect == 0x03 && ev.param != 0 {
			ch.tonePortaMem = ev.param
		}
		return
	}
	ch.sampleIndex = smpIdx
	ch.sample = smp
	ch.note = noteIdx
	ch.basePitch = float64(noteIdx) + float64(smp.relNote) + float64(smp.finetune)/128.0
	ch.playPitch = ch.basePitch
	ch.targetPitch = ch.basePitch
	ch.samplePos = 0
	if ev.effect == 0x09 || delayed {
		off := int(ch.sampleOffMem) * 256
		if ev.param != 0 {
			off = int(ev.param) * 256
		}
		if smp.is16 {
			off /= 2
		}
		if off < smp.length {
			ch.samplePos = float64(off)
		}
	}
	ch.sampleDir = 1
	ch.keyOn = true
	ch.active = true
	ch.baseVolume = smp.volume
	ch.volume = smp.volume
	ch.pan = smp.panning
	ch.fadeoutVol = 65536
	ch.volEnvPos = 0
	ch.panEnvPos = 0
	ch.autoVibPos = 0
}

func (p *Player) keyOff(ch *xmChannel) {
	ch.keyOn = false
}

func (p *Player) applyVolumeColumn(ch *xmChannel, vol uint8, tick0 bool) {
	if vol < 0x10 {
		return
	}
	switch {
	case vol <= 0x50:
		if tick0 {
			ch.volume = int(vol - 0x10)
			ch.baseVolume = ch.volume
		}
	case vol >= 0x60 && vol <= 0x6F:
		if !tick0 {
			ch.volume = clampInt(ch.volume-int(vol&0x0F), 0, 64)
		}
	case vol >= 0x70 && vol <= 0x7F:
		if !tick0 {
			ch.volume = clampInt(ch.volume+int(vol&0x0F), 0, 64)
		}
	case vol >= 0x80 && vol <= 0x8F:
		if tick0 {
			ch.volume = clampInt(ch.volume-int(vol&0x0F), 0, 64)
		}
	case vol >= 0x90 && vol <= 0x9F:
		if tick0 {
			ch.volume = clampInt(ch.volume+int(vol&0x0F), 0, 64)
		}
	case vol >= 0xA0 && vol <= 0xAF:
		if tick0 {
			ch.vibratoSpd = (vol & 0x0F) << 2
		}
	case vol >= 0xB0 && vol <= 0xBF:
		if !tick0 {
			p.doVibrato(ch, 0, vol&0x0F)
		}
	case vol >= 0xC0 && vol <= 0xCF:
		if tick0 {
			ch.pan = int((vol & 0x0F) * 17)
		}
	case vol >= 0xD0 && vol <= 0xDF:
		if !tick0 {
			ch.pan = clampInt(ch.pan-int(vol&0x0F), 0, 255)
		}
	case vol >= 0xE0 && vol <= 0xEF:
		if !tick0 {
			ch.pan = clampInt(ch.pan+int(vol&0x0F), 0, 255)
		}
	case vol >= 0xF0:
		if !tick0 {
			p.doTonePorta(ch, vol&0x0F)
		}
	}
}

func (p *Player) applyEffect(ch *xmChannel, ev xmEvent, tick0 bool, onlySetup bool) {
	x := ev.param >> 4
	y := ev.param & 0x0F
	switch ev.effect {
	case 0x00:
		if tick0 {
			ch.arpX, ch.arpY = x, y
		} else if ev.param != 0 {
			step := p.tick % 3
			pitch := ch.basePitch
			if step == 1 {
				pitch += float64(ch.arpX)
			}
			if step == 2 {
				pitch += float64(ch.arpY)
			}
			ch.playPitch = pitch
		}
	case 0x01:
		if ev.param != 0 {
			ch.portaUpMem = ev.param
		}
		if !tick0 {
			ch.basePitch += float64(ch.portaUpMem) / 16.0
			ch.playPitch = ch.basePitch
		}
	case 0x02:
		if ev.param != 0 {
			ch.portaDownMem = ev.param
		}
		if !tick0 {
			ch.basePitch -= float64(ch.portaDownMem) / 16.0
			ch.playPitch = ch.basePitch
		}
	case 0x03:
		if ev.param != 0 {
			ch.tonePortaMem = ev.param
		}
		if !tick0 {
			p.doTonePorta(ch, ch.tonePortaMem)
		}
	case 0x04:
		if x != 0 {
			ch.vibratoSpd = x << 2
		}
		if y != 0 {
			ch.vibratoDepth = y
		}
		if !tick0 {
			p.doVibrato(ch, ch.vibratoSpd, ch.vibratoDepth)
		}
	case 0x05:
		if !tick0 {
			p.doTonePorta(ch, ch.tonePortaMem)
			p.doVolSlide(ch, ev.param)
		}
	case 0x06:
		if !tick0 {
			p.doVibrato(ch, ch.vibratoSpd, ch.vibratoDepth)
			p.doVolSlide(ch, ev.param)
		}
	case 0x07:
		if x != 0 {
			ch.tremoloSpd = x << 2
		}
		if y != 0 {
			ch.tremoloDepth = y
		}
		if !tick0 {
			d := int(evalWaveform(ch.tremoloWave, ch.tremoloPhase) * float64(ch.tremoloDepth))
			ch.volume = clampInt(ch.baseVolume+d, 0, 64)
			ch.tremoloPhase += int(ch.tremoloSpd)
		}
	case 0x08:
		if tick0 {
			ch.pan = int(ev.param)
		}
	case 0x09:
		if tick0 && ev.param != 0 {
			ch.sampleOffMem = ev.param
		}
	case 0x0A:
		if !tick0 {
			p.doVolSlide(ch, ev.param)
		}
	case 0x0B:
		if tick0 {
			p.nextPos = int(ev.param)
			p.nextRow = 0
			p.nextRowSame = false
		}
	case 0x0C:
		if tick0 {
			ch.baseVolume = clampInt(int(ev.param), 0, 64)
			ch.volume = ch.baseVolume
		}
	case 0x0D:
		if tick0 {
			p.nextRow = int(x)*10 + int(y)
			if p.nextRow > 255 {
				p.nextRow = 0
			}
			p.nextRowSame = false
		}
	case 0x0E:
		p.applyExtended(ch, x, y, tick0)
	case 0x0F:
		if tick0 {
			if ev.param == 0 {
				break
			}
			if ev.param < 32 {
				p.speed = int(ev.param)
			} else {
				p.bpm = int(ev.param)
			}
		}
	case 0x10:
		if tick0 {
			p.globalVol = clampInt(int(ev.param), 0, 64)
		}
	case 0x11:
		if ev.param != 0 {
			ch.globVolMem = int(int8(ev.param))
		}
		if !tick0 {
			p.globalVol = clampInt(p.globalVol+int(int8(ev.param)), 0, 64)
		}
	case 0x14:
		if !tick0 {
			ch.basePitch += float64(ch.portaUpMem) / 64.0
			ch.playPitch = ch.basePitch
		}
	case 0x15:
		if !tick0 {
			p.doVibrato(ch, ch.vibratoSpd, ch.vibratoDepth)
		}
	case 0x19:
		if !tick0 {
			if p.tick == int(ev.param) {
				p.keyOff(ch)
			}
		}
	case 0x1B:
		if tick0 {
			ch.volEnvPos = int(ev.param)
			ch.panEnvPos = int(ev.param)
		}
	case 0x1D:
		if !tick0 {
			p.doPanSlide(ch, ev.param)
		}
	case 0x1B + 5: // 0x20 T tremor
		if x != 0 || y != 0 {
			ch.tremorOn, ch.tremorOff = int(x), int(y)
		}
		if !tick0 && ch.tremorOn+ch.tremorOff > 0 {
			span := ch.tremorOn + ch.tremorOff
			pos := ch.tremorPos % span
			if pos >= ch.tremorOn {
				ch.volume = 0
			} else {
				ch.volume = ch.baseVolume
			}
			ch.tremorPos++
		}
	case 0x21:
		if x == 1 && !tick0 {
			ch.basePitch += float64(y) / 64.0
			ch.playPitch = ch.basePitch
		}
		if x == 2 && !tick0 {
			ch.basePitch -= float64(y) / 64.0
			ch.playPitch = ch.basePitch
		}
	}
	if onlySetup {
		return
	}
}

func (p *Player) applyExtended(ch *xmChannel, x, y uint8, tick0 bool) {
	switch x {
	case 0x1:
		if tick0 {
			ch.basePitch += float64(y) / 64.0
			ch.playPitch = ch.basePitch
		}
	case 0x2:
		if tick0 {
			ch.basePitch -= float64(y) / 64.0
			ch.playPitch = ch.basePitch
		}
	case 0x4:
		if tick0 {
			ch.vibratoWave = y
		}
	case 0x5:
		if tick0 && ch.sample != nil {
			ch.basePitch = float64(ch.note) + float64(ch.sample.relNote) + float64(int8(y<<4))/128.0
			ch.playPitch = ch.basePitch
		}
	case 0x6:
		if tick0 {
			if y == 0 {
				p.nextRow = p.row
				p.nextRowSame = true
			} else {
				p.nextRow = max(0, p.row-int(y))
				p.nextRowSame = true
			}
		}
	case 0x9:
		if tick0 {
			ch.retrigTicks = int(y)
			ch.retrigCount = 0
		}
		if !tick0 && ch.retrigTicks > 0 {
			ch.retrigCount++
			if ch.retrigCount >= ch.retrigTicks {
				ch.retrigCount = 0
				ch.samplePos = 0
			}
		}
	case 0xA:
		if tick0 {
			ch.baseVolume = clampInt(ch.baseVolume+int(y), 0, 64)
			ch.volume = ch.baseVolume
		}
	case 0xB:
		if tick0 {
			ch.baseVolume = clampInt(ch.baseVolume-int(y), 0, 64)
			ch.volume = ch.baseVolume
		}
	case 0xC:
		if tick0 {
			ch.cutTick = int(y)
			if y == 0 {
				ch.baseVolume = 0
				ch.volume = 0
			}
		}
		if !tick0 && p.tick == ch.cutTick {
			ch.baseVolume = 0
			ch.volume = 0
		}
	case 0xD:
		if tick0 {
			ch.delayTick = int(y)
		}
	case 0xE:
		if tick0 {
			p.patDelay = int(y)
		}
	}
}

func (p *Player) doTonePorta(ch *xmChannel, speed uint8) {
	step := float64(speed) / 16.0
	if step <= 0 {
		return
	}
	if ch.basePitch < ch.targetPitch {
		ch.basePitch += step
		if ch.basePitch > ch.targetPitch {
			ch.basePitch = ch.targetPitch
		}
	} else if ch.basePitch > ch.targetPitch {
		ch.basePitch -= step
		if ch.basePitch < ch.targetPitch {
			ch.basePitch = ch.targetPitch
		}
	}
	ch.playPitch = ch.basePitch
}

func (p *Player) doVibrato(ch *xmChannel, speed, depth uint8) {
	ch.vibratoPhase += int(speed)
	// libxm: "depth 8 == 2 semitones amplitude (-1 then +1)", i.e. ±1 semitone at depth 8
	d := evalWaveform(ch.vibratoWave, ch.vibratoPhase) * float64(depth) / 8.0
	ch.playPitch = ch.basePitch + d
}

func (p *Player) doVolSlide(ch *xmChannel, param uint8) {
	if param != 0 {
		ch.volSlideMem = param
	} else {
		param = ch.volSlideMem
	}
	x := int(param >> 4)
	y := int(param & 0x0F)
	// libxm: up-slide has precedence (matches FT2 behaviour for e.g. A1F)
	if x > 0 {
		ch.baseVolume = clampInt(ch.baseVolume+x, 0, 64)
	} else if y > 0 {
		ch.baseVolume = clampInt(ch.baseVolume-y, 0, 64)
	}
	ch.volume = ch.baseVolume
}

func (p *Player) doPanSlide(ch *xmChannel, param uint8) {
	if param != 0 {
		ch.panSlideMem = param
	} else {
		param = ch.panSlideMem
	}
	x := int(param >> 4)
	y := int(param & 0x0F)
	if x > 0 && y == 0 {
		ch.pan = clampInt(ch.pan+x, 0, 255)
	}
	if y > 0 && x == 0 {
		ch.pan = clampInt(ch.pan-y, 0, 255)
	}
}

func (p *Player) advanceChannelTick(ch *xmChannel) {
	if ch.inst == nil {
		return
	}
	if ch.inst.volEnv.enabled {
		ch.volEnvPos = advanceEnvelope(ch.inst.volEnv, ch.volEnvPos, ch.keyOn)
	}
	if ch.inst.panEnv.enabled {
		ch.panEnvPos = advanceEnvelope(ch.inst.panEnv, ch.panEnvPos, ch.keyOn)
	}
	if !ch.keyOn {
		fade := ch.inst.fadeout
		if fade <= 0 {
			fade = 256
		}
		ch.fadeoutVol -= fade
		if ch.fadeoutVol < 0 {
			ch.fadeoutVol = 0
		}
		if ch.fadeoutVol == 0 {
			ch.active = false
		}
	}
	if ch.sample != nil && ch.inst.vibratoDepth > 0 && ch.inst.vibratoRate > 0 {
		// libxm: "autovibrato_depth of 8 is the same as 4x1 (vibrato depth 1)"
		// vibrato depth 1 = 1/8 semitone peak → autovibrato_depth 8 = 1/8 semitone → divisor 64
		depth := float64(ch.inst.vibratoDepth) / 64.0
		if ch.inst.vibratoSweep > 0 && ch.autoVibPos < int(ch.inst.vibratoSweep) {
			depth *= float64(ch.autoVibPos) / float64(ch.inst.vibratoSweep)
		}
		ch.autoVibPos++
		// libxm: "autovibrato_depth 8 = same as 4x1 (vibrato depth 1)" → divisor 64
		ch.playPitch += evalWaveform(ch.inst.vibratoType, ch.autoVibPos*int(ch.inst.vibratoRate)) * depth
	}
}

func (p *Player) advanceRow() {
	newPos := p.pos
	newRow := p.row + 1
	hadNextRow := p.nextRow >= 0
	wrapped := false
	if hadNextRow {
		newRow = p.nextRow
		if p.nextPos < 0 && !p.nextRowSame {
			newPos++
		}
		p.nextRow = -1
		p.nextRowSame = false
	}
	if p.nextPos >= 0 {
		newPos = p.nextPos
		p.nextPos = -1
		if !hadNextRow {
			newRow = 0
		}
	}
	if newPos >= p.module.SongLength || (newPos < len(p.module.Orders) && p.module.Orders[newPos] == 0xFF) {
		wrapped = true
		newPos = p.module.Restart
		if newPos >= p.module.SongLength {
			newPos = 0
		}
		newRow = 0
	}
	if newPos < len(p.module.Orders) {
		ord := p.module.Orders[newPos]
		if ord != 0xFF && int(ord) < len(p.patterns) && newRow >= p.patterns[ord].rows {
			newRow = 0
			newPos++
			if newPos >= p.module.SongLength {
				wrapped = true
				newPos = p.module.Restart
				newRow = 0
			}
		}
	}
	if wrapped {
		p.repeating = true
	}
	if !p.repeating && newRow == 0 {
		idx := newPos >> 5
		bit := uint32(1) << (uint(newPos) & 31)
		if p.orderMap[idx]&bit != 0 {
			p.repeating = true
		} else {
			p.orderMap[idx] |= bit
		}
	}
	p.pos = newPos
	p.row = newRow
}
