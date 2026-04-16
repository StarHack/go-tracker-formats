// player.go - ProTracker-compatible MOD player.
// Implements formats.PCMTracker.
// Supports M.K., M!K!, FLT4/8, xCHN, xxCH, full ProTracker effect set.
package mod

import (
	"math"
	"rad2wav/formats"
	"strings"
)

const amigaClock = 3579545 // NTSC Paula

// Finetune nibble (0-15) → signed semitone-eighths (-8…+7).
var finetuneTab = [16]int8{0, 1, 2, 3, 4, 5, 6, 7, -8, -7, -6, -5, -4, -3, -2, -1}

// Amiga periods for C-1…B-3 at finetune 0 (ProTracker standard).
var basePeriods = [36]uint16{
	856, 808, 762, 720, 678, 640, 604, 570, 538, 508, 480, 453,
	428, 404, 381, 360, 339, 320, 302, 285, 269, 254, 240, 226,
	214, 202, 190, 180, 170, 160, 151, 143, 135, 127, 120, 113,
}

// Sine table: 64 entries, amplitude 0-255 (sin wave, positive half).
var sineTab [64]int16

func init() {
	for i := 0; i < 64; i++ {
		sineTab[i] = int16(math.Round(255.0 * math.Sin(float64(i)*2.0*math.Pi/64.0)))
	}
}

// periodForNote returns the Amiga period for note index n (0=C-1) at the given finetune nibble.
func periodForNote(n int, ft uint8) uint16 {
	if n < 0 || n >= 36 {
		return 0
	}
	shift := float64(finetuneTab[ft&15]) / 96.0 // finetune units → octaves
	return uint16(math.Round(float64(basePeriods[n]) * math.Pow(2.0, -shift)))
}

// noteForPeriod finds the closest note index (0-35) for a period value at finetune 0.
func noteForPeriod(period uint16) int {
	best, bestD := 0, uint16(0xFFFF)
	for i, p := range basePeriods {
		d := period - p
		if period < p {
			d = p - period
		}
		if d < bestD {
			bestD, best = d, i
		}
	}
	return best
}

// waveOutput returns the oscillator value for pos (0-63) for the given wave type.
// Returns a value in -255…+255.
func waveOutput(wave uint8, pos int) int16 {
	switch wave & 3 {
	case 0: // sine
		return sineTab[pos&63]
	case 1: // ramp down
		return int16(255 - (pos&63)*8)
	case 2: // square
		if pos&32 != 0 {
			return -255
		}
		return 255
	default: // random
		return int16((pos * 1664525) & 255)
	}
}

// clampVol clamps a volume to [0, 64].
func clampVol(v int) int {
	if v < 0 {
		return 0
	}
	if v > 64 {
		return 64
	}
	return v
}

// clampPeriod clamps an Amiga period to [113, 856].
func clampPeriod(p int) uint16 {
	if p < 113 {
		return 113
	}
	if p > 856 {
		return 856
	}
	return uint16(p)
}

// ─── Sample descriptor ───────────────────────────────────────────────────────

type modSample struct {
	name      string
	length    int   // bytes
	finetune  uint8 // raw nibble 0-15
	volume    uint8 // 0-64
	loopStart int   // bytes
	loopLen   int   // bytes (≤1 = no loop)
	data      []int8
}

func (s *modSample) hasLoop() bool { return s.loopLen > 2 }

// ─── Channel state ───────────────────────────────────────────────────────────

