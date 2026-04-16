// opal.go - Pure Go port of the Opal OPL3 emulator by Reality (opal.cpp).
package opal

const opl3SampleRate int32 = 49716

const (
	envOff = -1
	envAtt = 0
	envDec = 1
	envSus = 2
	envRel = 3
)

var opalRateTables = [4][8]uint16{
	{1, 0, 1, 0, 1, 0, 1, 0},
	{1, 0, 1, 0, 0, 0, 1, 0},
	{1, 0, 0, 0, 1, 0, 0, 0},
	{1, 0, 0, 0, 0, 0, 0, 0},
}

var opalExpTable = [256]uint16{
	1018, 1013, 1007, 1002, 996, 991, 986, 980, 975, 969, 964, 959, 953, 948, 942, 937,
	932, 927, 921, 916, 911, 906, 900, 895, 890, 885, 880, 874, 869, 864, 859, 854,
	849, 844, 839, 834, 829, 824, 819, 814, 809, 804, 799, 794, 789, 784, 779, 774,
	770, 765, 760, 755, 750, 745, 741, 736, 731, 726, 722, 717, 712, 708, 703, 698,
	693, 689, 684, 680, 675, 670, 666, 661, 657, 652, 648, 643, 639, 634, 630, 625,
	621, 616, 612, 607, 603, 599, 594, 590, 585, 581, 577, 572, 568, 564, 560, 555,
	551, 547, 542, 538, 534, 530, 526, 521, 517, 513, 509, 505, 501, 496, 492, 488,
	484, 480, 476, 472, 468, 464, 460, 456, 452, 448, 444, 440, 436, 432, 428, 424,
	420, 416, 412, 409, 405, 401, 397, 393, 389, 385, 382, 378, 374, 370, 367, 363,
	359, 355, 352, 348, 344, 340, 337, 333, 329, 326, 322, 318, 315, 311, 308, 304,
	300, 297, 293, 290, 286, 283, 279, 276, 272, 268, 265, 262, 258, 255, 251, 248,
	244, 241, 237, 234, 231, 227, 224, 220, 217, 214, 210, 207, 204, 200, 197, 194,
	190, 187, 184, 181, 177, 174, 171, 168, 164, 161, 158, 155, 152, 148, 145, 142,
	139, 136, 133, 130, 126, 123, 120, 117, 114, 111, 108, 105, 102, 99, 96, 93,
	90, 87, 84, 81, 78, 75, 72, 69, 66, 63, 60, 57, 54, 51, 48, 45,
	42, 40, 37, 34, 31, 28, 25, 22, 20, 17, 14, 11, 8, 6, 3, 0,
}

var opalLogSinTable = [256]uint16{
	2137, 1731, 1543, 1419, 1326, 1252, 1190, 1137, 1091, 1050, 1013, 979, 949, 920, 894, 869,
	846, 825, 804, 785, 767, 749, 732, 717, 701, 687, 672, 659, 646, 633, 621, 609,
	598, 587, 576, 566, 556, 546, 536, 527, 518, 509, 501, 492, 484, 476, 468, 461,
	453, 446, 439, 432, 425, 418, 411, 405, 399, 392, 386, 380, 375, 369, 363, 358,
	352, 347, 341, 336, 331, 326, 321, 316, 311, 307, 302, 297, 293, 289, 284, 280,
	276, 271, 267, 263, 259, 255, 251, 248, 244, 240, 236, 233, 229, 226, 222, 219,
	215, 212, 209, 205, 202, 199, 196, 193, 190, 187, 184, 181, 178, 175, 172, 169,
	167, 164, 161, 159, 156, 153, 151, 148, 146, 143, 141, 138, 136, 134, 131, 129,
	127, 125, 122, 120, 118, 116, 114, 112, 110, 108, 106, 104, 102, 100, 98, 96,
	94, 92, 91, 89, 87, 85, 83, 82, 80, 78, 77, 75, 74, 72, 70, 69,
	67, 66, 64, 63, 62, 60, 59, 57, 56, 55, 53, 52, 51, 49, 48, 47,
	46, 45, 43, 42, 41, 40, 39, 38, 37, 36, 35, 34, 33, 32, 31, 30,
	29, 28, 27, 26, 25, 24, 23, 23, 22, 21, 20, 20, 19, 18, 17, 17,
	16, 15, 15, 14, 13, 13, 12, 12, 11, 10, 10, 9, 9, 8, 8, 7,
	7, 7, 6, 6, 5, 5, 5, 4, 4, 4, 3, 3, 3, 2, 2, 2,
	2, 1, 1, 1, 1, 1, 1, 1, 0, 0, 0, 0, 0, 0, 0, 0,
}

