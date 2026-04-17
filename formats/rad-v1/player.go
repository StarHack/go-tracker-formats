// player.go - RAD v1 player (OPL2, 9 channels, 2-op).
// Pure Go port of the RAD v1 replayer by Shayde/Reality (public domain).
package radv1

import "github.com/StarHack/go-tracker-formats/formats"

const (
	kChannels    = 9
	kTrackLines  = 64
	kInstruments = 127
	kTracksV1    = 32

	cmPortamentoUp  = 0x1
	cmPortamentoDwn = 0x2
	cmToneSlide     = 0x3
	cmToneVolSlide  = 0x5
	cmVolSlide      = 0xA
	cmSetVol        = 0xC
	cmJumpToLine    = 0xD
	cmSetSpeed      = 0xF
)

const (
	fKeyOn   = 1 << 0
	fKeyOff  = 1 << 1
	fKeyedOn = 1 << 2
)

var radNoteFreq = [12]uint16{0x16b, 0x181, 0x198, 0x1b0, 0x1ca, 0x1e5, 0x202, 0x220, 0x241, 0x263, 0x287, 0x2ae}

var radOpOffsets2 = [9][2]uint16{
	{0x003, 0x000}, {0x004, 0x001}, {0x005, 0x002},
	{0x00B, 0x008}, {0x00C, 0x009}, {0x00D, 0x00A},
	{0x013, 0x010}, {0x014, 0x011}, {0x015, 0x012},
}

// radAlgCarriers[alg][op]: true if the op output level is scaled by channel volume.
// In the v1 format operators[1] (index 1) is the output-level carrier for alg 0 (FM).
var radAlgCarriers = [2][2]bool{
	{false, true}, // alg 0: FM
	{true, true},  // alg 1: additive
}

type radInstrument struct {
	feedback  uint8
	algorithm uint8
	volume    uint8
	operators [2][5]uint8
}

type radEffects struct {
	portSlide      int8
	volSlide       int8
	toneSlideFreq  uint16
	toneSlideOct   uint8
	toneSlideSpeed uint8
	toneSlideDir   int8
}

type radChannel struct {
	lastInstrument uint8
	instrument     *radInstrument
	volume         uint8
	keyFlags       uint8
	currFreq       uint16
	currOctave     int8
	fx             radEffects
}

// Player plays RAD v1 tune files via an OPL2 register callback.
// It implements the formats.Tracker interface.
type Player struct {
	opl3        func(uint16, uint8)
	instruments [kInstruments]radInstrument
	channels    [kChannels]radChannel
	playTime    uint32
	orderMap    [4]uint32
	repeating   bool
	hertz       int16
	orderList   []byte
	tracks      [kTracksV1][]byte
	track       []byte
	initialised bool
	speed       uint8
	orderSize   uint8
	speedCnt    uint8
	order       uint8
	line        uint8
	entrances   int8
	masterVol   uint8
	lineJump    int8
	opl3Regs    [512]uint8
	description []byte
	// note scratch
	noteNum   int8
	octaveNum int8
	instNum   uint8
	effectNum uint8
	param     uint8
}

// compile-time interface check
var _ formats.Tracker = (*Player)(nil)

// Init prepares the player for a v1 tune. Returns "" on success, error string on failure.
func (p *Player) Init(tune []byte, oplFn func(uint16, uint8)) string {
	p.initialised = false
	p.opl3 = oplFn
	if len(tune) < 0x11 || tune[0x10] != 0x10 {
		p.hertz = -1
		return "Not a RAD v1 tune."
	}
	for i := range p.tracks {
		p.tracks[i] = nil
	}
	s := tune[0x11:]
	flags := s[0]
	s = s[1:]
	p.speed = flags & 0x1F
	p.hertz = 50
	if flags&0x40 != 0 {
		p.hertz = 18
	}
	// Description: present only when flags bit 7 is set.
	p.description = nil
	if flags&0x80 != 0 {
		start := s
		for len(s) > 0 && s[0] != 0 {
			s = s[1:]
		}
		p.description = start[:len(start)-len(s)]
		if len(s) > 0 {
			s = s[1:]
		}
	}
	for i := range p.instruments {
		p.instruments[i] = radInstrument{}
	}
	for {
		instNum := s[0]
		s = s[1:]
		if instNum == 0 {
			break
		}
		inst := &p.instruments[instNum-1]
		inst.algorithm = s[8] & 1
		inst.feedback = (s[8] >> 1) & 0x7
		inst.volume = 64
		inst.operators[0][0] = s[0]
		inst.operators[1][0] = s[1]
		inst.operators[0][1] = s[2]
		inst.operators[1][1] = s[3]
		inst.operators[0][2] = s[4]
		inst.operators[1][2] = s[5]
		inst.operators[0][3] = s[6]
		inst.operators[1][3] = s[7]
		inst.operators[0][4] = s[9]
		inst.operators[1][4] = s[10]
		s = s[11:]
	}
	p.orderSize = s[0]
	s = s[1:]
	p.orderList = s[:p.orderSize]
	s = s[p.orderSize:]
	for i := 0; i < kTracksV1; i++ {
		pos := int(s[0]) | (int(s[1]) << 8)
		s = s[2:]
		if pos != 0 {
			p.tracks[i] = tune[pos:]
		}
	}
	for i := range p.opl3Regs {
		p.opl3Regs[i] = 255
	}
	p.Stop()
	p.initialised = true
	return ""
}