type modChannel struct {
	// Current sample
	sample   *modSample
	finetune uint8

	// Pitch
	period     uint16 // base Amiga period
	playPeriod uint16 // effective playback period after arp/vibrato
	note       int    // note index 0-35 (-1 = none)
	portDst    uint16 // tone-portamento target period
	portSpd    uint16 // tone-portamento speed
	portaUp    uint8  // 1xx memory
	portaDown  uint8  // 2xx memory
	glissando  bool   // E3x: snap tone-portamento to semitone

	// Volume
	volume  int // 0-64 (channel volume, before tremolo)
	tremVol int // effective volume after tremolo (used for mixing)

	// Vibrato
	vibPos    int
	vibSpd    int
	vibDep    int
	vibWave   uint8
	vibRetrig bool // E4 bit 2: do not retrigger

	// Tremolo
	tremPos    int
	tremSpd    int
	tremDep    int
	tremWave   uint8
	tremRetrig bool

	// Arpeggio
	arpNotes [3]uint16 // arp[0]=base period, arp[1],arp[2]=alt periods
	arpStep  int       // 0/1/2

	// Mixer state
	pos     float64 // sample position
	posStep float64 // advance per output sample
	pan     int     // 0=full left … 255=full right (128=centre)
	active  bool

	// Volume-slide (Axy / 5xy / 6xy)
	volSlide    int   // per-tick delta
	volSlideMem uint8 // Axy/5xy/6xy memory

	// Fine effects (instantaneous on tick 0 only)
	sampleOffset int // last 9xx offset memory in bytes

	// E6: pattern loop
	loopRow   int
	loopCnt   int
	loopArmed bool

	// E9: retrigger
	retrigSpd int // retrigger every N ticks (0 = off)
	retrigCnt int

	// EC: note cut, ED: note delay
	cutTick   int // tick to silence, -1 = none
	delayTick int // tick to play note, -1 = none
	// saved note info for note-delay
	delayPeriod uint16
	delaySample *modSample
	delayVolume int // -1 = no volume change
}

// ─── Player ──────────────────────────────────────────────────────────────────

// Player is a ProTracker-compatible MOD player.
type Player struct {
	sampleRate int
	data       []byte

	title    string
	samples  [32]modSample // 1-indexed; samples[0] unused
	numChan  int
	songLen  int
	restart  int
	order    [128]uint8
	patterns [][]uint32 // [patIdx * 64*numChan + row*numChan + chan] = packed note

	// Playback
	pos   int // current order position
	row   int // row within pattern (0-63)
	speed int // ticks per row
	bpm   int // beats per minute

	tick       int // current tick within row (0 = "note tick")
	samCnt     int // samples since last tick
	samPerTick int // samples per tick at current BPM

	channels []modChannel

	// Pending jumps (set during row processing, applied afterwards)
	nextPos     int // Bxx: jump to order position (-1 = none)
	nextRow     int // Dxx: break to row in next pattern (-1 = none)
	patDelayCnt int // EEx pattern delay counter

	// Repeat detection
	orderMap  [4]uint32
	repeating bool

	initialised bool
}

// Compile-time interface check.
var _ formats.PCMTracker = (*Player)(nil)