// opalOperator represents a single FM operator.
type opalOperator struct {
	master         *Opal
	ch             *opalChannel
	phase          uint32
	waveform       uint16
	freqMultTimes2 uint16
	envelopeStage  int
	envelopeLevel  int16
	outputLevel    uint16
	attackRate     uint16
	decayRate      uint16
	sustainLevel   uint16
	releaseRate    uint16
	attackShift    uint16
	attackMask     uint16
	attackAdd      uint16
	attackTabIdx   int
	decayShift     uint16
	decayMask      uint16
	decayAdd       uint16
	decayTabIdx    int
	releaseShift   uint16
	releaseMask    uint16
	releaseAdd     uint16
	releaseTabIdx  int
	keyScaleShift  uint16
	keyScaleLevel  uint16
	out            [2]int16
	keyOn          bool
	keyScaleRate   bool
	sustainMode    bool
	tremoloEnable  bool
	vibratoEnable  bool
}

// opalChannel represents a single OPL3 channel.
type opalChannel struct {
	master         *Opal
	op             [4]*opalOperator
	freq           uint16
	octave         uint16
	phaseStep      uint32
	keyScaleNumber uint16
	feedbackShift  uint16
	modulationType uint16
	channelPair    *opalChannel
	enable         bool
	leftEnable     bool
	rightEnable    bool
}

// Opal is the OPL3 emulator.
type Opal struct {
	sampleRate   int32
	sampleAccum  int32
	lastOutput   [2]int16
	currOutput   [2]int16
	ch           [18]opalChannel
	op           [36]opalOperator
	clock        uint16
	tremoloClock uint16
	tremoloLevel uint16
	vibratoTick  uint16
	vibratoClock uint16
	noteSel      bool
	tremoloDepth bool
	vibratoDepth bool
}

// newOpal creates and initialises a new OPL3 emulator at the given sample rate.
func New(sampleRate int) *Opal {
	o := &Opal{}
	o.opalInit(sampleRate)
	return o
}

func (o *Opal) opalInit(sampleRate int) {
	o.clock = 0
	o.tremoloClock = 0
	o.vibratoTick = 0
	o.vibratoClock = 0
	o.noteSel = false
	o.tremoloDepth = false
	o.vibratoDepth = false

	for i := range o.op {
		o.op[i].master = o
		o.op[i].freqMultTimes2 = 1
		o.op[i].envelopeStage = envOff
		o.op[i].envelopeLevel = 0x1FF
		o.op[i].keyScaleShift = 15
	}
	for i := range o.ch {
		o.ch[i].master = o
		o.ch[i].enable = true
	}

	chanOps := [18]int{0, 1, 2, 6, 7, 8, 12, 13, 14, 18, 19, 20, 24, 25, 26, 30, 31, 32}
	for i := range o.ch {
		ch := &o.ch[i]
		op := chanOps[i]
		if i < 3 || (i >= 9 && i < 12) {
			ch.op[0] = &o.op[op]
			ch.op[1] = &o.op[op+3]
			ch.op[2] = &o.op[op+6]
			ch.op[3] = &o.op[op+9]
		} else {
			ch.op[0] = &o.op[op]
			ch.op[1] = &o.op[op+3]
		}
		for j := 0; j < 4; j++ {
			if ch.op[j] != nil {
				ch.op[j].ch = ch
			}
		}
	}

	for i := range o.op {
		o.op[i].computeRates()
	}
	o.opalSetSampleRate(sampleRate)
}

