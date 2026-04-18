package it

import (
	"math"
	"strings"

	"github.com/StarHack/go-tracker-formats/formats"
)

type itChannel struct {
	active   bool
	sample   *itSample
	refNote  int
	note     int
	inst     int
	pos      float64
	posStep  float64
	volume   int
	mixVol   int
	panBase  int
	disabled bool
	envTick  int
	volEnv   *itEnvelopeData
	instGVol int

	keyOff         bool
	fadeMul        float64
	fadeOut        int
	freqMul        float64
	vibMul         float64
	vibPos         int
	vibSpd         uint8
	vibDep         uint8
	volSlideMem    uint8
	portDownMem    uint8
	portUpMem      uint8
	toneMem        uint8
	toneTargetNote int

	panBrelloOff int
	panBSpd      uint8
	panBDep      uint8
	panBPos      int
	panSlideMem  uint8

	arpeggOff int
	arpMem    uint8

	tremorOnLen  int
	tremorOffLen int
	tremorPhase  int
	tremorMem    uint8

	tremSpd uint8
	tremDep uint8
	tremPos int
	tremMul float64

	uVibSpd uint8
	uVibDep uint8
	uVibPos int
	uVibMul float64

	nVolMem uint8

	s3CutAfter int

	retrigMem   uint8
	retrigCount int

	noteDeferAt    int
	deferNote      uint8
	deferHasInst   bool
	deferInst      uint8
	deferHasVolPan bool
	deferVolPan    uint8
	deferHasG      bool
	deferPrevNote  int

	glissando bool
	vibWave   uint8
	tremWave  uint8
	panBWave  uint8
	s2Fine    float64 // semitones from S2x (MOD finetune / 96)

	zMacroIdx uint8 // SFx: active parametered Z macro (0–15), default 0

	fltCut    int  // Z00–Z7F (macro 0): 0–127, higher = brighter
	fltRes    int  // Z80–Z8F: 0–127 resonance
	fltHiPass bool // filter mode (custom fixed macros); default lowpass
	fltDirty  bool

	fltB0, fltB1, fltB2, fltA1, fltA2 float64
	fltX1L, fltX2L, fltY1L, fltY2L     float64
	fltX1R, fltX2R, fltY1R, fltY2R     float64
	fltExtRange bool // song extended filter range (IT header 0x1000)

	lastMidiZParam uint8 // last Z / \ parameter for smooth interpolation
	zSmoothActive  bool
	zSmoothFrom    uint8
	zSmoothTo      uint8

	macroDry          float64 // F0F003z: dry path gain (parallel with macroWet)
	macroWet          float64 // F0F003z: wet tap gain (delayed post-filter bus)
	macroPitchSemis   float64 // Ec00z: pitch bend in semitones (macro contribution)
	macroMIDIProgram uint8   // Ccz: last program change from macro
	macroBankMSB     uint8  // CC0 bank select MSB
	macroBankLSB     uint8  // CC32 bank select LSB
	macroPlugParam   [32]uint8 // last F0FnnnZ values by idx&31 (plugin param scratch)
	macroExtLastCC   uint8    // last BC…Z CC number not handled by itApplyMIDICC switch
	macroExtLastVal  uint8    // value for macroExtLastCC
}

type Player struct {
	mod         *Module
	sampleRate  int
	title       string
	channels    []itChannel
	order       int
	row         int
	tick        int
	speed       int
	tempo       int
	samCnt      int
	samPerTick  int
	patRows     int
	nextPos     int
	nextRow     int
	repeating   bool
	songEnded   bool
	initialised bool
	globalVol   int
	masterVol   int

	patternDelayExtra int
	rowTickLimit      int
	loopStartOrder    int
	loopStartRow      int
	loopActive        bool
	jumpLoopAfterRow  bool
	sbJumpBudget      int
	sbArmOrder        int
	sbArmRow          int
	seRepeatRemain    int
	skipNoteOnNextRow bool
	gvolSlideMem      uint8

	// S9D: clear filter on each new note; S9C: global (filter persists across notes).
	localFilters bool

	// Per-channel wet delay line: tap reads a delayed copy of post-filter audio (plugin wet bus approximation).
	wetRing [64]itWetRing
}

const itWetDelaySamples = 4096

type itWetRing struct {
	buf [itWetDelaySamples][2]float64
	i   int
}