// Init prepares the player. Returns "" on success.
func (p *Player) Init(tune []byte, sampleRate int) string {
	p.initialised = false
	p.data = tune

	if err := Validate(tune); err != "" {
		return err
	}

	p.sampleRate = sampleRate

	// Title (first 20 bytes, null-trimmed)
	p.title = strings.TrimRight(string(tune[:20]), "\x00")

	// Tag → channel count
	var tag [4]byte
	copy(tag[:], tune[1080:1084])
	p.numChan = numChannelsFromTag(tag)

	// Parse 31 samples
	for i := 0; i < 31; i++ {
		b := 20 + i*30
		s := &p.samples[i+1]
		s.name = strings.TrimRight(string(tune[b:b+22]), "\x00")
		wordLen := int(tune[b+22])<<8 | int(tune[b+23])
		s.length = wordLen * 2
		s.finetune = tune[b+24] & 15
		s.volume = tune[b+25]
		if s.volume > 64 {
			s.volume = 64
		}
		s.loopStart = (int(tune[b+26])<<8 | int(tune[b+27])) * 2
		s.loopLen = (int(tune[b+28])<<8 | int(tune[b+29])) * 2
	}

	// Song length & restart
	p.songLen = int(tune[950])
	p.restart = int(tune[951])
	if p.restart >= p.songLen {
		p.restart = 0
	}
	copy(p.order[:], tune[952:1080])

	// Find max pattern number
	maxPat := 0
	for i := 0; i < p.songLen; i++ {
		if int(p.order[i]) > maxPat {
			maxPat = int(p.order[i])
		}
	}

	// Parse patterns
	p.patterns = make([][]uint32, maxPat+1)
	offset := 1084
	for pi := 0; pi <= maxPat; pi++ {
		pat := make([]uint32, 64*p.numChan)
		for row := 0; row < 64; row++ {
			for ch := 0; ch < p.numChan; ch++ {
				b0 := tune[offset]
				b1 := tune[offset+1]
				b2 := tune[offset+2]
				b3 := tune[offset+3]
				sample := uint32(b0&0xF0) | uint32(b2>>4)
				period := uint32(b0&0x0F)<<8 | uint32(b1)
				effect := uint32(b2 & 0x0F)
				param := uint32(b3)
				pat[row*p.numChan+ch] = sample<<24 | period<<12 | effect<<8 | param
				offset += 4
			}
		}
		p.patterns[pi] = pat
	}

	// Load sample data
	for i := 1; i <= 31; i++ {
		s := &p.samples[i]
		if s.length > 0 {
			raw := tune[offset : offset+s.length]
			s.data = make([]int8, s.length)
			for j, b := range raw {
				s.data[j] = int8(b)
			}
			offset += s.length
		}
	}

	// Amiga channel panning: LRRL (classic hard panning, softened to 75%).
	// 0=full left (0), 64=quarter left (64), 192=quarter right (192), 255=full right.
	defaultPan := []int{32, 224, 224, 32}

	p.channels = make([]modChannel, p.numChan)
	for i := range p.channels {
		p.channels[i].pan = defaultPan[i%4]
		p.channels[i].note = -1
		p.channels[i].cutTick = -1
		p.channels[i].delayTick = -1
		p.channels[i].delayVolume = -1
	}

	p.Stop()
	p.initialised = true
	return ""
}

// Stop resets playback to the beginning.
func (p *Player) Stop() {
	if p.channels != nil {
		for i := range p.channels {
			cp := p.channels[i].pan // preserve pan
			p.channels[i] = modChannel{}
			p.channels[i].pan = cp
			p.channels[i].note = -1
			p.channels[i].cutTick = -1
			p.channels[i].delayTick = -1
			p.channels[i].delayVolume = -1
		}
	}
	p.pos = 0
	p.row = 0
	p.speed = 6
	p.bpm = 125
	p.tick = 0
	p.samCnt = 0
	p.samPerTick = p.calcSamPerTick()
	p.nextPos = -1
	p.nextRow = -1
	p.patDelayCnt = 0
	p.repeating = false
	for i := range p.orderMap {
		p.orderMap[i] = 0
	}
}

// GetDescription returns the song title as bytes.
func (p *Player) GetDescription() []byte {
	if p.title == "" {
		return nil
	}
	return []byte(p.title)
}