func (o *Opal) opalSetSampleRate(sampleRate int) {
	if sampleRate == 0 {
		sampleRate = int(opl3SampleRate)
	}
	o.sampleRate = int32(sampleRate)
	o.sampleAccum = 0
	o.lastOutput = [2]int16{}
	o.currOutput = [2]int16{}
}

// Port writes a value to an OPL3 register.
func (o *Opal) Port(regNum uint16, val uint8) {
	opLookup := [32]int{
		0, 1, 2, 3, 4, 5, -1, -1, 6, 7, 8, 9, 10, 11, -1, -1,
		12, 13, 14, 15, 16, 17, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1,
	}
	typ := regNum & 0xE0

	if regNum == 0xBD {
		o.tremoloDepth = (val & 0x80) != 0
		o.vibratoDepth = (val & 0x40) != 0
		return
	}

	if typ == 0x00 {
		if regNum == 0x104 {
			mask := uint8(1)
			for i := 0; i < 6; i++ {
				chanIdx := i
				if i >= 3 {
					chanIdx = i + 6
				}
				primary := &o.ch[chanIdx]
				secondary := &o.ch[chanIdx+3]
				if val&mask != 0 {
					primary.channelPair = secondary
					secondary.enable = false
				} else {
					primary.channelPair = nil
					secondary.enable = true
				}
				mask <<= 1
			}
		} else if regNum == 0x08 {
			o.noteSel = (val & 0x40) != 0
			for i := range o.ch {
				o.ch[i].computeKeyScaleNumber()
			}
		}
		return
	}

	if typ >= 0xA0 && typ <= 0xC0 {
		chanNum := int(regNum & 15)
		if chanNum >= 9 {
			return
		}
		if regNum&0x100 != 0 {
			chanNum += 9
		}
		ch := &o.ch[chanNum]
		switch regNum & 0xF0 {
		case 0xA0:
			ch.setFrequencyLow(uint16(val))
		case 0xB0:
			ch.setKeyOn((val & 0x20) != 0)
			ch.setOctave(uint16(val>>2) & 7)
			ch.setFrequencyHigh(uint16(val & 3))
		case 0xC0:
			ch.rightEnable = (val & 0x20) != 0
			ch.leftEnable = (val & 0x10) != 0
			ch.setFeedback(uint16(val>>1) & 7)
			ch.setModulationType(uint16(val & 1))
		}
		return
	}

	if (typ >= 0x20 && typ <= 0x80) || typ == 0xE0 {
		opIdx := opLookup[regNum&0x1F]
		if opIdx < 0 {
			return
		}
		if regNum&0x100 != 0 {
			opIdx += 18
		}
		op := &o.op[opIdx]
		switch typ {
		case 0x20:
			op.tremoloEnable = (val & 0x80) != 0
			op.vibratoEnable = (val & 0x40) != 0
			op.sustainMode = (val & 0x20) != 0
			op.keyScaleRate = (val & 0x10) != 0
			op.computeRates()
			op.setFrequencyMultiplier(uint16(val & 15))
		case 0x40:
			op.setKeyScale(uint16(val >> 6))
			op.setOutputLevel(uint16(val & 0x3F))
		case 0x60:
			op.attackRate = uint16(val >> 4)
			op.decayRate = uint16(val & 15)
			op.computeRates()
		case 0x80:
			op.setSustainLevel(uint16(val >> 4))
			op.releaseRate = uint16(val & 15)
			op.computeRates()
		case 0xE0:
			op.waveform = uint16(val & 7)
		}
	}
}