// Stop halts all sound and resets the player to the beginning.
func (p *Player) Stop() {
	for reg := uint16(0x20); reg < 0xF6; reg++ {
		var val uint8
		if reg >= 0x60 && reg < 0xA0 {
			val = 0xFF
		}
		p.setOPL3(reg, val)
	}
	p.setOPL3(1, 0x20)
	p.setOPL3(8, 0)
	p.setOPL3(0xBD, 0)
	p.setOPL3(0x104, 0)
	p.setOPL3(0x105, 0) // OPL2 mode (no OPL3 enable)
	p.playTime = 0
	p.repeating = false
	for i := range p.orderMap {
		p.orderMap[i] = 0
	}
	p.speedCnt = 1
	p.order = 0
	p.track = p.getTrack()
	p.line = 0
	p.entrances = 0
	p.masterVol = 64
	p.lineJump = -1
	for i := range p.channels {
		ch := &p.channels[i]
		ch.lastInstrument = 0
		ch.instrument = nil
		ch.volume = 0
		ch.keyFlags = 0
	}
}

// Update advances playback by one tick. Returns true when the tune starts repeating.
func (p *Player) Update() bool {
	if !p.initialised {
		return false
	}
	p.playLine()
	for i := 0; i < kChannels; i++ {
		p.continueFX(i, &p.channels[i].fx)
	}
	p.playTime++
	return p.repeating
}

// GetHertz returns the required Update() call rate in Hz.
func (p *Player) GetHertz() int { return int(p.hertz) }

// GetDescription returns the raw embedded description bytes from the tune.
func (p *Player) GetDescription() []byte { return p.description }

func (p *Player) setOPL3(reg uint16, val uint8) {
	p.opl3Regs[reg] = val
	p.opl3(reg, val)
}

func (p *Player) getOPL3(reg uint16) uint8 { return p.opl3Regs[reg] }

func (p *Player) getTrack() []byte {
	if int(p.order) >= int(p.orderSize) {
		p.order = 0
	}
	trackNum := p.orderList[p.order]
	if trackNum&0x80 != 0 {
		p.order = trackNum & 0x7F
		trackNum = p.orderList[p.order] & 0x7F
	}
	if p.order < 128 {
		byteIdx := p.order >> 5
		bit := uint32(1) << (p.order & 31)
		if p.orderMap[byteIdx]&bit != 0 {
			p.repeating = true
		} else {
			p.orderMap[byteIdx] |= bit
		}
	}
	return p.tracks[trackNum]
}

func (p *Player) unpackNote(s []byte, lastInst *uint8) ([]byte, bool) {
	chanID := s[0]
	s = s[1:]
	p.instNum = 0
	p.effectNum = 0
	p.param = 0
	n := s[0]
	s = s[1:]
	note := n & 0x7F
	if n&0x80 != 0 {
		p.instNum = 16
	}
	n = s[0]
	s = s[1:]
	p.instNum |= n >> 4
	if p.instNum != 0 {
		*lastInst = p.instNum
	}
	p.effectNum = n & 0x0F
	if p.effectNum != 0 {
		p.param = s[0]
		s = s[1:]
	}
	p.noteNum = int8(note & 15)
	p.octaveNum = int8(note >> 4)
	return s, (chanID & 0x80) != 0
}