// Sample generates one interleaved stereo output sample.
// It manages tick timing internally and returns true on repeat.
func (p *Player) Sample(left, right *int16) bool {
	// On tick boundary: process the row/tick
	if p.samCnt == 0 {
		if p.tick == 0 {
			p.processRow()
		} else {
			p.processTick()
		}
		p.tick++
		if p.tick >= p.speed+p.patDelayCnt*p.speed {
			p.tick = 0
			if p.patDelayCnt > 0 {
				p.patDelayCnt--
			}
			p.advanceRow()
		}
		p.samPerTick = p.calcSamPerTick()
	}
	p.samCnt++
	if p.samCnt >= p.samPerTick {
		p.samCnt = 0
	}

	// Mix all channels
	var lAcc, rAcc int32
	for i := range p.channels {
		ch := &p.channels[i]
		if !ch.active || ch.sample == nil || ch.period == 0 {
			continue
		}
		s := ch.sample
		// Compute position step: amigaClock / (period * sampleRate)
		period := ch.playPeriod
		if period == 0 {
			period = ch.period
		}
		if period == 0 {
			continue
		}
		ch.posStep = float64(amigaClock) / (float64(period) * float64(p.sampleRate))

		// Read sample (linear interpolation)
		pos := ch.pos
		i0 := int(pos)
		var s0, s1 int16
		if i0 < len(s.data) {
			s0 = int16(s.data[i0])
		}
		i1 := i0 + 1
		if s.hasLoop() && i1 >= s.loopStart+s.loopLen {
			i1 = s.loopStart
		}
		if i1 < len(s.data) {
			s1 = int16(s.data[i1])
		}
		frac := pos - float64(i0)
		sample := int16(float64(s0)*(1.0-frac) + float64(s1)*frac)

		// Volume scale: 8-bit sample * vol/64 → roughly 14-bit range
		vol := int32(ch.tremVol)
		sampleScaled := int32(sample) * vol * 256 / 64

		// Panning
		panR := int32(ch.pan) // 0-255
		panL := int32(255 - ch.pan)
		lAcc += sampleScaled * panL / 255
		rAcc += sampleScaled * panR / 255

		// Advance position
		ch.pos += ch.posStep
		floorPos := int(ch.pos)
		if s.hasLoop() {
			for floorPos >= s.loopStart+s.loopLen {
				ch.pos -= float64(s.loopLen)
				floorPos = int(ch.pos)
			}
		} else if floorPos >= s.length {
			ch.active = false
		}
	}

	// Scale: up to 32 channels, 14-bit samples → up to 21 bits; scale to 15-bit for headroom
	divisor := int32(p.numChan * 4)
	lOut := int32(lAcc / divisor)
	rOut := int32(rAcc / divisor)
	if lOut > 32767 {
		lOut = 32767
	} else if lOut < -32768 {
		lOut = -32768
	}
	if rOut > 32767 {
		rOut = 32767
	} else if rOut < -32768 {
		rOut = -32768
	}
	*left = int16(lOut)
	*right = int16(rOut)

	return p.repeating
}

// calcSamPerTick returns samples per tick for the current BPM.
// Formula: sampleRate * 60 / (bpm * 24)  (= sampleRate * 2.5 / bpm)
func (p *Player) calcSamPerTick() int {
	v := p.sampleRate * 5 / (p.bpm * 2)
	if v < 1 {
		v = 1
	}
	return v
}

// ─── Row processing ──────────────────────────────────────────────────────────

// processRow handles tick 0 of a new row: trigger notes, load instruments, set effects.
func (p *Player) processRow() {
	if p.pos >= p.songLen {
		return
	}
	patIdx := int(p.order[p.pos])
	if patIdx >= len(p.patterns) {
		return
	}
	pat := p.patterns[patIdx]

	for ci := range p.channels {
		ch := &p.channels[ci]
		packed := pat[p.row*p.numChan+ci]

		sampleNum := int(packed >> 24)
		period := uint16((packed >> 12) & 0xFFF)
		effect := uint8((packed >> 8) & 0xF)
		param := uint8(packed & 0xFF)
		xParam := param >> 4
		yParam := param & 0xF

		// Reset per-row state
		ch.volSlide = 0
		ch.arpStep = 0
		ch.arpNotes[0] = ch.period
		ch.playPeriod = ch.period
		ch.tremVol = ch.volume
		ch.cutTick = -1
		ch.delayTick = -1
		ch.delayVolume = -1

		// Load instrument (sample)
		var inst *modSample
		if sampleNum >= 1 && sampleNum <= 31 {
			inst = &p.samples[sampleNum]
		}
		if inst != nil {
			ch.sample = inst
			ch.finetune = inst.finetune
			// Loading a sample sets the volume even without a note
			ch.volume = int(inst.volume)
			ch.tremVol = ch.volume
		}

		// Tone portamento (3xx, 5xy): set target but don't trigger note
		if effect == 0x3 || effect == 0x5 {
			if period != 0 {
				ch.note = noteForPeriod(period)
				ch.portDst = periodForNote(ch.note, ch.finetune)
			}
			if effect == 0x3 && param != 0 {
				ch.portSpd = uint16(param)
			}
			if effect == 0x5 {
				p.setVolSlide(ch, xParam, yParam)
			}
			continue
		}

		// Vibrato + vol slide (6xy): note triggers normally, but slide memory is set now.
		if effect == 0x6 {
			p.setVolSlide(ch, xParam, yParam)
		}
		if effect == 0xA {
			p.setVolSlide(ch, xParam, yParam)
		}

		// Note delay (EDx): store the note for later
		if effect == 0xE && xParam == 0xD {
			if yParam > 0 {
				ch.delayTick = int(yParam)
				if period != 0 && ch.sample != nil {
					ch.delaySample = ch.sample
					ch.delayPeriod = periodForNote(noteForPeriod(period), ch.finetune)
					ch.delayVolume = ch.volume
				}
				continue
			}
		}

		// Trigger note
		if period != 0 {
			fineTune := ch.finetune
			n := noteForPeriod(period)
			ch.note = n
			p2 := periodForNote(n, fineTune)
			ch.portDst = p2
			ch.period = p2
			ch.playPeriod = p2
			if ch.sample != nil {
				if effect == 0x9 {
					ch.pos = float64(ch.sampleOffset)
				} else {
					ch.pos = 0
				}
				ch.active = true
			}
			if !ch.vibRetrig {
				ch.vibPos = 0
			}
			if !ch.tremRetrig {
				ch.tremPos = 0
			}
		}

		// Process effects on tick 0
		p.applyEffect(ch, effect, param, xParam, yParam, true)
	}
}