// Sample generates one stereo sample pair at the configured sample rate.
func (o *Opal) Sample(left, right *int16) {
	for o.sampleAccum >= o.sampleRate {
		o.lastOutput[0] = o.currOutput[0]
		o.lastOutput[1] = o.currOutput[1]
		o.opalOutput(&o.currOutput[0], &o.currOutput[1])
		o.sampleAccum -= o.sampleRate
	}
	omblend := o.sampleRate - o.sampleAccum
	*left = int16((int32(o.lastOutput[0])*omblend + int32(o.currOutput[0])*o.sampleAccum) / o.sampleRate)
	*right = int16((int32(o.lastOutput[1])*omblend + int32(o.currOutput[1])*o.sampleAccum) / o.sampleRate)
	o.sampleAccum += opl3SampleRate
}

func (o *Opal) opalOutput(left, right *int16) {
	var leftMix, rightMix int32
	for i := range o.ch {
		var l, r int16
		o.ch[i].chanOutput(&l, &r)
		leftMix += int32(l)
		rightMix += int32(r)
	}
	if leftMix < -0x8000 {
		*left = -0x8000
	} else if leftMix > 0x7FFF {
		*left = 0x7FFF
	} else {
		*left = int16(leftMix)
	}
	if rightMix < -0x8000 {
		*right = -0x8000
	} else if rightMix > 0x7FFF {
		*right = 0x7FFF
	} else {
		*right = int16(rightMix)
	}

	o.clock++

	// Tremolo: 13440-sample triangle wave
	o.tremoloClock = (o.tremoloClock + 1) % 13440
	if o.tremoloClock < 13440/2 {
		o.tremoloLevel = o.tremoloClock / 256
	} else {
		o.tremoloLevel = (13440 - o.tremoloClock) / 256
	}
	if !o.tremoloDepth {
		o.tremoloLevel >>= 2
	}

	// Vibrato: advances every 1024 OPL3 samples
	o.vibratoTick++
	if o.vibratoTick >= 1024 {
		o.vibratoTick = 0
		o.vibratoClock = (o.vibratoClock + 1) & 7
	}
}

// ---------------------------------------------------------------------------
// Channel methods
// ---------------------------------------------------------------------------

func (ch *opalChannel) chanOutput(left, right *int16) {
	if !ch.enable {
		*left = 0
		*right = 0
		return
	}

	vibrato := int16((ch.freq >> 7) & 7)
	if !ch.master.vibratoDepth {
		vibrato >>= 1
	}
	clk := ch.master.vibratoClock
	if clk&3 == 0 {
		vibrato = 0
	} else {
		if clk&1 != 0 {
			vibrato >>= 1
		}
		if clk&4 != 0 {
			vibrato = -vibrato
		}
	}
	vibrato <<= ch.octave

	var out, acc int16

	if ch.channelPair != nil {
		secMod := ch.channelPair.modulationType
		if secMod == 0 {
			if ch.modulationType == 0 {
				out = ch.op[0].opOutput(ch.keyScaleNumber, ch.phaseStep, vibrato, 0, ch.feedbackShift)
				out = ch.op[1].opOutput(ch.keyScaleNumber, ch.phaseStep, vibrato, out, 0)
				out = ch.op[2].opOutput(ch.keyScaleNumber, ch.phaseStep, vibrato, out, 0)
				out = ch.op[3].opOutput(ch.keyScaleNumber, ch.phaseStep, vibrato, out, 0)
			} else {
				out = ch.op[0].opOutput(ch.keyScaleNumber, ch.phaseStep, vibrato, 0, ch.feedbackShift)
				acc = ch.op[1].opOutput(ch.keyScaleNumber, ch.phaseStep, vibrato, 0, 0)
				acc = ch.op[2].opOutput(ch.keyScaleNumber, ch.phaseStep, vibrato, acc, 0)
				out += ch.op[3].opOutput(ch.keyScaleNumber, ch.phaseStep, vibrato, acc, 0)
			}
		} else {
			if ch.modulationType == 0 {
				out = ch.op[0].opOutput(ch.keyScaleNumber, ch.phaseStep, vibrato, 0, ch.feedbackShift)
				out = ch.op[1].opOutput(ch.keyScaleNumber, ch.phaseStep, vibrato, out, 0)
				acc = ch.op[2].opOutput(ch.keyScaleNumber, ch.phaseStep, vibrato, 0, 0)
				out += ch.op[3].opOutput(ch.keyScaleNumber, ch.phaseStep, vibrato, acc, 0)
			} else {
				out = ch.op[0].opOutput(ch.keyScaleNumber, ch.phaseStep, vibrato, 0, ch.feedbackShift)
				acc = ch.op[1].opOutput(ch.keyScaleNumber, ch.phaseStep, vibrato, 0, 0)
				out += ch.op[2].opOutput(ch.keyScaleNumber, ch.phaseStep, vibrato, acc, 0)
				out += ch.op[3].opOutput(ch.keyScaleNumber, ch.phaseStep, vibrato, 0, 0)
			}
		}
	} else {
		if ch.modulationType == 0 {
			out = ch.op[0].opOutput(ch.keyScaleNumber, ch.phaseStep, vibrato, 0, ch.feedbackShift)
			out = ch.op[1].opOutput(ch.keyScaleNumber, ch.phaseStep, vibrato, out, 0)
		} else {
			out = ch.op[0].opOutput(ch.keyScaleNumber, ch.phaseStep, vibrato, 0, ch.feedbackShift)
			out += ch.op[1].opOutput(ch.keyScaleNumber, ch.phaseStep, vibrato, 0, 0)
		}
	}

	if ch.leftEnable {
		*left = out
	} else {
		*left = 0
	}
	if ch.rightEnable {
		*right = out
	} else {
		*right = 0
	}
}