func (p *Player) playLine() {
	p.speedCnt--
	if p.speedCnt > 0 {
		return
	}
	p.speedCnt = p.speed
	for i := range p.channels {
		p.resetFX(&p.channels[i].fx)
	}
	p.lineJump = -1
	trk := p.track
	if trk != nil && (trk[0]&0x7F) <= p.line {
		lineID := trk[0]
		trk = trk[1:]
		for {
			chanNum := int(trk[0] & 15)
			ch := &p.channels[chanNum]
			var last bool
			trk, last = p.unpackNote(trk, &ch.lastInstrument)
			p.playNote(chanNum, p.noteNum, p.octaveNum, uint16(p.instNum), p.effectNum, p.param)
			if last {
				break
			}
		}
		if lineID&0x80 != 0 {
			trk = nil
		}
		p.track = trk
	}
	p.line++
	if int(p.line) >= kTrackLines || p.lineJump >= 0 {
		if p.lineJump >= 0 {
			p.line = uint8(p.lineJump)
		} else {
			p.line = 0
		}
		p.order++
		p.track = p.getTrack()
	}
}

func (p *Player) playNote(chanNum int, notenum, octave int8, instnum uint16, cmd, param uint8) {
	ch := &p.channels[chanNum]
	if p.entrances >= 8 {
		return
	}
	p.entrances++
	fx := &ch.fx

	if cmd == cmToneSlide {
		if notenum > 0 && notenum <= 12 {
			fx.toneSlideOct = uint8(octave)
			fx.toneSlideFreq = radNoteFreq[notenum-1]
		}
		if param != 0 {
			fx.toneSlideSpeed = param
		}
		p.getSlideDir(chanNum, fx)
		p.entrances--
		return
	}
	if instnum > 0 {
		inst := &p.instruments[instnum-1]
		ch.instrument = inst
		p.loadInstrument(chanNum)
		ch.keyFlags |= fKeyOff | fKeyOn
	}
	if notenum > 0 {
		if notenum == 15 {
			ch.keyFlags |= fKeyOff
		}
		if ch.instrument != nil {
			p.playNoteOPL2(chanNum, octave, notenum)
		}
	}
	switch cmd {
	case cmSetVol:
		p.setVolume(chanNum, param)
	case cmSetSpeed:
		p.speed = param
		p.speedCnt = param
	case cmPortamentoUp:
		fx.portSlide = int8(param)
	case cmPortamentoDwn:
		fx.portSlide = -int8(param)
	case cmToneVolSlide:
		val := int8(param)
		if val >= 50 {
			val = -(val - 50)
		}
		fx.volSlide = val
		if param != 0 {
			fx.toneSlideSpeed = param
		}
		p.getSlideDir(chanNum, fx)
	case cmVolSlide:
		val := int8(param)
		if val >= 50 {
			val = -(val - 50)
		}
		fx.volSlide = val
	case cmJumpToLine:
		if int(param) < kTrackLines {
			p.lineJump = int8(param)
		}
	}
	p.entrances--
}

func (p *Player) loadInstrument(chanNum int) {
	ch := &p.channels[chanNum]
	inst := ch.instrument
	if inst == nil {
		return
	}
	ch.volume = inst.volume
	algBit := uint8(0)
	if inst.algorithm == 1 {
		algBit = 1
	}
	// 0x30 = bits 4+5 = left+right speakers enabled (required by Opal emulator).
	p.setOPL3(0xC0+uint16(chanNum), 0x30|(inst.feedback<<1)|algBit)
	for i := 0; i < 2; i++ {
		op := inst.operators[i]
		reg := radOpOffsets2[chanNum][i]
		vol := uint16(^op[1]) & 0x3F
		if radAlgCarriers[inst.algorithm][i] {
			vol = vol * uint16(inst.volume) / 64
			vol = vol * uint16(p.masterVol) / 64
		}
		p.setOPL3(reg+0x20, op[0])
		p.setOPL3(reg+0x40, (op[1]&0xC0)|uint8((vol^0x3F)&0x3F))
		p.setOPL3(reg+0x60, op[2])
		p.setOPL3(reg+0x80, op[3])
		p.setOPL3(reg+0xE0, op[4])
	}
}