// processTick handles ticks 1…(speed-1): apply ongoing effects.
func (p *Player) processTick() {
	if p.pos >= p.songLen {
		return
	}
	patIdx := int(p.order[p.pos])
	if patIdx >= len(p.patterns) {
		return
	}
	pat := p.patterns[patIdx]

	for ci := range p.channels {
		ch := &p.channels[ci]
		ch.playPeriod = ch.period
		ch.tremVol = ch.volume
		packed := pat[p.row*p.numChan+ci]

		effect := uint8((packed >> 8) & 0xF)
		param := uint8(packed & 0xFF)
		xParam := param >> 4
		yParam := param & 0xF

		p.applyEffect(ch, effect, param, xParam, yParam, false)
	}
}

// applyEffect applies a MOD effect. tick0 is true only on the note trigger tick.
func (p *Player) applyEffect(ch *modChannel, effect, param, x, y uint8, tick0 bool) {
	switch effect {
	case 0x0: // Arpeggio
		if param == 0 {
			ch.playPeriod = ch.period
			break
		}
		if tick0 {
			n := ch.note
			if n < 0 {
				n = 0
			}
			ft := uint8(0)
			if ch.sample != nil {
				ft = ch.sample.finetune
			}
			ch.arpNotes[0] = periodForNote(n, ft)
			n1 := n + int(x)
			if n1 >= 36 {
				n1 = 35
			}
			ch.arpNotes[1] = periodForNote(n1, ft)
			n2 := n + int(y)
			if n2 >= 36 {
				n2 = 35
			}
			ch.arpNotes[2] = periodForNote(n2, ft)
			ch.arpStep = 0
			ch.playPeriod = ch.arpNotes[0]
		} else {
			ch.arpStep = (ch.arpStep + 1) % 3
			ch.playPeriod = ch.arpNotes[ch.arpStep]
		}

	case 0x1: // Portamento up
		step := param
		if step == 0 {
			step = ch.portaUp
		} else {
			ch.portaUp = step
		}
		if !tick0 {
			ch.period = clampPeriod(int(ch.period) - int(step))
		}
		ch.playPeriod = ch.period

	case 0x2: // Portamento down
		step := param
		if step == 0 {
			step = ch.portaDown
		} else {
			ch.portaDown = step
		}
		if !tick0 {
			ch.period = clampPeriod(int(ch.period) + int(step))
		}
		ch.playPeriod = ch.period

	case 0x3: // Tone portamento
		if !tick0 && ch.portDst != 0 {
			p.doPortamento(ch)
		}
		ch.playPeriod = ch.period

	case 0x4: // Vibrato
		if tick0 {
			if x != 0 {
				ch.vibSpd = int(x)
			}
			if y != 0 {
				ch.vibDep = int(y)
			}
		} else {
			p.doVibrato(ch)
		}

	case 0x5: // Tone portamento + volume slide
		if !tick0 {
			if ch.portDst != 0 {
				p.doPortamento(ch)
			}
			p.doVolSlide(ch)
		}
		ch.playPeriod = ch.period

	case 0x6: // Vibrato + volume slide
		if !tick0 {
			p.doVibrato(ch)
			p.doVolSlide(ch)
		}

	case 0x7: // Tremolo
		if tick0 {
			if x != 0 {
				ch.tremSpd = int(x)
			}
			if y != 0 {
				ch.tremDep = int(y)
			}
		}
		delta := int(waveOutput(ch.tremWave, ch.tremPos)) * ch.tremDep / 64
		ch.tremVol = clampVol(ch.volume + delta)
		if !tick0 {
			ch.tremPos = (ch.tremPos + ch.tremSpd) & 63
		}

	case 0x8: // Set panning (0x00=left, 0x80=centre, 0xFF=right)
		ch.pan = int(param)

	case 0x9: // Sample offset
		if tick0 {
			if param != 0 {
				ch.sampleOffset = int(param) * 256
			}
			ch.pos = float64(ch.sampleOffset)
			ch.active = ch.sample != nil
		}

	case 0xA: // Volume slide
		if !tick0 {
			p.doVolSlide(ch)
		}

	case 0xB: // Position jump
		if tick0 {
			p.nextPos = int(param)
			p.nextRow = 0
		}

	case 0xC: // Set volume
		if tick0 {
			ch.volume = clampVol(int(param))
			ch.tremVol = ch.volume
		}

	case 0xD: // Pattern break
		if tick0 {
			nr := int(x)*10 + int(y)
			if nr >= 64 {
				nr = 0
			}
			if p.nextRow < 0 || nr < p.nextRow {
				p.nextRow = nr
			}
		}

	case 0xE: // Extended effects
		p.applyExtended(ch, x, y, tick0)

	case 0xF: // Set speed / BPM
		if tick0 {
			if param == 0 {
				param = 1
			}
			if param < 32 {
				p.speed = int(param)
			} else {
				p.bpm = int(param)
			}
		}
	}
}