func (w *itWetRing) clear() {
	for j := range w.buf {
		w.buf[j][0], w.buf[j][1] = 0, 0
	}
	w.i = 0
}

// readWrite returns the sample written ~delayMs ago, then stores dryL/dryR at the ring head.
func (w *itWetRing) readWrite(sampleRate int, delayMs int, dryL, dryR float64) (wetL, wetR float64) {
	delay := sampleRate * delayMs / 1000
	if delay < 1 {
		delay = 1
	}
	if delay >= itWetDelaySamples {
		delay = itWetDelaySamples - 1
	}
	r := w.i - delay
	for r < 0 {
		r += itWetDelaySamples
	}
	wetL, wetR = w.buf[r][0], w.buf[r][1]
	w.buf[w.i][0], w.buf[w.i][1] = dryL, dryR
	w.i++
	if w.i >= itWetDelaySamples {
		w.i = 0
	}
	return wetL, wetR
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

func itFreq(c5 int, playNote, refNote int) float64 {
	return float64(c5) * math.Pow(2.0, float64(playNote-refNote)/12.0)
}

func (p *Player) calcSamPerTick() int {
	t := p.tempo
	if t <= 0 {
		t = 125
	}
	v := p.sampleRate * 5 / (t * 2)
	if v < 1 {
		v = 1
	}
	return v
}

func (p *Player) currentPattern() *itPattern {
	if p.mod == nil || p.order < 0 || p.order >= len(p.mod.Orders) {
		return nil
	}
	ord := p.mod.Orders[p.order]
	if ord == 254 || ord == 255 {
		return nil
	}
	idx := int(ord)
	if idx < 0 || idx >= len(p.mod.Patterns) {
		return nil
	}
	pp := &p.mod.Patterns[idx]
	return pp
}

func (p *Player) Init(tune []byte, sampleRate int) string {
	p.initialised = false
	if err := Validate(tune); err != nil {
		return err.Error()
	}
	m, err := loadModule(tune)
	if err != nil {
		return err.Error()
	}
	p.mod = m
	p.sampleRate = sampleRate
	p.title = m.Title
	p.Stop()
	p.initialised = true
	return ""
}

func (p *Player) Stop() {
	if p.mod == nil {
		return
	}
	m := p.mod
	p.channels = make([]itChannel, 64)
	for i := range p.channels {
		cv := int(m.ChnVol[i])
		if cv > 64 {
			cv = 64
		}
		p.channels[i].volume = cv
		p.channels[i].mixVol = cv
		pan := int(m.ChnPan[i])
		if pan&128 != 0 {
			p.channels[i].disabled = true
			p.channels[i].panBase = 128
			continue
		}
		pan &= 127
		if pan > 64 {
			pan = 64
		}
		p.channels[i].panBase = pan * 255 / 64
		if p.channels[i].panBase > 255 {
			p.channels[i].panBase = 255
		}
		p.channels[i].disabled = false
		p.channels[i].toneTargetNote = -1
		p.channels[i].s3CutAfter = -1
		p.channels[i].zMacroIdx = 0
		p.channels[i].fltCut = 127
		p.channels[i].fltRes = 0
		p.channels[i].fltHiPass = false
		p.channels[i].fltDirty = true
		p.channels[i].fltExtRange = m.ExtFilterRange
		p.channels[i].lastMidiZParam = 0
		p.channels[i].zSmoothActive = false
		p.channels[i].macroDry = 1
		p.channels[i].macroWet = 0
		p.channels[i].macroPitchSemis = 0
		p.channels[i].macroMIDIProgram = 0
		p.channels[i].macroBankMSB = 0
		p.channels[i].macroBankLSB = 0
		p.channels[i].itFilterResetState()
		p.channels[i].tremMul = 1
		p.channels[i].uVibMul = 1
		p.channels[i].noteDeferAt = -1
	}
	p.globalVol = clampInt(int(m.GV), 0, 128)
	if p.globalVol == 0 {
		p.globalVol = 128
	}
	p.masterVol = clampInt(int(m.MV), 0, 128)
	if p.masterVol == 0 {
		p.masterVol = 128
	}
	spd := int(m.Speed)
	if spd <= 0 {
		spd = 6
	}
	p.speed = clampInt(spd, 1, 255)
	tmp := int(m.Tempo)
	if tmp <= 0 {
		tmp = 125
	}
	p.tempo = clampInt(tmp, 32, 255)
	p.order, p.row, p.tick = 0, 0, 0
	p.samCnt = 0
	p.samPerTick = p.calcSamPerTick()
	p.nextPos, p.nextRow = -1, -1
	p.repeating = false
	p.songEnded = false
	p.patternDelayExtra = 0
	p.rowTickLimit = p.speed
	p.loopActive = false
	p.jumpLoopAfterRow = false
	p.loopStartOrder, p.loopStartRow = 0, 0
	p.sbJumpBudget = 0
	p.sbArmOrder, p.sbArmRow = -1, -1
	p.seRepeatRemain = -1
	p.skipNoteOnNextRow = false
	p.gvolSlideMem = 0
	p.localFilters = false
	if pat := p.currentPattern(); pat != nil {
		p.patRows = pat.Rows
	} else {
		p.patRows = 64
	}
	for i := range p.wetRing {
		p.wetRing[i].clear()
	}
}

func (p *Player) GetDescription() []byte {
	if strings.TrimSpace(p.title) == "" {
		return nil
	}
	return []byte(p.title)
}

func (p *Player) resolveSample(ch *itChannel, inst, note int) (*itSample, int) {
	if !p.mod.UseInstruments {
		if inst < 1 || inst >= len(p.mod.Samples) {
			return nil, 60
		}
		s := &p.mod.Samples[inst]
		if !s.SampleExists || len(s.Data) == 0 {
			return nil, 60
		}
		return s, 60
	}
	if inst < 1 || inst >= len(p.mod.Instruments) {
		return nil, 60
	}
	if note < 0 || note > 119 {
		return nil, 60
	}
	kb := p.mod.Instruments[inst].keyboard[note]
	ref := int(kb[0])
	smpIdx := int(kb[1])
	if smpIdx <= 0 || smpIdx >= len(p.mod.Samples) {
		return nil, ref
	}
	s := &p.mod.Samples[smpIdx]
	if !s.SampleExists || len(s.Data) == 0 {
		return nil, ref
	}
	return s, ref
}

// itApplyMacroProgramChange applies CCZ / program change: bank (CC0/CC32) + program select instrument/sample index.
func (p *Player) itApplyMacroProgramChange(ch *itChannel, prm uint8) {
	ch.macroMIDIProgram = prm
	if p.mod == nil {
		return
	}
	bank := (int(ch.macroBankMSB) << 7) | int(ch.macroBankLSB)
	slot := bank + int(prm)
	var maxIdx int
	if p.mod.UseInstruments {
		maxIdx = len(p.mod.Instruments) - 1
	} else {
		maxIdx = len(p.mod.Samples) - 1
	}
	if maxIdx < 1 {
		return
	}
	maxSlot := maxIdx - 1
	if slot < 0 {
		slot = 0
	}
	if slot > maxSlot {
		slot = maxSlot
	}
	ch.inst = 1 + slot
	if !ch.active || ch.note < 0 || ch.note > 119 {
		return
	}
	smp, ref := p.resolveSample(ch, ch.inst, ch.note)
	if smp == nil {
		ch.active = false
		return
	}
	ch.sample = smp
	ch.refNote = ref
	ch.itFilterMarkDirty()
	ch.itFilterResetState()
}

// itMacroExternalCC records unmapped MIDI CC macros for optional host use.
func (*Player) itMacroExternalCC(ch *itChannel, cc, val int) {
	ch.macroExtLastCC = uint8(cc & 0x7F)
	ch.macroExtLastVal = uint8(clampInt(val, 0, 127))
}

func (p *Player) triggerVolInst(ch *itChannel, cell itCell) {
	if ch.disabled {
		return
	}
	if cell.HasVolPan && cell.VolPan <= 64 {
		ch.volume = int(cell.VolPan)
		ch.mixVol = ch.volume
	}
	if cell.HasInst {
		ch.inst = int(cell.Inst)
	}
}

func (p *Player) triggerNote(ci int, ch *itChannel, cell itCell) {
	if ch.disabled || !cell.HasNote {
		return
	}
	n := int(cell.Note)
	if n == 255 {
		ch.keyOff = true
		return
	}
	if n == 254 {
		ch.active = false
		ch.keyOff = false
		p.wetRing[ci].clear()
		return
	}
	if n > 119 {
		return
	}
	smp, ref := p.resolveSample(ch, ch.inst, n)
	if smp == nil {
		ch.active = false
		p.wetRing[ci].clear()
		return
	}
	ch.sample = smp
	ch.refNote = ref
	ch.note = n
	ch.pos = 0
	ch.keyOff = false
	ch.fadeMul = 1
	ch.fadeOut = 0
	ch.envTick = 0
	ch.volEnv = nil
	ch.instGVol = 128
	ch.freqMul = 1
	ch.vibMul = 1
	ch.uVibMul = 1
	ch.tremMul = 1
	ch.arpeggOff = 0
	ch.s3CutAfter = -1
	ch.retrigCount = 0
	ch.s2Fine = 0
	ch.macroDry = 1
	ch.macroWet = 0
	ch.macroPitchSemis = 0
	if p.mod.UseInstruments && ch.inst >= 1 && ch.inst < len(p.mod.Instruments) {
		ins := &p.mod.Instruments[ch.inst]
		if ins.VolEnv.Valid {
			ch.volEnv = &ins.VolEnv
		}
		gv := ins.GVol
		if gv <= 0 {
			gv = 128
		}
		if gv > 128 {
			gv = 128
		}
		ch.instGVol = gv
		ch.fadeOut = ins.FadeOut
	}
	if !cell.HasVolPan || cell.VolPan > 64 {
		ch.volume = clampInt(smp.DefaultVol, 0, 64)
		if ch.volume == 0 {
			ch.volume = 64
		}
		ch.mixVol = ch.volume
	}
	if p.localFilters {
		ch.fltCut = 127
		ch.fltRes = 0
		ch.fltHiPass = false
		ch.itFilterMarkDirty()
		ch.itFilterResetState()
	} else if p.mod.UseInstruments && ch.inst >= 1 && ch.inst < len(p.mod.Instruments) {
		ins := &p.mod.Instruments[ch.inst]
		ch.fltCut = clampInt(int(ins.FilterCutoff), 0, 127)
		ch.fltRes = clampInt(int(ins.FilterResonance), 0, 127)
		ch.itFilterMarkDirty()
		ch.itFilterResetState()
	}
	if cell.HasCmd && cell.Cmd == 7 && cell.HasNote {
		ch.toneTargetNote = n
	} else {
		ch.toneTargetNote = -1
	}
	ch.active = true
	p.wetRing[ci].clear()
}

func (p *Player) trigger(ci int, ch *itChannel, cell itCell) {
	p.triggerVolInst(ch, cell)
	p.triggerNote(ci, ch, cell)
}

func (p *Player) applyGlobalRowEffects(pat *itPattern, row int) {
	for ci := 0; ci < 64; ci++ {
		cell := pat.Data[row][ci]
		if !cell.HasCmd {
			continue
		}
		cmd := cell.Cmd
		prm := cell.Param
		switch cmd {
		case 1:
			if prm > 0 {
				p.speed = clampInt(int(prm), 1, 255)
			}
		case 2:
			p.nextPos = int(prm)
			p.nextRow = 0
		case 3:
			target := p.order + 1
			nr := int(prm)
			if nr < 0 {
				nr = 0
			}
			if pr := p.patternRowsAtOrderIndex(target); pr > 0 && nr >= pr {
				nr = pr - 1
			}
			p.nextPos = target
			p.nextRow = nr
		case 20:
			if prm >= 32 {
				p.tempo = clampInt(int(prm), 32, 255)
				p.samPerTick = p.calcSamPerTick()
			}
		case 22:
			if prm <= 128 {
				p.globalVol = clampInt(int(prm), 0, 128)
			}
		}
	}
}

// applyGlobalVolumeSlide applies Wxy (cmd 23): like Dxy on global volume (OpenMPT IT).
func (p *Player) applyGlobalVolumeSlide(pat *itPattern, row int, tick0 bool) {
	if pat == nil || row < 0 || row >= pat.Rows {
		return
	}
	var prm uint8
	found := false
	for ci := 0; ci < 64; ci++ {
		c := pat.Data[row][ci]
		if !c.HasCmd || c.Cmd != 23 {
			continue
		}
		prm = c.Param
		found = true
		break
	}
	if !found {
		return
	}
	if tick0 {
		if prm != 0 {
			p.gvolSlideMem = prm
		}
		return
	}
	if prm == 0 {
		prm = p.gvolSlideMem
	}
	if prm == 0 {
		return
	}
	x, y := int(prm>>4), int(prm&0x0F)
	if x > 0 && y == 0 {
		p.globalVol = clampInt(p.globalVol+x, 0, 128)
	} else if y > 0 && x == 0 {
		p.globalVol = clampInt(p.globalVol-y, 0, 128)
	}
}

func (p *Player) processRow() {
	if p.mod == nil {
		return
	}
	for p.order < len(p.mod.Orders) && p.mod.Orders[p.order] == 254 {
		p.order++
	}
	if p.order < 0 || p.order >= len(p.mod.Orders) {
		p.songEnded = true
		return
	}
	ord := p.mod.Orders[p.order]
	if ord == 255 {
		p.songEnded = true
		p.repeating = true
		return
	}
	pat := p.currentPattern()
	if pat == nil || p.row >= pat.Rows {
		return
	}
	p.applyGlobalRowEffects(pat, p.row)
	skipNotes := p.skipNoteOnNextRow
	if skipNotes {
		p.skipNoteOnNextRow = false
	}
	p.scanPatternRowGlobals(pat, p.row, skipNotes)
	p.applyGlobalVolumeSlide(pat, p.row, true)
	for ci := 0; ci < 64; ci++ {
		ch := &p.channels[ci]
		cell := pat.Data[p.row][ci]
		ch.mixVol = ch.volume
		ch.vibMul = 1
		ch.uVibMul = 1
		ch.tremMul = 1
		ch.panBrelloOff = 0
		ch.noteDeferAt = -1
		prevNote := ch.note
		prevActive := ch.active
		p.triggerVolInst(ch, cell)
		if skipNotes {
			p.applyITRowEffects(ch, cell, true)
			p.itArpeggioSetOffset(ch, cell, 0)
			p.itApplySCNoteCut(ch, cell)
			continue
		}
		nd := noteDelayTicks(cell)
		if cell.HasNote && nd > 0 {
			n := int(cell.Note)
			if n <= 119 && n != 254 && n != 255 {
				ch.noteDeferAt = nd
				ch.deferNote = cell.Note
				ch.deferHasInst = cell.HasInst
				ch.deferInst = cell.Inst
				ch.deferHasVolPan = cell.HasVolPan
				ch.deferVolPan = cell.VolPan
				ch.deferHasG = cell.HasCmd && cell.Cmd == 7 && cell.HasNote
				ch.deferPrevNote = prevNote
			} else {
				p.triggerNote(ci, ch, cell)
			}
		} else {
			p.triggerNote(ci, ch, cell)
		}
		if cell.HasCmd && cell.Cmd == 7 && cell.HasNote && prevActive && ch.active && ch.sample != nil {
			n := int(cell.Note)
			if n <= 119 {
				base := itFreq(ch.sample.C5Speed, prevNote, ch.refNote)
				cur := itFreq(ch.sample.C5Speed, ch.note, ch.refNote)
				if base > 0 && cur > 0 {
					ch.freqMul *= base / cur
				}
			}
		}
		ch.vibMul = 1
		ch.uVibMul = 1
		p.applyITRowEffects(ch, cell, true)
		p.itArpeggioSetOffset(ch, cell, 0)
		p.itApplySCNoteCut(ch, cell)
	}
}

func (p *Player) processTickIT() {
	if p.mod == nil {
		return
	}
	pat := p.currentPattern()
	if pat == nil || p.row >= pat.Rows {
		return
	}
	for ci := 0; ci < 64; ci++ {
		ch := &p.channels[ci]
		if ch.disabled {
			continue
		}
		if ch.zSmoothActive && p.tick > 0 {
			ticks := p.rowTickLimit
			if ticks < 2 {
				ticks = 2
			}
			a := float64(p.tick) / float64(ticks-1)
			v := float64(ch.zSmoothFrom) + a*(float64(ch.zSmoothTo)-float64(ch.zSmoothFrom))
			prm := uint8(int(v + 0.5))
			p.itMacroZApply(ch, prm)
		}
		cell := pat.Data[p.row][ci]
		if ch.noteDeferAt >= 0 && p.tick == ch.noteDeferAt {
			syn := itCell{
				HasNote: true, Note: ch.deferNote,
				HasInst: ch.deferHasInst, Inst: ch.deferInst,
				HasVolPan: ch.deferHasVolPan, VolPan: ch.deferVolPan,
			}
			prevActive := ch.active
			p.trigger(ci, ch, syn)
			if ch.deferHasG {
				n := int(ch.deferNote)
				if prevActive && ch.active && ch.sample != nil && n <= 119 {
					ch.toneTargetNote = n
					base := itFreq(ch.sample.C5Speed, ch.deferPrevNote, ch.refNote)
					cur := itFreq(ch.sample.C5Speed, ch.note, ch.refNote)
					if base > 0 && cur > 0 {
						ch.freqMul *= base / cur
					}
				}
				ch.deferHasG = false
			}
			ch.noteDeferAt = -1
		}
		if ch.s3CutAfter >= 0 && p.tick == ch.s3CutAfter {
			ch.active = false
			ch.s3CutAfter = -1
		}
		ch.mixVol = ch.volume
		p.applyITRowEffects(ch, cell, false)
		p.itTremorTick(ch)
		p.itPanbrelloTick(ch, cell)
		p.itArpeggioSetOffset(ch, cell, p.tick)
	}
	p.applyGlobalVolumeSlide(pat, p.row, false)
}

func (p *Player) advanceRow() {
	fromJump := p.nextRow >= 0
	jumpPos := p.nextPos
	newPos := p.order
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
	effectJump := jumpPos >= 0 || fromJump
	p.nextPos, p.nextRow = -1, -1
	if !effectJump && p.jumpLoopAfterRow && p.loopActive {
		if p.sbJumpBudget > 0 {
			newPos = p.loopStartOrder
			newRow = p.loopStartRow
			p.sbJumpBudget--
			if p.sbJumpBudget <= 0 {
				p.loopActive = false
				p.sbArmOrder, p.sbArmRow = -1, -1
			}
		} else {
			p.loopActive = false
			p.sbArmOrder, p.sbArmRow = -1, -1
		}
		p.jumpLoopAfterRow = false
	}
	for newPos < len(p.mod.Orders) && p.mod.Orders[newPos] == 254 {
		newPos++
	}
	if newPos >= len(p.mod.Orders) {
		p.order = 0
		p.row = 0
		p.repeating = true
		p.songEnded = true
		p.refreshPatRows()
		return
	}
	ord := p.mod.Orders[newPos]
	if ord == 255 {
		p.order = newPos
		p.row = 0
		p.repeating = true
		p.songEnded = true
		p.refreshPatRows()
		return
	}
	idx := int(ord)
	if idx < 0 || idx >= len(p.mod.Patterns) {
		newPos++
		p.order = newPos
		p.row = 0
		p.refreshPatRows()
		return
	}
	pr := p.mod.Patterns[idx].Rows
	if newRow >= pr && pr > 0 {
		if fromJump && jumpPos >= 0 {
			newRow = pr - 1
			if newRow < 0 {
				newRow = 0
			}
		} else {
			newRow = 0
			newPos++
			for newPos < len(p.mod.Orders) && p.mod.Orders[newPos] == 254 {
				newPos++
			}
			if newPos >= len(p.mod.Orders) {
				p.order = 0
				p.row = 0
				p.repeating = true
				p.songEnded = true
				p.refreshPatRows()
				return
			}
			if p.mod.Orders[newPos] == 255 {
				p.order = newPos
				p.row = 0
				p.repeating = true
				p.songEnded = true
				p.refreshPatRows()
				return
			}
		}
	}
	p.order, p.row = newPos, newRow
	p.refreshPatRows()
}

func (p *Player) refreshPatRows() {
	if pat := p.currentPattern(); pat != nil {
		p.patRows = pat.Rows
	} else {
		p.patRows = 64
	}
}

func (p *Player) patternRowsAtOrderIndex(orderIdx int) int {
	if p.mod == nil || orderIdx < 0 || orderIdx >= len(p.mod.Orders) {
		return 0
	}
	o := p.mod.Orders[orderIdx]
	if o == 254 || o == 255 {
		return 0
	}
	pi := int(o)
	if pi < 0 || pi >= len(p.mod.Patterns) {
		return 0
	}
	return p.mod.Patterns[pi].Rows
}

// mapITLoopPhys maps monotonic playback phase into a sample frame coordinate for
// interpolation. backward is true on the return leg of a ping-pong loop.
func mapITLoopPhys(s *itSample, phase float64, frames int) (phys float64, backward bool) {
	if s == nil || frames <= 0 {
		return 0, false
	}
	if !s.Looped {
		return phase, false
	}
	Ls := float64(s.LoopStart)
	Le := float64(s.LoopEnd)
	if phase < Ls {
		return phase, false
	}
	Lf := Le - Ls
	if Lf <= 0 {
		if phase < 0 {
			return 0, false
		}
		if phase >= float64(frames) {
			return float64(frames) - 1, false
		}
		return phase, false
	}
	if !s.PingPong {
		u := phase - Ls
		ru := math.Mod(u, Lf)
		if ru < 0 {
			ru += Lf
		}
		if ru >= Lf {
			ru = Lf - 1e-9
		}
		return Ls + ru, false
	}
	u := phase - Ls
	seg := math.Mod(u, 2*Lf)
	if seg >= Lf {
		return Le - 1 - (seg - Lf), true
	}
	return Ls + seg, false
}

func itInterpMono(s *itSample, phys float64, back bool, frames int) float64 {
	if frames <= 0 || len(s.Data) == 0 {
		return 0
	}
	if back {
		ihi := int(math.Ceil(phys))
		if ihi >= frames {
			ihi = frames - 1
		}
		ilo := ihi - 1
		if ilo < 0 {
			ilo = 0
		}
		fr := float64(ihi) - phys
		if fr < 0 {
			fr = 0
		}
		if fr > 1 {
			fr = 1
		}
		return float64(s.Data[ihi])*fr + float64(s.Data[ilo])*(1-fr)
	}
	iLo := int(math.Floor(phys))
	if iLo < 0 {
		iLo = 0
	}
	if iLo >= frames {
		iLo = frames - 1
	}
	iHi := iLo + 1
	if s.Looped && !s.PingPong && iHi >= s.LoopEnd {
		iHi = s.LoopStart
	} else if iHi >= frames {
		iHi = frames - 1
	}
	fr := phys - float64(iLo)
	if fr < 0 {
		fr = 0
	}
	if fr > 1 {
		fr = 1
	}
	return float64(s.Data[iLo])*(1-fr) + float64(s.Data[iHi])*fr
}

func itInterpStereo(s *itSample, phys float64, back bool, frames int) (l, r float64) {
	if frames <= 0 || len(s.Data) < 2 {
		return 0, 0
	}
	if back {
		ihi := int(math.Ceil(phys))
		if ihi >= frames {
			ihi = frames - 1
		}
		ilo := ihi - 1
		if ilo < 0 {
			ilo = 0
		}
		fr := float64(ihi) - phys
		if fr < 0 {
			fr = 0
		}
		if fr > 1 {
			fr = 1
		}
		hi := ihi * 2
		lo := ilo * 2
		if hi+1 >= len(s.Data) || lo+1 >= len(s.Data) {
			return 0, 0
		}
		l = float64(s.Data[hi])*fr + float64(s.Data[lo])*(1-fr)
		r = float64(s.Data[hi+1])*fr + float64(s.Data[lo+1])*(1-fr)
		return l, r
	}
	iLo := int(math.Floor(phys))
	if iLo < 0 {
		iLo = 0
	}
	if iLo >= frames {
		iLo = frames - 1
	}
	iHi := iLo + 1
	if s.Looped && !s.PingPong && iHi >= s.LoopEnd {
		iHi = s.LoopStart
	} else if iHi >= frames {
		iHi = frames - 1
	}
	fr := phys - float64(iLo)
	if fr < 0 {
		fr = 0
	}
	if fr > 1 {
		fr = 1
	}
	bLo := iLo * 2
	bHi := iHi * 2
	if bLo+1 >= len(s.Data) || bHi+1 >= len(s.Data) {
		return 0, 0
	}
	l = float64(s.Data[bLo])*(1-fr) + float64(s.Data[bHi])*fr
	r = float64(s.Data[bLo+1])*(1-fr) + float64(s.Data[bHi+1])*fr
	return l, r
}

func (p *Player) Sample(left, right *int16) bool {
	if !p.initialised || p.mod == nil {
		*left, *right = 0, 0
		return false
	}
	if p.songEnded {
		*left, *right = 0, 0
		return p.repeating
	}
	if p.samCnt == 0 && p.tick == 0 {
		p.processRow()
	} else if p.samCnt == 0 && p.tick > 0 {
		p.processTickIT()
	}
	var lAcc, rAcc int64
	activeCh := 0
	for i := range p.channels {
		ch := &p.channels[i]
		if ch.disabled || !ch.active || ch.sample == nil || len(ch.sample.Data) == 0 || ch.mixVol <= 0 {
			continue
		}
		activeCh++
		playNote := clampInt(ch.note+ch.arpeggOff, 0, 119)
		freq := itFreq(ch.sample.C5Speed, playNote, ch.refNote) * math.Pow(2.0, (ch.s2Fine+ch.macroPitchSemis)/12.0) * ch.freqMul * ch.vibMul * ch.uVibMul
		if freq <= 0 {
			continue
		}
		ch.posStep = freq / float64(p.sampleRate)
		s := ch.sample
		envMul := envelopeAmp(ch.volEnv, ch.envTick, ch.keyOff) * ch.fadeMul
		frames := s.Frames
		if frames <= 0 {
			frames = s.Length
		}
		phys, back := mapITLoopPhys(s, ch.pos, frames)
		if !s.Looped && phys >= float64(frames) {
			ch.active = false
			continue
		}
		if !s.Looped && phys < 0 {
			ch.active = false
			continue
		}

		var smpL, smpR float64
		if s.Stereo {
			smpL, smpR = itInterpStereo(s, phys, back, frames)
		} else {
			smpL = itInterpMono(s, phys, back, frames)
			smpR = smpL
		}
		volScale := float64(ch.mixVol) / 64.0
		volScale *= float64(s.GlobalVol) / 64.0
		volScale *= float64(ch.instGVol) / 128.0
		volScale *= float64(p.globalVol) / 128.0
		volScale *= float64(p.masterVol) / 128.0
		volScale *= envMul
		volScale *= ch.tremMul
		if volScale < 0 {
			volScale = 0
		}
		smpL *= volScale
		smpR *= volScale

		fltL, fltR := ch.itFilterProcessStereo(smpL, smpR, p.sampleRate)
		wetL, wetR := p.wetRing[i].readWrite(p.sampleRate, 42, fltL, fltR)
		smpL = ch.macroDry*fltL + ch.macroWet*wetL
		smpR = ch.macroDry*fltR + ch.macroWet*wetR

		pan := ch.panBase + ch.panBrelloOff
		if pan < 0 {
			pan = 0
		}
		if pan > 255 {
			pan = 255
		}
		pl := float64(255-pan) / 255.0
		pr := float64(pan) / 255.0
		lAcc += int64(smpL * pl)
		rAcc += int64(smpR * pr)

		ch.pos += ch.posStep
		if !s.Looped && ch.pos >= float64(frames) {
			ch.active = false
		}
	}
	div := int64(activeCh)
	if div < 1 {
		div = 1
	}
	*left = clamp16(lAcc / div)
	*right = clamp16(rAcc / div)

	p.samCnt++
	if p.samCnt >= p.samPerTick {
		p.samCnt = 0
		for i := range p.channels {
			ch := &p.channels[i]
			if ch.active {
				ch.envTick++
				if ch.keyOff && ch.fadeOut > 0 {
					ch.fadeMul -= float64(ch.fadeOut) / 65536.0
					if ch.fadeMul < 0 {
						ch.fadeMul = 0
					}
					if ch.fadeMul <= 0 {
						ch.active = false
					}
				}
			}
		}
		p.tick++
		if p.tick >= p.rowTickLimit {
			for i := range p.channels {
				ch := &p.channels[i]
				if ch.zSmoothActive {
					ch.lastMidiZParam = ch.zSmoothTo
					ch.zSmoothActive = false
					if p.rowTickLimit <= 1 {
						p.itMacroZApply(ch, ch.zSmoothTo)
					}
				}
			}
			p.tick = 0
			if p.seRepeatRemain > 0 {
				p.seRepeatRemain--
				p.skipNoteOnNextRow = true
			} else {
				p.advanceRow()
			}
		}
	}
	return p.repeating
}
