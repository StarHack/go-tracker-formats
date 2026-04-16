// player.go - RAD v2.1 player (OPL3, 9 channels, up to 4-op, riffs).
// Pure Go port of the RAD v2.1 replayer by Shayde/Reality (public domain).
package radv2

import "rad2wav/formats"


const (
	kTracks      = 100
	kChannels    = 9
	kTrackLines  = 64
	kRiffTracks  = 10
	kInstruments = 127

	cmPortamentoUp  = 0x1
	cmPortamentoDwn = 0x2
	cmToneSlide     = 0x3
	cmToneVolSlide  = 0x5
	cmVolSlide      = 0xA
	cmSetVol        = 0xC
	cmJumpToLine    = 0xD
	cmSetSpeed      = 0xF
	cmIgnore        = 'I' - 55
	cmMultiplier    = 'M' - 55
	cmRiff          = 'R' - 55
	cmTranspose     = 'T' - 55
	cmFeedback      = 'U' - 55
	cmVolume        = 'V' - 55
)

type noteSource int

const (
	srcNone noteSource = iota
	srcRiff
	srcIRiff
)

const (
	fKeyOn   = 1 << 0
	fKeyOff  = 1 << 1
	fKeyedOn = 1 << 2
)

var radNoteSize = [8]int8{0, 2, 1, 3, 1, 3, 2, 4}

var radChanOffsets3 = [9]uint16{0, 1, 2, 0x100, 0x101, 0x102, 6, 7, 8}
var radChn2Offsets3 = [9]uint16{3, 4, 5, 0x103, 0x104, 0x105, 0x106, 0x107, 0x108}
var radNoteFreq = [12]uint16{0x16b, 0x181, 0x198, 0x1b0, 0x1ca, 0x1e5, 0x202, 0x220, 0x241, 0x263, 0x287, 0x2ae}
var radOpOffsets3 = [9][4]uint16{
	{0x00B, 0x008, 0x003, 0x000},
	{0x00C, 0x009, 0x004, 0x001},
	{0x00D, 0x00A, 0x005, 0x002},
	{0x10B, 0x108, 0x103, 0x100},
	{0x10C, 0x109, 0x104, 0x101},
	{0x10D, 0x10A, 0x105, 0x102},
	{0x113, 0x110, 0x013, 0x010},
	{0x114, 0x111, 0x014, 0x011},
	{0x115, 0x112, 0x015, 0x012},
}
var radOpOffsets2 = [9][2]uint16{
	{0x003, 0x000},
	{0x004, 0x001},
	{0x005, 0x002},
	{0x00B, 0x008},
	{0x00C, 0x009},
	{0x00D, 0x00A},
	{0x013, 0x010},
	{0x014, 0x011},
	{0x015, 0x012},
}

var radAlgCarriers = [7][4]bool{
	{true, false, false, false}, // 0 - 2op FM
	{true, true, false, false},  // 1 - 2op additive
	{true, false, false, false}, // 2 - 4op FM chain
	{true, false, false, true},  // 3 - 4op FM+add
	{true, false, true, false},  // 4 - 4op
	{true, false, true, true},   // 5 - 4op
	{true, true, true, true},    // 6 - 4op all additive
}

type radInstrument struct {
	feedback  [2]uint8
	panning   [2]uint8
	algorithm uint8
	detune    uint8
	volume    uint8
	riffSpeed uint8
	riff      []byte
	operators [4][5]uint8
}

type radEffects struct {
	portSlide      int8
	volSlide       int8
	toneSlideFreq  uint16
	toneSlideOct   uint8
	toneSlideSpeed uint8
	toneSlideDir   int8
}

type radRiff struct {
	fx              radEffects
	track           []byte
	trackStart      []byte
	line            uint8
	speed           uint8
	speedCnt        uint8
	transposeOctave int8
	transposeNote   int8
	lastInstrument  uint8
}