func (p *Player) applyExtended(ch *modChannel, x, y uint8, tick0 bool) {
	switch x {
	case 0x0: // E0x: Set filter (ignored in software)

	case 0x1: // E1x: Fine portamento up
		if tick0 {
			ch.period = clampPeriod(int(ch.period) - int(y))
		}

	case 0x2: // E2x: Fine portamento down
		if tick0 {
			ch.period = clampPeriod(int(ch.period) + int(y))
		}

	case 0x3: // E3x: Glissando (snap tone-portamento to semitone)
		ch.glissando = y != 0

	case 0x4: // E4x: Set vibrato waveform
		ch.vibWave = y & 3
		ch.vibRetrig = (y & 4) != 0

	case 0x5: // E5x: Set finetune
		ch.finetune = y
		if ch.note >= 0 {
			ch.period = periodForNote(ch.note, y)
			ch.playPeriod = ch.period
		}

	case 0x6: // E6x: Pattern loop
		if tick0 {
			if y == 0 {
				ch.loopRow = p.row
				ch.loopArmed = true
			} else if ch.loopArmed {
				if ch.loopCnt == 0 {
					ch.loopCnt = int(y)
				}
				if ch.loopCnt > 0 {
					p.nextRow = ch.loopRow
					ch.loopCnt--
					if ch.loopCnt == 0 {
						ch.loopArmed = false
					}
				}
			}
		}

	case 0x7: // E7x: Set tremolo waveform
		ch.tremWave = y & 3
		ch.tremRetrig = (y & 4) != 0

	case 0x8: // E8x: Set panning (coarse: 0=left, 8=centre, 15=right)
		ch.pan = int(y) * 17

	case 0x9: // E9x: Retrigger note every y ticks
		if tick0 {
			ch.retrigSpd = int(y)
			ch.retrigCnt = 0
		} else if ch.retrigSpd > 0 {
			ch.retrigCnt++
			if ch.retrigCnt >= ch.retrigSpd {
				ch.retrigCnt = 0
				ch.pos = 0
				ch.active = true
			}
		}

	case 0xA: // EAx: Fine volume slide up
		if tick0 {
			ch.volume = clampVol(ch.volume + int(y))
			ch.tremVol = ch.volume
		}

	case 0xB: // EBx: Fine volume slide down
		if tick0 {
			ch.volume = clampVol(ch.volume - int(y))
			ch.tremVol = ch.volume
		}

	case 0xC: // ECx: Note cut at tick y
		if tick0 {
			ch.cutTick = int(y)
		} else if ch.cutTick >= 0 && p.tick == ch.cutTick {
			ch.volume = 0
			ch.tremVol = 0
		}

	case 0xD: // EDx: Note delay — handled in processRow
		if !tick0 && ch.delayTick >= 0 && p.tick == ch.delayTick {
			if ch.delaySample != nil {
				ch.sample = ch.delaySample
				ch.period = ch.delayPeriod
				ch.pos = 0
				ch.active = true
				if ch.delayVolume >= 0 {
					ch.volume = ch.delayVolume
					ch.tremVol = ch.volume
				}
			}
		}

	case 0xE: // EEx: Pattern delay (extra rows)
		if tick0 {
			if p.patDelayCnt == 0 {
				p.patDelayCnt = int(y)
			}
		}

	case 0xF: // EFx: Invert loop (ignored)
	}
}