func (p *Player) playNoteOPL2(chanNum int, octave, note int8) {
	ch := &p.channels[chanNum]
	o2 := uint16(chanNum)
	if ch.keyFlags&fKeyOff != 0 {
		ch.keyFlags &^= fKeyOff | fKeyedOn
		p.setOPL3(0xB0+o2, p.getOPL3(0xB0+o2)&^0x20)
	}
	if note == 15 {
		return
	}
	freq := radNoteFreq[note-1]
	ch.currFreq = freq
	ch.currOctave = octave
	p.setOPL3(0xA0+o2, uint8(freq&0xFF))
	if ch.keyFlags&fKeyOn != 0 {
		ch.keyFlags = (ch.keyFlags &^ fKeyOn) | fKeyedOn
	}
	keyBit := uint8(0)
	if ch.keyFlags&fKeyedOn != 0 {
		keyBit = 0x20
	}
	p.setOPL3(0xB0+o2, uint8(freq>>8)|uint8(octave<<2)|keyBit)
}

func (p *Player) resetFX(fx *radEffects) {
	fx.portSlide = 0
	fx.volSlide = 0
	fx.toneSlideDir = 0
}

func (p *Player) continueFX(chanNum int, fx *radEffects) {
	ch := &p.channels[chanNum]
	if fx.portSlide != 0 {
		p.portamento(chanNum, fx, fx.portSlide, false)
	}
	if fx.volSlide != 0 {
		vol := int8(ch.volume)
		vol -= fx.volSlide
		if vol < 0 {
			vol = 0
		}
		p.setVolume(chanNum, uint8(vol))
	}
	if fx.toneSlideDir != 0 {
		p.portamento(chanNum, fx, fx.toneSlideDir, true)
	}
}

func (p *Player) setVolume(chanNum int, vol uint8) {
	ch := &p.channels[chanNum]
	if vol > 64 {
		vol = 64
	}
	ch.volume = vol
	scaledVol := uint16(vol) * uint16(p.masterVol) / 64
	inst := ch.instrument
	if inst == nil {
		return
	}
	for i := 0; i < 2; i++ {
		if !radAlgCarriers[inst.algorithm][i] {
			continue
		}
		op := inst.operators[i]
		opVol := uint16((op[1]&63)^63) * scaledVol / 64
		reg := 0x40 + radOpOffsets2[chanNum][i]
		p.setOPL3(reg, (p.getOPL3(reg)&0xC0)|uint8(opVol^0x3F))
	}
}

func (p *Player) getSlideDir(chanNum int, fx *radEffects) {
	ch := &p.channels[chanNum]
	speed := int8(fx.toneSlideSpeed)
	if speed > 0 {
		oct := fx.toneSlideOct
		freq := fx.toneSlideFreq
		oldFreq := ch.currFreq
		oldOct := uint8(ch.currOctave)
		if oldOct > oct {
			speed = -speed
		} else if oldOct == oct {
			if oldFreq > freq {
				speed = -speed
			} else if oldFreq == freq {
				speed = 0
			}
		}
	}
	fx.toneSlideDir = speed
}

func (p *Player) portamento(chanNum int, fx *radEffects, amount int8, toneSlide bool) {
	ch := &p.channels[chanNum]
	freq := ch.currFreq
	oct := ch.currOctave
	freq = uint16(int32(freq) + int32(amount))
	if freq < 0x156 {
		if oct > 0 {
			oct--
			freq += 0x2AE - 0x156
		} else {
			freq = 0x156
		}
	} else if freq > 0x2AE {
		if oct < 7 {
			oct++
			freq -= 0x2AE - 0x156
		} else {
			freq = 0x2AE
		}
	}
	if toneSlide {
		if amount >= 0 {
			if oct > int8(fx.toneSlideOct) || (oct == int8(fx.toneSlideOct) && freq >= fx.toneSlideFreq) {
				freq = fx.toneSlideFreq
				oct = int8(fx.toneSlideOct)
			}
		} else {
			if oct < int8(fx.toneSlideOct) || (oct == int8(fx.toneSlideOct) && freq <= fx.toneSlideFreq) {
				freq = fx.toneSlideFreq
				oct = int8(fx.toneSlideOct)
			}
		}
	}
	ch.currFreq = freq
	ch.currOctave = oct
	off := uint16(chanNum)
	p.setOPL3(0xA0+off, uint8(freq&0xFF))
	p.setOPL3(0xB0+off, (uint8(freq>>8)&3)|uint8(oct<<2)|(p.getOPL3(0xB0+off)&0xE0))
}