type radChannel struct {
	lastInstrument uint8
	instrument     *radInstrument
	volume         uint8
	detuneA        uint8
	detuneB        uint8
	keyFlags       uint8
	currFreq       uint16
	currOctave     int8
	fx             radEffects
	riff           radRiff
	iriff          radRiff
}

// Player plays RAD v1 or v2.1 tune files via an OPL3 callback.
type Player struct {
	opl3          func(uint16, uint8)
	tune          []byte
	version       int  // 1 = v1, 2 = v2
	useOPL3       bool // true for v2
	instruments   [kInstruments]radInstrument
	channels      [kChannels]radChannel
	playTime      uint32
	orderMap      [4]uint32
	repeating     bool
	hertz         int16
	orderList     []byte
	tracks        [kTracks][]byte
	riffs         [kRiffTracks][kChannels][]byte
	track         []byte
	initialised   bool
	speed         uint8
	orderListSize uint8
	speedCnt      uint8
	order         uint8
	line          uint8
	entrances     int8
	masterVol     uint8
	lineJump      int8
	opl3Regs      [512]uint8

	// exported by unpackNote
	noteNum   int8
	octaveNum int8
	instNum   uint8
	effectNum uint8
	param     uint8
	lastNote  bool
}

// compile-time interface check
var _ formats.Tracker = (*Player)(nil)

// GetDescription returns the raw embedded description bytes from the tune.
func (p *Player) GetDescription() []byte {
	if len(p.tune) < 0x12 {
		return nil
	}
	s := p.tune[0x11:]
	flags := s[0]
	s = s[1:]
	if flags&0x20 != 0 && len(s) >= 2 {
		s = s[2:]
	}
	var desc []byte
	for len(s) > 0 {
		c := s[0]
		s = s[1:]
		if c == 0 {
			break
		}
		desc = append(desc, c)
	}
	return desc
}