// setVolSlide decodes Axy volume-slide params and stores the per-tick delta.
func (p *Player) setVolSlide(ch *modChannel, x, y uint8) {
	param := (x << 4) | y
	if param != 0 {
		ch.volSlideMem = param
	} else {
		param = ch.volSlideMem
		x = param >> 4
		y = param & 0x0F
	}
	if x > 0 {
		ch.volSlide = int(x)
	} else {
		ch.volSlide = -int(y)
	}
}

func (p *Player) doVolSlide(ch *modChannel) {
	ch.volume = clampVol(ch.volume + ch.volSlide)
	ch.tremVol = ch.volume
}

func (p *Player) doPortamento(ch *modChannel) {
	if ch.period < ch.portDst {
		ch.period = clampPeriod(int(ch.period) + int(ch.portSpd))
		if ch.period > ch.portDst {
			ch.period = ch.portDst
		}
	} else if ch.period > ch.portDst {
		ch.period = clampPeriod(int(ch.period) - int(ch.portSpd))
		if ch.period < ch.portDst {
			ch.period = ch.portDst
		}
	}
	if ch.glissando {
		n := noteForPeriod(ch.period)
		ch.period = periodForNote(n, ch.finetune)
	}
}

func (p *Player) doVibrato(ch *modChannel) {
	delta := int(waveOutput(ch.vibWave, ch.vibPos)) * ch.vibDep / 128
	ch.playPeriod = clampPeriod(int(ch.period) + delta)
	ch.vibPos = (ch.vibPos + ch.vibSpd) & 63
}

// advanceRow moves to the next row, handling pattern breaks, position jumps, and repeats.
func (p *Player) advanceRow() {
	newPos := p.pos
	newRow := p.row + 1

	if p.nextRow >= 0 {
		newRow = p.nextRow
		p.nextRow = -1
		if p.nextPos < 0 {
			newPos++
		}
	}
	if p.nextPos >= 0 {
		newPos = p.nextPos
		p.nextPos = -1
		if p.nextRow < 0 {
			newRow = 0
		}
	}

	if newRow >= 64 {
		newRow = 0
		newPos++
	}
	if newPos >= p.songLen {
		newPos = p.restart
		newRow = 0
	}

	// Detect repeat by tracking order positions visited
	if newRow == 0 {
		byteIdx := newPos >> 5
		bit := uint32(1) << (uint(newPos) & 31)
		if p.orderMap[byteIdx]&bit != 0 {
			p.repeating = true
		} else {
			p.orderMap[byteIdx] |= bit
		}
	}

	p.pos = newPos
	p.row = newRow
}