func (ch *opalChannel) setFrequencyLow(freq uint16) {
	ch.freq = (ch.freq & 0x300) | (freq & 0xFF)
	ch.computePhaseStep()
}

func (ch *opalChannel) setFrequencyHigh(freq uint16) {
	ch.freq = (ch.freq & 0xFF) | ((freq & 3) << 8)
	ch.computePhaseStep()
	ch.computeKeyScaleNumber()
}

func (ch *opalChannel) setOctave(oct uint16) {
	ch.octave = oct & 7
	ch.computePhaseStep()
	ch.computeKeyScaleNumber()
}

func (ch *opalChannel) setKeyOn(on bool) {
	ch.op[0].opSetKeyOn(on)
	ch.op[1].opSetKeyOn(on)
}

func (ch *opalChannel) setFeedback(val uint16) {
	if val != 0 {
		ch.feedbackShift = 9 - val
	} else {
		ch.feedbackShift = 0
	}
}

func (ch *opalChannel) setModulationType(t uint16) {
	ch.modulationType = t
}

func (ch *opalChannel) computePhaseStep() {
	ch.phaseStep = uint32(ch.freq) << ch.octave
}

func (ch *opalChannel) computeKeyScaleNumber() {
	var lsb uint16
	if ch.master.noteSel {
		lsb = ch.freq >> 9
	} else {
		lsb = (ch.freq >> 8) & 1
	}
	ch.keyScaleNumber = ch.octave<<1 | lsb
	for i := 0; i < 4; i++ {
		if ch.op[i] == nil {
			continue
		}
		ch.op[i].computeRates()
		ch.op[i].computeKeyScaleLevel()
	}
}

// ---------------------------------------------------------------------------
// Operator methods
// ---------------------------------------------------------------------------