// Init initialises the player for a tune.  opl3Fn is called for every OPL3
// register write.
func (p *Player) Init(tune []byte, opl3Fn func(uint16, uint8)) string {
	p.initialised = false
	p.tune = tune

	if len(tune) < 0x11 || (tune[0x10] != 0x10 && tune[0x10] != 0x21) {
		p.hertz = -1
		return "Not a RAD v1 or v2.1 tune."
	}
	p.version = int(tune[0x10] >> 4) // 1 for v1, 2 for v2
	p.useOPL3 = p.version >= 2

	p.opl3 = opl3Fn

	for i := range p.tracks {
		p.tracks[i] = nil
	}
	for i := range p.riffs {
		for j := range p.riffs[i] {
			p.riffs[i][j] = nil
		}
	}

	s := tune[0x11:]

	flags := s[0]
	s = s[1:]
	p.speed = flags & 0x1F

	p.hertz = 50
	if flags&0x20 != 0 {
		bpm := int(s[0]) | (int(s[1]) << 8)
		s = s[2:]
		p.hertz = int16(bpm * 2 / 5)
	}
	if flags&0x40 != 0 {
		p.hertz = 18
	}

	// Skip description (null-terminated): v2 always, v1 only when flags bit 7 set
	if p.version >= 2 || flags&0x80 != 0 {
		for s[0] != 0 {
			s = s[1:]
		}
		s = s[1:]
	}

	// Parse instruments
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

		if p.version >= 2 {
			// v2 instrument
			nameLen := int(s[0])
			s = s[1+nameLen:]

			alg := s[0]
			s = s[1:]
			inst.algorithm = alg & 7
			inst.panning[0] = (alg >> 3) & 3
			inst.panning[1] = (alg >> 5) & 3

			if inst.algorithm < 7 {
				b := s[0]
				s = s[1:]
				inst.feedback[0] = b & 15
				inst.feedback[1] = b >> 4

				b = s[0]
				s = s[1:]
				inst.detune = b >> 4
				inst.riffSpeed = b & 15

				inst.volume = s[0]
				s = s[1:]

				for i := 0; i < 4; i++ {
					for j := 0; j < 5; j++ {
						inst.operators[i][j] = s[0]
						s = s[1:]
					}
				}
			} else {
				// MIDI instrument: skip 6 bytes
				s = s[6:]
			}

			// Instrument riff?
			if alg&0x80 != 0 {
				size := int(s[0]) | (int(s[1]) << 8)
				s = s[2:]
				inst.riff = s[:size]
				s = s[size:]
			} else {
				inst.riff = nil
			}
		} else {
			// v1 instrument: 11 bytes
			// Byte layout: mod_avekm, car_avekm, mod_ksltl, car_ksltl,
			//              mod_ardr, car_ardr, mod_slrr, car_slrr,
			//              fb_conn, mod_wave, car_wave
			inst.algorithm = s[8] & 1
			inst.panning[0] = 0
			inst.panning[1] = 0
			inst.feedback[0] = (s[8] >> 1) & 0x7
			inst.feedback[1] = 0
			inst.detune = 0
			inst.riffSpeed = 0
			inst.volume = 64
			inst.operators[0][0] = s[0]  // mod AVEKM
			inst.operators[1][0] = s[1]  // car AVEKM
			inst.operators[0][1] = s[2]  // mod KSLTL
			inst.operators[1][1] = s[3]  // car KSLTL
			inst.operators[0][2] = s[4]  // mod ARDR
			inst.operators[1][2] = s[5]  // car ARDR
			inst.operators[0][3] = s[6]  // mod SLRR
			inst.operators[1][3] = s[7]  // car SLRR
			inst.operators[0][4] = s[9]  // mod WAVE
			inst.operators[1][4] = s[10] // car WAVE
			inst.operators[2] = [5]uint8{}
			inst.operators[3] = [5]uint8{}
			inst.riff = nil
			s = s[11:]
		}
	}

	// Order list
	p.orderListSize = s[0]
	s = s[1:]
	p.orderList = s[:p.orderListSize]
	s = s[p.orderListSize:]

	// Tracks
	if p.version >= 2 {
		for {
			trackNum := s[0]
			s = s[1:]
			if int(trackNum) >= kTracks {
				break
			}
			size := int(s[0]) | (int(s[1]) << 8)
			s = s[2:]
			p.tracks[trackNum] = s[:size]
			s = s[size:]
		}
		// Riffs
		for {
			riffID := s[0]
			s = s[1:]
			riffNum := int(riffID >> 4)
			chanNum := int(riffID & 15)
			if riffNum >= kRiffTracks || chanNum > kChannels {
				break
			}
			size := int(s[0]) | (int(s[1]) << 8)
			s = s[2:]
			if chanNum > 0 {
				p.riffs[riffNum][chanNum-1] = s[:size]
			}
			s = s[size:]
		}
	} else {
		// v1: 32 x uint16 LE absolute offsets
		for i := 0; i < 32; i++ {
			pos := int(s[0]) | (int(s[1]) << 8)
			s = s[2:]
			if pos != 0 {
				p.tracks[i] = tune[pos:]
			}
		}
	}

	for i := range p.opl3Regs {
		p.opl3Regs[i] = 255
	}
	p.Stop()
	p.initialised = true
	return ""
}

// Stop halts all sounds and resets playback to the beginning.
func (p *Player) Stop() {
	// Clear OPL3 state
	for reg := uint16(0x20); reg < 0xF6; reg++ {
		var val uint8
		if reg >= 0x60 && reg < 0xA0 {
			val = 0xFF
		}
		p.setOPL3(reg, val)
		p.setOPL3(reg+0x100, val)
	}
	p.setOPL3(1, 0x20)
	p.setOPL3(8, 0)
	p.setOPL3(0xBD, 0)
	p.setOPL3(0x104, 0)
	p.setOPL3(0x105, 1)

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
		ch.detuneA = 0
		ch.detuneB = 0
		ch.keyFlags = 0
		ch.riff.speedCnt = 0
		ch.iriff.speedCnt = 0
	}
}

