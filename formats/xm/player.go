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
	volEnvValue float64
	panEnvValue float64

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
	tremorOn     bool
	tremorTicks  int
	tremorParam  uint8
	retrigParam  uint8
	retrigTicks  int
	delayTick    int
	delayed      delayedEvent
	autoVibPos   int

	patternLoopOrigin int
	patternLoopCount  int
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

var sinLUT = [16]int{0, 12, 24, 37, 48, 60, 71, 81, 90, 98, 106, 112, 118, 122, 125, 127}

func evalWaveform(waveform uint8, step int) int {
	s := uint8(step)
	waveform &= 127
	switch waveform {
	case 0: // Sine (LUT-based, matching libxm)
		q := s >> 2
		var idx uint8
		if q&0x10 != 0 {
			idx = 0x0F - (q & 0x0F)
		} else {
			idx = q & 0x0F
		}
		if q < 0x20 {
			return -sinLUT[idx]
		}
		return sinLUT[idx]
	case 1: // Ramp down
		return int(int8(^s))
	case 2: // Square
		if s < 0x80 {
			return -128
		}
		return 127
	default:
		return 0
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
	if !env.enabled || len(env.points) < 2 {
		return pos
	}
	if env.loopEnabled && env.loopEnd >= 0 && env.loopEnd < len(env.points) &&
		env.loopStart >= 0 && env.loopStart < len(env.points) {
		endFrame := env.points[env.loopEnd].frame
		if pos == endFrame {
			if !keyOn || !env.sustainEnabled || env.sustainPoint != env.loopEnd {
				pos = env.points[env.loopStart].frame
			}
		}
	}
	if keyOn && env.sustainEnabled && env.sustainPoint >= 0 && env.sustainPoint < len(env.points) {
		sf := env.points[env.sustainPoint].frame
		if pos == sf {
			return pos
		}
	}
	return pos + 1
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
	var acc int16
	for i := 0; i < count; i++ {
		d := int16(binary.LittleEndian.Uint16(data[i*2:]))
		acc += d
		out[i] = acc
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
			rawFinetune := int8(data[sh+13])
		finetune := int8((int(rawFinetune) + 128) / 8 - 16)
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
		p.channels[i].fadeoutVol = 32767
		p.channels[i].sampleDir = 1
		p.channels[i].delayTick = -1
		p.channels[i].volEnvValue = 1.0
		p.channels[i].panEnvValue = 32.0
	}
	p.Stop()
	p.initialised = true
	return ""
}

func (p *Player) Stop() {
	for i := range p.channels {
		pan := p.channels[i].pan
		p.channels[i] = xmChannel{pan: pan, fadeoutVol: 32767, sampleDir: 1, delayTick: -1, volEnvValue: 1.0, panEnvValue: 32.0}
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

		effLen := s.length
		if s.loopType != 0 && s.loopLen > 0 {
			effLen = s.loopStart + s.loopLen
			if effLen > s.length {
				effLen = s.length
			}
		}

		i0 := int(pos)
		frac := pos - float64(i0)
		var i1 int

		if s.loopType == 2 && i0 >= effLen {
			i0 = effLen*2 - 1 - i0
			if i0 < s.loopStart {
				i0 = s.loopStart
			}
			i1 = i0 - 1
			if i1 < s.loopStart {
				i1 = i0
			}
		} else {
			if i0 < 0 || i0 >= effLen {
				ch.active = false
				continue
			}
			i1 = i0 + 1
			if s.loopType == 2 && i1 >= effLen {
				i1 = i0
			} else if s.loopType == 1 && i1 >= effLen {
				i1 = s.loopStart
			} else if i1 >= s.length {
				i1 = i0
			}
		}

		s0 := float64(s.data[i0])
		s1 := s0
		if i1 >= 0 && i1 < s.length {
			s1 = float64(s.data[i1])
		}
		mix := s0*(1-frac) + s1*frac
		volMul := float64(ch.volume) / 64.0
		volMul *= ch.volEnvValue
		volMul *= float64(ch.fadeoutVol) / 32767.0
		volMul *= float64(p.globalVol) / 64.0
		pan := ch.pan
		if ch.inst != nil && ch.inst.panEnv.enabled {
			ep := int(ch.panEnvValue)
			pan += (ep - 32) * (128 - abs(pan-128)) / 32
			if pan < 0 {
				pan = 0
			} else if pan > 255 {
				pan = 255
			}
		}
		lVol := math.Sqrt(float64(256-pan) / 256.0)
		rVol := math.Sqrt(float64(pan) / 256.0)
		// AMPLIFICATION = 0.25 (matches libxm), also normalize 8-bit-shifted
		// samples from int16 range to float: divide by 32768
		scaledF := mix * volMul * 0.25 / 32768.0
		lAcc += int64(scaledF * lVol * 32767.0)
		rAcc += int64(scaledF * rVol * 32767.0)
		ch.samplePos += step
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

func abs(a int) int {
	if a < 0 {
		return -a
	}
	return a
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
	end := float64(s.loopStart + s.loopLen)
	if ch.samplePos >= end {
		off := float64(s.loopStart)
		ch.samplePos -= off
		if s.loopType == 2 {
			cycle := float64(s.loopLen * 2)
			ch.samplePos = math.Mod(ch.samplePos, cycle)
			if ch.samplePos < 0 {
				ch.samplePos += cycle
			}
		} else {
			ch.samplePos = math.Mod(ch.samplePos, float64(s.loopLen))
			if ch.samplePos < 0 {
				ch.samplePos += float64(s.loopLen)
			}
		}
		ch.samplePos += off
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
		ch.delayTick = -1
		ch.delayed.active = false
		if ev.effect == 0x0E && (ev.param>>4) == 0x0D && (ev.param&0x0F) > 0 {
			ch.delayTick = int(ev.param & 0x0F)
			ch.delayed = delayedEvent{active: true, event: ev}
			p.applyEffect(ch, ev, true, true)
			continue
		}
		p.triggerEvent(ch, ev)
		p.applyVolumeColumn(ch, ev.vol, true)
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
			p.triggerEvent(ch, ch.delayed.event)
		}
	}
}

func (p *Player) triggerEvent(ch *xmChannel, ev xmEvent) {
	if ev.inst > 0 && int(ev.inst) < len(p.instruments) {
		ch.instIndex = int(ev.inst)
		ch.inst = &p.instruments[ch.instIndex]
	}

	if ev.note > 0 && ev.note < 97 {
		noteIdx := int(ev.note) - 1
		isTonePorta := ev.effect == 0x03 || ev.effect == 0x05 || ev.vol >= 0xF0
		if isTonePorta {
			if ch.sample != nil {
				ch.targetPitch = float64(noteIdx) + float64(ch.sample.relNote) + float64(ch.sample.finetune)/16.0
			}
		} else if ch.inst != nil {
			p.triggerNote(ch, ev, noteIdx)
		}
	} else if ev.note == 97 {
		p.keyOff(ch)
	}

	if ev.inst > 0 {
		if ch.sample != nil {
			ch.baseVolume = ch.sample.volume
			ch.volume = ch.baseVolume
			ch.pan = ch.sample.panning
		}
		if ev.note != 97 {
			p.triggerInstrument(ch)
		}
	}
}

func (p *Player) triggerNote(ch *xmChannel, ev xmEvent, noteIdx int) {
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

	noteVal := noteIdx + int(smp.relNote)
	if noteVal < 0 || noteVal >= 120 {
		return
	}

	ch.sampleIndex = smpIdx
	ch.sample = smp
	ch.note = noteIdx
	if ev.effect == 0x0E && (ev.param>>4) == 0x5 {
		finetune := float64(int(ev.param&0x0F)*2-16) / 16.0
		ch.basePitch = float64(noteIdx) + float64(smp.relNote) + finetune
	} else {
		ch.basePitch = float64(noteIdx) + float64(smp.relNote) + float64(smp.finetune)/16.0
	}
	ch.playPitch = ch.basePitch
	ch.targetPitch = ch.basePitch
	ch.samplePos = 0
	ch.sampleDir = 1
	ch.active = true

	if ev.effect == 0x09 {
		if ev.param != 0 {
			ch.sampleOffMem = ev.param
		}
		off := int(ch.sampleOffMem) * 256
		if smp.is16 {
			off /= 2
		}
		if off < smp.length {
			ch.samplePos = float64(off)
		}
	}
}

func (p *Player) triggerInstrument(ch *xmChannel) {
	ch.keyOn = true
	ch.fadeoutVol = 32767
	ch.volEnvPos = 0
	ch.panEnvPos = 0
	ch.autoVibPos = 0
	ch.retrigTicks = 0
	ch.tremorTicks = 0

	if ch.vibratoWave&0x04 == 0 {
		ch.vibratoPhase = 0
	}
	if ch.tremoloWave&0x04 == 0 {
		ch.tremoloPhase = 0
	}

	if ch.inst != nil {
		if ch.inst.volEnv.enabled {
			ch.volEnvValue = float64(envelopeValue(ch.inst.volEnv, 0)) / 64.0
		} else {
			ch.volEnvValue = 1.0
		}
		if ch.inst.panEnv.enabled {
			ch.panEnvValue = float64(envelopeValue(ch.inst.panEnv, 0))
		} else {
			ch.panEnvValue = 32.0
		}
	}
}

func (p *Player) keyOff(ch *xmChannel) {
	ch.keyOn = false
	if ch.inst == nil || !ch.inst.volEnv.enabled || len(ch.inst.volEnv.points) == 0 {
		ch.volume = 0
		ch.baseVolume = 0
	}
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
			if vol&0x0F != 0 {
				ch.vibratoSpd = (vol & 0x0F) << 2
			}
		}
	case vol >= 0xB0 && vol <= 0xBF:
		if !tick0 {
			if vol&0x0F != 0 {
				ch.vibratoDepth = vol & 0x0F
			}
			p.doVibrato(ch, ch.vibratoSpd, ch.vibratoDepth)
		}
	case vol >= 0xC0 && vol <= 0xCF:
		if tick0 {
			ch.pan = int(vol&0x0F) << 4
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
		if tick0 {
			if vol&0x0F != 0 {
				ch.tonePortaMem = (vol & 0x0F) << 4
			}
		} else {
			p.doTonePorta(ch, ch.tonePortaMem)
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
			t := p.speed - p.tick
			if t%3 == 0 {
				ch.playPitch = ch.basePitch
			} else if t%3 == 2 {
				ch.playPitch = ch.basePitch + float64(ch.arpY)
			} else {
				ch.playPitch = ch.basePitch + float64(ch.arpX)
			}
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
			wv := evalWaveform(ch.tremoloWave, ch.tremoloPhase)
			offset := int(int16(wv) * int16(ch.tremoloDepth) * 4 / 128)
			ch.volume = clampInt(ch.baseVolume-offset, 0, 64)
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
			p.nextRow = int(ev.param)
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
	case 0x11: // H - Global volume slide
		if ev.param != 0 {
			ch.globVolMem = int(ev.param)
		}
		if !tick0 {
			param := uint8(ch.globVolMem)
			if param>>4 > 0 {
				p.globalVol = clampInt(p.globalVol+int(param>>4), 0, 64)
			} else {
				p.globalVol = clampInt(p.globalVol-int(param&0x0F), 0, 64)
			}
		}
	case 0x14: // K - Key off at tick y
		if !tick0 {
			if p.tick == int(ev.param) {
				p.keyOff(ch)
			}
		}
	case 0x15: // L - Set envelope position
		if tick0 {
			ch.volEnvPos = int(ev.param)
			ch.panEnvPos = int(ev.param)
		}
	case 0x19: // P - Panning slide
		if !tick0 {
			p.doPanSlide(ch, ev.param)
		}
	case 0x1B: // R - Multi retrig note
		if tick0 {
			rp := ev.param
			if rp&0x0F != 0 {
				ch.retrigParam = (ch.retrigParam & 0xF0) | (rp & 0x0F)
			}
			if rp&0xF0 != 0 {
				ch.retrigParam = (ch.retrigParam & 0x0F) | (rp & 0xF0)
			}
		}
		p.doMultiRetrig(ch, ch.retrigParam)
	case 0x1D: // T - Tremor
		if ev.param > 0 {
			ch.tremorParam = ev.param
		}
		if !tick0 {
			tp := ch.tremorParam
			if ch.tremorTicks == 0 {
				ch.tremorOn = !ch.tremorOn
				if ch.tremorOn {
					ch.tremorTicks = int(tp >> 4)
				} else {
					ch.tremorTicks = int(tp & 0x0F)
				}
			} else {
				ch.tremorTicks--
			}
			if !ch.tremorOn {
				ch.volume = 0
			} else {
				ch.volume = ch.baseVolume
			}
		}
	case 0x21: // X - Extra fine portamento
		if x == 1 && tick0 {
			ch.basePitch += float64(y) / 64.0
			ch.playPitch = ch.basePitch
		}
		if x == 2 && tick0 {
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
	case 0x1: // E1y - Fine portamento up
		if tick0 {
			ch.basePitch += float64(y) / 16.0
			ch.playPitch = ch.basePitch
		}
	case 0x2: // E2y - Fine portamento down
		if tick0 {
			ch.basePitch -= float64(y) / 16.0
			ch.playPitch = ch.basePitch
		}
	case 0x4: // E4y - Set vibrato waveform
		if tick0 {
			ch.vibratoWave = y
		}
	case 0x5: // E5y - Set finetune (handled inside triggerNote)
	case 0x7: // E7y - Set tremolo waveform
		if tick0 {
			ch.tremoloWave = y
		}
	case 0x6:
		if tick0 {
			if y == 0 {
				ch.patternLoopOrigin = p.row
			} else {
				if int(y) == ch.patternLoopCount {
					ch.patternLoopCount = 0
				} else {
					ch.patternLoopCount++
					p.nextPos = p.pos
					p.nextRow = ch.patternLoopOrigin
					p.nextRowSame = true
				}
			}
		}
	case 0x9: // E9y - Retrigger note (only on non-zero ticks, matching libxm)
		if !tick0 && y > 0 && p.tick%int(y) == 0 {
			p.triggerInstrument(ch)
			if ch.inst != nil && ch.note > 0 {
				p.triggerNote(ch, xmEvent{note: uint8(ch.note + 1), effect: 0xFF}, ch.note)
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
	case 0xC: // ECy - Note cut (only fires on non-zero ticks in libxm)
		if !tick0 && p.tick == int(y) {
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
	wv := evalWaveform(ch.vibratoWave, ch.vibratoPhase)
	offset := int16(wv) * int16(depth) / 0x10
	ch.playPitch = ch.basePitch + float64(offset)/64.0
	ch.vibratoPhase += int(speed)
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
	if param&0xF0 != 0 {
		ch.pan = clampInt(ch.pan+int(param>>4), 0, 255)
	} else {
		ch.pan = clampInt(ch.pan-int(param&0x0F), 0, 255)
	}
}

func (p *Player) doMultiRetrig(ch *xmChannel, param uint8) {
	y := param & 0x0F
	if y == 0 {
		return
	}
	ch.retrigTicks++
	if ch.retrigTicks < int(y) {
		return
	}
	ch.retrigTicks = 0

	ch.samplePos = 0
	ch.sampleDir = 1
	ch.active = ch.sample != nil

	var addTab = [16]int{0, 0, 0, 0, 0, 0, 0, 0, 0, 1, 2, 4, 8, 16, 0, 0}
	var mulTab = [16]int{1, 1, 1, 1, 1, 1, 2, 1, 1, 1, 1, 1, 1, 1, 3, 2}
	x := int(param >> 4)
	vol := ch.volume + addTab[x] - addTab[x^8]
	vol = vol * mulTab[x] / mulTab[x^8]
	if vol < 0 {
		vol = 0
	} else if vol > 64 {
		vol = 64
	}
	ch.volume = vol
	ch.baseVolume = vol
}

func (p *Player) advanceChannelTick(ch *xmChannel) {
	if ch.inst == nil {
		return
	}
	// libxm: read envelope value at current pos, THEN advance counter
	if ch.inst.volEnv.enabled {
		ch.volEnvValue = float64(envelopeValue(ch.inst.volEnv, ch.volEnvPos)) / 64.0
		ch.volEnvPos = advanceEnvelope(ch.inst.volEnv, ch.volEnvPos, ch.keyOn)
	} else {
		ch.volEnvValue = 1.0
	}
	if ch.inst.panEnv.enabled {
		ch.panEnvValue = float64(envelopeValue(ch.inst.panEnv, ch.panEnvPos))
		ch.panEnvPos = advanceEnvelope(ch.inst.panEnv, ch.panEnvPos, ch.keyOn)
	} else {
		ch.panEnvValue = 32.0
	}
	if !ch.keyOn {
		ch.fadeoutVol -= ch.inst.fadeout
		if ch.fadeoutVol < 0 {
			ch.fadeoutVol = 0
		}
	} else {
		ch.fadeoutVol = 32767
	}
	if ch.sample != nil && ch.inst.vibratoDepth > 0 && ch.inst.vibratoRate > 0 {
		step := int(uint8(uint16(ch.autoVibPos) * uint16(ch.inst.vibratoRate)))
		wv := evalWaveform(ch.inst.vibratoType, step)
		offset := int8(int(wv) * (-int(ch.inst.vibratoDepth)) / 128)
		if ch.inst.vibratoSweep > 0 && ch.autoVibPos < int(ch.inst.vibratoSweep) {
			offset = int8(int16(offset) * int16(ch.autoVibPos) / int16(ch.inst.vibratoSweep))
		}
		ch.autoVibPos++
		ch.playPitch += float64(offset) / 64.0
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