func (op *opalOperator) opOutput(keyScaleNum uint16, phaseStep uint32, vibrato int16, mod int16, fbshift uint16) int16 {
	if op.vibratoEnable {
		phaseStep += uint32(vibrato)
	}
	op.phase += (phaseStep * uint32(op.freqMultTimes2)) / 2

	trem := uint16(0)
	if op.tremoloEnable {
		trem = op.master.tremoloLevel
	}
	level := (uint16(op.envelopeLevel) + op.outputLevel + op.keyScaleLevel + trem) << 3

	switch op.envelopeStage {
	case envAtt:
		if op.attackRate != 0 {
			if op.attackMask == 0 || (op.master.clock&op.attackMask) == 0 {
				tabVal := opalRateTables[op.attackTabIdx][(op.master.clock>>op.attackShift)&7]
				add := uint16((int32(op.attackAdd>>tabVal) * int32(^op.envelopeLevel)) >> 3)
				op.envelopeLevel = int16(uint16(op.envelopeLevel) + add)
				if op.envelopeLevel <= 0 {
					op.envelopeLevel = 0
					op.envelopeStage = envDec
				}
			}
		}
	case envDec:
		if op.decayRate != 0 {
			if op.decayMask == 0 || (op.master.clock&op.decayMask) == 0 {
				tabVal := opalRateTables[op.decayTabIdx][(op.master.clock>>op.decayShift)&7]
				add := uint16(op.decayAdd >> tabVal)
				op.envelopeLevel = int16(uint16(op.envelopeLevel) + add)
				if op.envelopeLevel >= int16(op.sustainLevel) {
					op.envelopeLevel = int16(op.sustainLevel)
					op.envelopeStage = envSus
				}
			}
		}
	case envSus:
		if op.sustainMode {
			break
		}
		// not sustaining: fall through to release
		fallthrough
	case envRel:
		if op.releaseRate != 0 {
			if op.releaseMask == 0 || (op.master.clock&op.releaseMask) == 0 {
				tabVal := opalRateTables[op.releaseTabIdx][(op.master.clock>>op.releaseShift)&7]
				add := uint16(op.releaseAdd >> tabVal)
				op.envelopeLevel = int16(uint16(op.envelopeLevel) + add)
				if op.envelopeLevel >= 0x1FF {
					op.envelopeLevel = 0x1FF
					op.envelopeStage = envOff
					op.out[0] = 0
					op.out[1] = 0
					return 0
				}
			}
		}
	default: // envOff
		op.out[0] = 0
		op.out[1] = 0
		return 0
	}

	if fbshift != 0 {
		mod += (op.out[0] + op.out[1]) >> fbshift
	}

	phase := uint16(op.phase>>10) + uint16(mod)
	offset := phase & 0xFF
	var logsin uint16
	negate := false

	switch op.waveform {
	case 0:
		if phase&0x100 != 0 {
			offset ^= 0xFF
		}
		logsin = opalLogSinTable[offset]
		negate = (phase & 0x200) != 0
	case 1:
		if phase&0x200 != 0 {
			offset = 0
		} else if phase&0x100 != 0 {
			offset ^= 0xFF
		}
		logsin = opalLogSinTable[offset]
	case 2:
		if phase&0x100 != 0 {
			offset ^= 0xFF
		}
		logsin = opalLogSinTable[offset]
	case 3:
		if phase&0x100 != 0 {
			offset = 0
		}
		logsin = opalLogSinTable[offset]
	case 4:
		if phase&0x200 != 0 {
			offset = 0
		} else {
			if phase&0x80 != 0 {
				offset ^= 0xFF
			}
			offset = (offset + offset) & 0xFF
			negate = (phase & 0x100) != 0
		}
		logsin = opalLogSinTable[offset]
	case 5:
		if phase&0x200 != 0 {
			offset = 0
		} else {
			offset = (offset + offset) & 0xFF
			if phase&0x80 != 0 {
				offset ^= 0xFF
			}
		}
		logsin = opalLogSinTable[offset]
	case 6:
		logsin = 0
		negate = (phase & 0x200) != 0
	default:
		logsin = phase & 0x1FF
		if phase&0x200 != 0 {
			logsin ^= 0x1FF
			negate = true
		}
		logsin <<= 3
	}

	mix := logsin + level
	if mix > 0x1FFF {
		mix = 0x1FFF
	}

	v := int16(opalExpTable[mix&0xFF]) + 1024
	v >>= mix >> 8
	v += v
	if negate {
		v = ^v
	}

	op.out[1] = op.out[0]
	op.out[0] = v
	return v
}