// Update advances playback by one tick. Returns true when the tune starts to repeat.
func (p *Player) Update() bool {
	if !p.initialised {
		return false
	}

	for i := 0; i < kChannels; i++ {
		ch := &p.channels[i]
		p.tickRiff(i, &ch.iriff, false)
		p.tickRiff(i, &ch.riff, true)
	}

	p.playLine()

	for i := 0; i < kChannels; i++ {
		ch := &p.channels[i]
		p.continueFX(i, &ch.iriff.fx)
		p.continueFX(i, &ch.riff.fx)
		p.continueFX(i, &ch.fx)
	}

	p.playTime++
	return p.repeating
}

// GetHertz returns the update rate in Hz (-1 on error).
func (p *Player) GetHertz() int { return int(p.hertz) }

// GetPlayTimeInSeconds returns elapsed play time in seconds.
func (p *Player) GetPlayTimeInSeconds() int {
	if p.hertz <= 0 {
		return 0
	}
	return int(p.playTime) / int(p.hertz)
}

func (p *Player) setOPL3(reg uint16, val uint8) {
	p.opl3Regs[reg] = val
	p.opl3(reg, val)
}

func (p *Player) getOPL3(reg uint16) uint8 {
	return p.opl3Regs[reg]
}

func (p *Player) getTrack() []byte {
	if int(p.order) >= int(p.orderListSize) {
		p.order = 0
	}
	trackNum := p.orderList[p.order]
	if trackNum&0x80 != 0 {
		p.order = trackNum & 0x7F
		trackNum = p.orderList[p.order] & 0x7F
	}
	// Track repeat detection
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

func (p *Player) skipToLine(trk []byte, linenum uint8, chanRiff bool) []byte {
	for {
		if len(trk) == 0 {
			return nil
		}
		lineID := trk[0]
		if (lineID & 0x7F) >= linenum {
			return trk
		}
		if lineID&0x80 != 0 {
			break
		}
		trk = trk[1:]
		// Skip channel notes
		for {
			if len(trk) == 0 {
				return nil
			}
			chanID := trk[0]
			trk = trk[1:]
			if p.version >= 2 {
				size := int(radNoteSize[(chanID>>4)&7])
				trk = trk[size:]
			} else {
				// v1: note byte + inst/effect byte + optional param byte
				if len(trk) < 2 {
					return nil
				}
				instEffect := trk[1]
				if instEffect&0x0f != 0 {
					trk = trk[3:] // note + inst/effect + param
				} else {
					trk = trk[2:] // note + inst/effect
				}
			}
			if chanID&0x80 != 0 || chanRiff {
				break
			}
		}
	}
	return nil
}

func (p *Player) unpackNote(s []byte, lastInstrument *uint8) ([]byte, bool) {
	if len(s) == 0 {
		return s, true
	}
	chanID := s[0]
	s = s[1:]

	p.instNum = 0
	p.effectNum = 0
	p.param = 0

	note := uint8(0)

	if p.version >= 2 {
		// v2 note format
		if chanID&0x40 != 0 {
			n := s[0]
			s = s[1:]
			note = n & 0x7F
			if n&0x80 != 0 {
				p.instNum = *lastInstrument
			}
		}
		if chanID&0x20 != 0 {
			p.instNum = s[0]
			s = s[1:]
			*lastInstrument = p.instNum
		}
		if chanID&0x10 != 0 {
			p.effectNum = s[0]
			s = s[1:]
			p.param = s[0]
			s = s[1:]
		}
	} else {
		// v1 note format: note byte, then inst/effect byte, optional param
		n := s[0]
		s = s[1:]
		note = n & 0x7F
		if n&0x80 != 0 {
			p.instNum = 16 // high bit of instrument number
		}
		n = s[0]
		s = s[1:]
		p.instNum |= n >> 4
		if p.instNum != 0 {
			*lastInstrument = p.instNum
		}
		p.effectNum = n & 0x0F
		if p.effectNum != 0 {
			p.param = s[0]
			s = s[1:]
		}
	}

	p.noteNum = int8(note & 15)
	p.octaveNum = int8(note >> 4)

	last := (chanID & 0x80) != 0
	return s, last
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
			p.playNote(chanNum, p.noteNum, p.octaveNum, uint16(p.instNum), p.effectNum, p.param, srcNone, 0)
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

func (p *Player) playNote(chanNum int, notenum, octave int8, instnum uint16, cmd, param uint8, src noteSource, op int) {
	ch := &p.channels[chanNum]

	if p.entrances >= 8 {
		return
	}
	p.entrances++

	var fx *radEffects
	switch src {
	case srcRiff:
		fx = &ch.riff.fx
	case srcIRiff:
		fx = &ch.iriff.fx
	default:
		fx = &ch.fx
	}

	transposing := false

	if cmd == cmToneSlide {
		if notenum > 0 && notenum <= 12 {
			fx.toneSlideOct = uint8(octave)
			fx.toneSlideFreq = radNoteFreq[notenum-1]
		}
		// apply tone-slide speed directly (same as the toneslide label)
		speedVal := param
		if speedVal != 0 {
			fx.toneSlideSpeed = speedVal
		}
		p.getSlideDir(chanNum, fx)
		p.entrances--
		return
	}

	if instnum > 0 {
		oldInst := ch.instrument
		inst := &p.instruments[instnum-1]
		ch.instrument = inst

		if inst.algorithm == 7 {
			p.entrances--
			return
		}

		p.loadInstrumentOPL3(chanNum)
		ch.keyFlags |= fKeyOff | fKeyOn
		p.resetFX(&ch.iriff.fx)

		if src != srcIRiff || inst != oldInst {
			if inst.riff != nil && inst.riffSpeed > 0 {
				ch.iriff.track = inst.riff
				ch.iriff.trackStart = inst.riff
				ch.iriff.line = 0
				ch.iriff.speed = inst.riffSpeed
				ch.iriff.lastInstrument = 0
				if notenum >= 1 && notenum <= 12 {
					ch.iriff.transposeOctave = octave
					ch.iriff.transposeNote = notenum
					transposing = true
				} else {
					ch.iriff.transposeOctave = 3
					ch.iriff.transposeNote = 12
				}
				ch.iriff.speedCnt = 1
				p.tickRiff(chanNum, &ch.iriff, false)
			} else {
				ch.iriff.speedCnt = 0
			}
		}
	}

	if cmd == cmRiff || cmd == cmTranspose {
		p.resetFX(&ch.riff.fx)
		p0 := param / 10
		p1 := param % 10
		if p1 > 0 {
			ch.riff.track = p.riffs[p0][p1-1]
		} else {
			ch.riff.track = nil
		}
		if ch.riff.track != nil {
			ch.riff.trackStart = ch.riff.track
			ch.riff.line = 0
			ch.riff.speed = p.speed
			ch.riff.lastInstrument = 0
			if cmd == cmTranspose && notenum >= 1 && notenum <= 12 {
				ch.riff.transposeOctave = octave
				ch.riff.transposeNote = notenum
				transposing = true
			} else {
				ch.riff.transposeOctave = 3
				ch.riff.transposeNote = 12
			}
			ch.riff.speedCnt = 1
			p.tickRiff(chanNum, &ch.riff, true)
		} else {
			ch.riff.speedCnt = 0
		}
	}

	if !transposing && notenum > 0 {
		if notenum == 15 {
			ch.keyFlags |= fKeyOff
		}
		if ch.instrument == nil || ch.instrument.algorithm < 7 {
			p.playNoteOPL3(chanNum, octave, notenum)
		}
	}

	switch cmd {
	case cmSetVol:
		p.setVolume(chanNum, param)
	case cmSetSpeed:
		switch src {
		case srcNone:
			p.speed = param
			p.speedCnt = param
		case srcRiff:
			ch.riff.speed = param
			ch.riff.speedCnt = param
		case srcIRiff:
			ch.iriff.speed = param
			ch.iriff.speedCnt = param
		}
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
		// also apply tone-slide
		speed := param
		if speed != 0 {
			fx.toneSlideSpeed = speed
		}
		p.getSlideDir(chanNum, fx)
	case cmToneSlide:
		// handled above before the instrument/note processing
	case cmVolSlide:
		val := int8(param)
		if val >= 50 {
			val = -(val - 50)
		}
		fx.volSlide = val
	case cmJumpToLine:
		if int(param) < kTrackLines && src == srcNone {
			p.lineJump = int8(param)
		}
	case cmMultiplier:
		if src == srcIRiff {
			p.loadInstMultiplierOPL3(chanNum, op, param)
		}
	case cmVolume:
		if src == srcIRiff {
			p.loadInstVolumeOPL3(chanNum, op, param)
		}
	case cmFeedback:
		if src == srcIRiff {
			which := param / 10
			fb := param % 10
			p.loadInstFeedbackOPL3(chanNum, int(which), fb)
		}
	}

	p.entrances--
}

func (p *Player) loadInstrumentOPL3(chanNum int) {
	ch := &p.channels[chanNum]
	inst := ch.instrument
	if inst == nil {
		return
	}

	alg := inst.algorithm
	ch.volume = inst.volume
	ch.detuneA = (inst.detune + 1) >> 1
	ch.detuneB = inst.detune >> 1

	if p.useOPL3 {
		// OPL3: 4-op support
		if chanNum < 6 {
			mask := uint8(1 << uint(chanNum))
			var fourOp uint8
			if alg == 2 || alg == 3 {
				fourOp = mask
			}
			p.setOPL3(0x104, (p.getOPL3(0x104)&^mask)|fourOp)
		}

		algBit := uint8(0)
		if alg == 3 || alg == 5 || alg == 6 {
			algBit = 1
		}
		p.setOPL3(0xC0+radChanOffsets3[chanNum], ((inst.panning[1]^3)<<4)|inst.feedback[1]<<1|algBit)

		algBit2 := uint8(0)
		if alg == 1 || alg == 6 {
			algBit2 = 1
		}
		p.setOPL3(0xC0+radChn2Offsets3[chanNum], ((inst.panning[0]^3)<<4)|inst.feedback[0]<<1|algBit2)

		blank := [5]uint8{0, 0x3F, 0, 0xF0, 0}
		for i := 0; i < 4; i++ {
			var op [5]uint8
			if alg < 2 && i >= 2 {
				op = blank
			} else {
				op = inst.operators[i]
			}
			reg := radOpOffsets3[chanNum][i]
			vol := uint16(^op[1]) & 0x3F
			if radAlgCarriers[alg][i] {
				vol = vol * uint16(inst.volume) / 64
				vol = vol * uint16(p.masterVol) / 64
			}
			p.setOPL3(reg+0x20, op[0])
			p.setOPL3(reg+0x40, (op[1]&0xC0)|uint8((vol^0x3F)&0x3F))
			p.setOPL3(reg+0x60, op[2])
			p.setOPL3(reg+0x80, op[3])
			p.setOPL3(reg+0xE0, op[4])
		}
	} else {
		// OPL2: 2-op, simple channel offset
		algBit := uint8(0)
		if alg == 1 {
			algBit = 1
		}
		p.setOPL3(0xC0+uint16(chanNum), ((inst.panning[0]^3)<<4)|inst.feedback[0]<<1|algBit)

		for i := 0; i < 2; i++ {
			op := inst.operators[i]
			reg := radOpOffsets2[chanNum][i]
			vol := uint16(^op[1]) & 0x3F
			// alg 0 = FM (only op[1] is carrier), alg 1 = additive (both carriers)
			if radAlgCarriers[alg][i] {
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
}

func (p *Player) playNoteOPL3(chanNum int, octave, note int8) {
	ch := &p.channels[chanNum]

	var o1, o2 uint16
	if p.useOPL3 {
		o1 = radChanOffsets3[chanNum]
		o2 = radChn2Offsets3[chanNum]
	} else {
		o1 = 0 // unused for OPL2
		o2 = uint16(chanNum)
	}

	if ch.keyFlags&fKeyOff != 0 {
		ch.keyFlags &^= fKeyOff | fKeyedOn
		if p.useOPL3 {
			p.setOPL3(0xB0+o1, p.getOPL3(0xB0+o1)&^0x20)
		}
		p.setOPL3(0xB0+o2, p.getOPL3(0xB0+o2)&^0x20)
	}
	if note == 15 {
		return
	}

	op4 := p.useOPL3 && ch.instrument != nil && ch.instrument.algorithm >= 2

	freq := radNoteFreq[note-1]
	frq2 := freq
	ch.currFreq = freq
	ch.currOctave = octave

	freq += uint16(ch.detuneA)
	frq2 -= uint16(ch.detuneB)

	if op4 {
		p.setOPL3(0xA0+o1, uint8(frq2&0xFF))
	}
	p.setOPL3(0xA0+o2, uint8(freq&0xFF))

	if ch.keyFlags&fKeyOn != 0 {
		ch.keyFlags = (ch.keyFlags &^ fKeyOn) | fKeyedOn
	}
	keyBit := uint8(0)
	if ch.keyFlags&fKeyedOn != 0 {
		keyBit = 0x20
	}
	if op4 {
		p.setOPL3(0xB0+o1, uint8(frq2>>8)|uint8(octave<<2)|keyBit)
	} else if p.useOPL3 {
		p.setOPL3(0xB0+o1, 0)
	}
	p.setOPL3(0xB0+o2, uint8(freq>>8)|uint8(octave<<2)|keyBit)
}

func (p *Player) resetFX(fx *radEffects) {
	fx.portSlide = 0
	fx.volSlide = 0
	fx.toneSlideDir = 0
}

func (p *Player) tickRiff(chanNum int, riff *radRiff, chanRiff bool) {
	if riff.speedCnt == 0 {
		p.resetFX(&riff.fx)
		return
	}
	riff.speedCnt--
	if riff.speedCnt > 0 {
		return
	}
	riff.speedCnt = riff.speed

	line := riff.line
	riff.line++
	if int(riff.line) >= kTrackLines {
		riff.speedCnt = 0
	}

	p.resetFX(&riff.fx)

	trk := riff.track
	if trk != nil && (trk[0]&0x7F) == line {
		lineID := trk[0]
		trk = trk[1:]
		if chanRiff {
			var last bool
			trk, last = p.unpackNote(trk, &riff.lastInstrument)
			_ = last
			p.transpose(riff.transposeNote, riff.transposeOctave)
			p.playNote(chanNum, p.noteNum, p.octaveNum, uint16(p.instNum), p.effectNum, p.param, srcRiff, 0)
		} else {
			for {
				col := int(trk[0] & 15)
				var last bool
				trk, last = p.unpackNote(trk, &riff.lastInstrument)
				if p.effectNum != cmIgnore {
					p.transpose(riff.transposeNote, riff.transposeOctave)
				}
				opIdx := 0
				if col > 0 {
					opIdx = (col - 1) & 3
				}
				p.playNote(chanNum, p.noteNum, p.octaveNum, uint16(p.instNum), p.effectNum, p.param, srcIRiff, opIdx)
				if last {
					break
				}
			}
		}
		if lineID&0x80 != 0 {
			trk = nil
		}
		riff.track = trk
	}

	// Check for jump command on next line
	if trk == nil {
		return
	}
	nextLine := trk[0] & 0x7F
	if nextLine != riff.line {
		return
	}
	trk = trk[1:]
	var dummy uint8
	trk, _ = p.unpackNote(trk, &dummy)
	if p.effectNum == cmJumpToLine && int(p.param) < kTrackLines {
		riff.line = p.param
		riff.track = p.skipToLine(riff.trackStart, p.param, chanRiff)
	}
}

func (p *Player) continueFX(chanNum int, fx *radEffects) {
	ch := &p.channels[chanNum]

	if fx.portSlide != 0 {
		p.portamento(uint16(chanNum), fx, fx.portSlide, false)
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
		p.portamento(uint16(chanNum), fx, fx.toneSlideDir, true)
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
	alg := inst.algorithm
	numOps := 4
	if !p.useOPL3 {
		numOps = 2
	}
	for i := 0; i < numOps; i++ {
		if !radAlgCarriers[alg][i] {
			continue
		}
		op := inst.operators[i]
		opVol := uint16((op[1]&63)^63) * scaledVol / 64
		var reg uint16
		if p.useOPL3 {
			reg = 0x40 + radOpOffsets3[chanNum][i]
		} else {
			reg = 0x40 + radOpOffsets2[chanNum][i]
		}
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

func (p *Player) loadInstMultiplierOPL3(chanNum, op int, mult uint8) {
	reg := 0x20 + radOpOffsets3[chanNum][op]
	p.setOPL3(reg, (p.getOPL3(reg)&0xF0)|(mult&15))
}

func (p *Player) loadInstVolumeOPL3(chanNum, op int, vol uint8) {
	reg := 0x40 + radOpOffsets3[chanNum][op]
	p.setOPL3(reg, (p.getOPL3(reg)&0xC0)|((vol&0x3F)^0x3F))
}

func (p *Player) loadInstFeedbackOPL3(chanNum, which int, fb uint8) {
	if which == 0 {
		reg := 0xC0 + radChn2Offsets3[chanNum]
		p.setOPL3(reg, (p.getOPL3(reg)&0x31)|((fb&7)<<1))
	} else if which == 1 {
		reg := 0xC0 + radChanOffsets3[chanNum]
		p.setOPL3(reg, (p.getOPL3(reg)&0x31)|((fb&7)<<1))
	}
}

func (p *Player) portamento(chanNum uint16, fx *radEffects, amount int8, toneSlide bool) {
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

	frq2 := freq - uint16(ch.detuneB)
	freq += uint16(ch.detuneA)

	if p.useOPL3 {
		chanOffset := radChn2Offsets3[chanNum]
		p.setOPL3(0xA0+chanOffset, uint8(freq&0xFF))
		p.setOPL3(0xB0+chanOffset, (uint8(freq>>8)&3)|uint8(oct<<2)|(p.getOPL3(0xB0+chanOffset)&0xE0))
		chanOffset = radChanOffsets3[chanNum]
		p.setOPL3(0xA0+chanOffset, uint8(frq2&0xFF))
		p.setOPL3(0xB0+chanOffset, (uint8(frq2>>8)&3)|uint8(oct<<2)|(p.getOPL3(0xB0+chanOffset)&0xE0))
	} else {
		chanOffset := uint16(chanNum)
		p.setOPL3(0xA0+chanOffset, uint8(freq&0xFF))
		p.setOPL3(0xB0+chanOffset, (uint8(freq>>8)&3)|uint8(oct<<2)|(p.getOPL3(0xB0+chanOffset)&0xE0))
	}
}

func (p *Player) transpose(note, octave int8) {
	if p.noteNum >= 1 && p.noteNum <= 12 {
		toct := octave - 3
		if toct != 0 {
			p.octaveNum += toct
			if p.octaveNum < 0 {
				p.octaveNum = 0
			} else if p.octaveNum > 7 {
				p.octaveNum = 7
			}
		}
		tnot := note - 12
		if tnot != 0 {
			p.noteNum += tnot
			if p.noteNum < 1 {
				p.noteNum += 12
				if p.octaveNum > 0 {
					p.octaveNum--
				} else {
					p.noteNum = 1
				}
			}
		}
	}
}