func (op *opalOperator) opSetKeyOn(on bool) {
	if op.keyOn == on {
		return
	}
	op.keyOn = on
	if on {
		if op.attackRate == 15 {
			op.envelopeStage = envDec
			op.envelopeLevel = 0
		} else {
			op.envelopeStage = envAtt
		}
		op.phase = 0
	} else {
		if op.envelopeStage != envOff && op.envelopeStage != envRel {
			op.envelopeStage = envRel
		}
	}
}

func (op *opalOperator) setFrequencyMultiplier(scale uint16) {
	mulTimes2 := [16]uint16{1, 2, 4, 6, 8, 10, 12, 14, 16, 18, 20, 20, 24, 24, 30, 30}
	op.freqMultTimes2 = mulTimes2[scale&15]
}

func (op *opalOperator) setKeyScale(scale uint16) {
	if scale > 0 {
		op.keyScaleShift = 3 - scale
	} else {
		op.keyScaleShift = 15
	}
	op.computeKeyScaleLevel()
}

func (op *opalOperator) setOutputLevel(level uint16) {
	op.outputLevel = level * 4
}

func (op *opalOperator) setSustainLevel(level uint16) {
	if level < 15 {
		op.sustainLevel = uint16(level)
	} else {
		op.sustainLevel = 31
	}
	op.sustainLevel *= 16
}

func (op *opalOperator) computeRates() {
	if op.ch == nil {
		return
	}
	ksrShift := uint16(2)
	if op.keyScaleRate {
		ksrShift = 0
	}

	computeRate := func(rate uint16) (shift, mask, add uint16, tabIdx int) {
		cr := int(rate)*4 + int(op.ch.keyScaleNumber>>ksrShift)
		rh := cr >> 2
		rl := cr & 3
		if rh < 12 {
			shift = uint16(12 - rh)
		}
		mask = (1 << shift) - 1
		if rh < 12 {
			add = 1
		} else {
			add = 1 << uint(rh-12)
		}
		tabIdx = rl
		return
	}

	op.attackShift, op.attackMask, op.attackAdd, op.attackTabIdx = computeRate(op.attackRate)
	if op.attackRate == 15 {
		op.attackAdd = 0xFFF
	}
	op.decayShift, op.decayMask, op.decayAdd, op.decayTabIdx = computeRate(op.decayRate)
	op.releaseShift, op.releaseMask, op.releaseAdd, op.releaseTabIdx = computeRate(op.releaseRate)
}

func (op *opalOperator) computeKeyScaleLevel() {
	if op.ch == nil {
		return
	}
	levtab := [128]uint16{
		0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0,
		0, 0, 0, 0, 0, 0, 0, 0, 0, 8, 12, 16, 20, 24, 28, 32,
		0, 0, 0, 0, 0, 12, 20, 28, 32, 40, 44, 48, 52, 56, 60, 64,
		0, 0, 0, 20, 32, 44, 52, 60, 64, 72, 76, 80, 84, 88, 92, 96,
		0, 0, 32, 52, 64, 76, 84, 92, 96, 104, 108, 112, 116, 120, 124, 128,
		0, 32, 64, 84, 96, 108, 116, 124, 128, 136, 140, 144, 148, 152, 156, 160,
		0, 64, 96, 116, 128, 140, 148, 156, 160, 168, 172, 176, 180, 184, 188, 192,
		0, 96, 128, 148, 160, 172, 180, 188, 192, 200, 204, 208, 212, 216, 220, 224,
	}
	i := (op.ch.octave << 4) | (op.ch.freq >> 6)
	if i >= 128 {
		i = 127
	}
	op.keyScaleLevel = levtab[i] >> op.keyScaleShift
}
