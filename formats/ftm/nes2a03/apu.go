// Copyright (c) 2014 Michael Fogleman
// SPDX-License-Identifier: MIT
//
// Derived from github.com/fogleman/nes nes/apu.go — decoupled from Console/CPU, gob save removed,
// DMC bus access via MemRead callback, no APU IRQ delivery, no output filter chain.

package nes2a03

import "math"

// CPUFrequency is the NTSC 2A03 CPU clock used for the APU (Hz).
const CPUFrequency = 1789773

var frameCounterRate = CPUFrequency / 240.0

var lengthTable = []byte{
	10, 254, 20, 2, 40, 4, 80, 6, 160, 8, 60, 10, 14, 12, 26, 14,
	12, 16, 24, 18, 48, 20, 96, 22, 192, 24, 72, 26, 16, 28, 32, 30,
}

var dutyTable = [][]byte{
	{0, 1, 0, 0, 0, 0, 0, 0},
	{0, 1, 1, 0, 0, 0, 0, 0},
	{0, 1, 1, 1, 1, 0, 0, 0},
	{1, 0, 0, 1, 1, 1, 1, 1},
}

var triangleTable = []byte{
	15, 14, 13, 12, 11, 10, 9, 8, 7, 6, 5, 4, 3, 2, 1, 0,
	0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15,
}

var noiseTable = []uint16{
	4, 8, 16, 32, 64, 96, 128, 160, 202, 254, 380, 508, 762, 1016, 2034, 4068,
}

var dmcTable = []byte{
	214, 190, 170, 160, 143, 127, 113, 107, 95, 80, 71, 64, 53, 42, 36, 27,
}

var pulseTable [31]float32
var tndTable [203]float32

func init() {
	for i := 0; i < 31; i++ {
		pulseTable[i] = 95.52 / (8128.0/float32(i) + 100)
	}
	for i := 0; i < 203; i++ {
		tndTable[i] = 163.67 / (24329.0/float32(i) + 100)
	}
}

// MemRead supplies DPCM DMA reads ($8000–$FFFF address space as seen by the DMC unit).
type MemRead func(addr uint16) byte

// APU is the five-channel 2A03 audio unit.
type APU struct {
	MemRead MemRead

	sampleCycle float64 // CPU cycles per output sample
	pulse1      Pulse
	pulse2      Pulse
	triangle    Triangle
	noise       Noise
	dmc         DMC
	cycle       uint64
	framePeriod byte
	frameValue  byte
	frameIRQ    bool
}

// New creates an APU stepped at NTSC CPU rate, sampled at sampleRateHz (e.g. 44100).
func New(sampleRateHz float64) *APU {
	a := &APU{}
	a.noise.shiftRegister = 1
	a.pulse1.channel = 1
	a.pulse2.channel = 2
	a.framePeriod = 4
	a.SetSampleRate(sampleRateHz)
	return a
}

// SetSampleRate recomputes the resampling ratio (may be called after New).
func (a *APU) SetSampleRate(sampleRateHz float64) {
	if sampleRateHz <= 0 {
		sampleRateHz = 44100
	}
	a.sampleCycle = CPUFrequency / sampleRateHz
}

// WriteRegister performs an APU register write ($4000–$4017).
func (a *APU) WriteRegister(address uint16, value byte) {
	switch address {
	case 0x4000:
		a.pulse1.writeControl(value)
	case 0x4001:
		a.pulse1.writeSweep(value)
	case 0x4002:
		a.pulse1.writeTimerLow(value)
	case 0x4003:
		a.pulse1.writeTimerHigh(value)
	case 0x4004:
		a.pulse2.writeControl(value)
	case 0x4005:
		a.pulse2.writeSweep(value)
	case 0x4006:
		a.pulse2.writeTimerLow(value)
	case 0x4007:
		a.pulse2.writeTimerHigh(value)
	case 0x4008:
		a.triangle.writeControl(value)
	case 0x4009:
	case 0x4010:
		a.dmc.writeControl(value)
	case 0x4011:
		a.dmc.writeValue(value)
	case 0x4012:
		a.dmc.writeAddress(value)
	case 0x4013:
		a.dmc.writeLength(value)
	case 0x400A:
		a.triangle.writeTimerLow(value)
	case 0x400B:
		a.triangle.writeTimerHigh(value)
	case 0x400C:
		a.noise.writeControl(value)
	case 0x400D:
	case 0x400E:
		a.noise.writePeriod(value)
	case 0x400F:
		a.noise.writeLength(value)
	case 0x4015:
		a.writeControl(value)
	case 0x4017:
		a.writeFrameCounter(value)
	}
}

// ReadStatus reads $4015.
func (a *APU) ReadStatus() byte {
	var result byte
	if a.pulse1.lengthValue > 0 {
		result |= 1
	}
	if a.pulse2.lengthValue > 0 {
		result |= 2
	}
	if a.triangle.lengthValue > 0 {
		result |= 4
	}
	if a.noise.lengthValue > 0 {
		result |= 8
	}
	if a.dmc.currentLength > 0 {
		result |= 16
	}
	return result
}

func (a *APU) writeControl(value byte) {
	a.pulse1.enabled = value&1 == 1
	a.pulse2.enabled = value&2 == 2
	a.triangle.enabled = value&4 == 4
	a.noise.enabled = value&8 == 8
	a.dmc.enabled = value&16 == 16
	if !a.pulse1.enabled {
		a.pulse1.lengthValue = 0
	}
	if !a.pulse2.enabled {
		a.pulse2.lengthValue = 0
	}
	if !a.triangle.enabled {
		a.triangle.lengthValue = 0
	}
	if !a.noise.enabled {
		a.noise.lengthValue = 0
	}
	if !a.dmc.enabled {
		a.dmc.currentLength = 0
	} else if a.dmc.currentLength == 0 {
		a.dmc.restart()
	}
}

func (a *APU) writeFrameCounter(value byte) {
	a.framePeriod = 4 + (value>>7)&1
	a.frameIRQ = (value>>6)&1 == 0
	if a.framePeriod == 5 {
		a.stepEnvelope()
		a.stepSweep()
		a.stepLength()
	}
}

func (a *APU) stepFrameCounter() {
	switch a.framePeriod {
	case 4:
		a.frameValue = (a.frameValue + 1) % 4
		switch a.frameValue {
		case 0, 2:
			a.stepEnvelope()
		case 1:
			a.stepEnvelope()
			a.stepSweep()
			a.stepLength()
		case 3:
			a.stepEnvelope()
			a.stepSweep()
			a.stepLength()
			a.fireIRQ()
		}
	case 5:
		a.frameValue = (a.frameValue + 1) % 5
		switch a.frameValue {
		case 0, 2:
			a.stepEnvelope()
		case 1, 3:
			a.stepEnvelope()
			a.stepSweep()
			a.stepLength()
		}
	}
}

func (a *APU) fireIRQ() { /* not wired in stand-alone music player */ }

func (a *APU) stepEnvelope() {
	a.pulse1.stepEnvelope()
	a.pulse2.stepEnvelope()
	a.triangle.stepCounter()
	a.noise.stepEnvelope()
}

func (a *APU) stepSweep() {
	a.pulse1.stepSweep()
	a.pulse2.stepSweep()
}

func (a *APU) stepLength() {
	a.pulse1.stepLength()
	a.pulse2.stepLength()
	a.triangle.stepLength()
	a.noise.stepLength()
}

func (a *APU) stepTimer() {
	if a.cycle%2 == 0 {
		a.pulse1.stepTimer()
		a.pulse2.stepTimer()
		a.noise.stepTimer()
		a.dmc.stepTimer(a.MemRead)
	}
	a.triangle.stepTimer()
}

// Step advances one CPU cycle. If ok is true, out is a new mixed mono sample in approximately ±1.
func (a *APU) Step() (ok bool, out float32) {
	cycle1 := a.cycle
	a.cycle++
	a.stepTimer()
	f1 := int(float64(cycle1) / frameCounterRate)
	f2 := int(float64(a.cycle) / frameCounterRate)
	if f1 != f2 {
		a.stepFrameCounter()
	}
	s1 := int(float64(cycle1) / a.sampleCycle)
	s2 := int(float64(a.cycle) / a.sampleCycle)
	if s1 != s2 {
		return true, a.mix()
	}
	return false, 0
}

func (a *APU) mix() float32 {
	p1 := a.pulse1.output()
	p2 := a.pulse2.output()
	t := a.triangle.output()
	n := a.noise.output()
	d := a.dmc.output()
	pulseOut := pulseTable[p1+p2]
	tndOut := tndTable[3*t+2*n+d]
	return pulseOut + tndOut
}

// --- Pulse ---

type Pulse struct {
	enabled         bool
	channel         byte
	lengthEnabled   bool
	lengthValue     byte
	timerPeriod     uint16
	timerValue      uint16
	dutyMode        byte
	dutyValue       byte
	sweepReload     bool
	sweepEnabled    bool
	sweepNegate     bool
	sweepShift      byte
	sweepPeriod     byte
	sweepValue      byte
	envelopeEnabled bool
	envelopeLoop    bool
	envelopeStart   bool
	envelopePeriod  byte
	envelopeValue   byte
	envelopeVolume  byte
	constantVolume  byte
}

func (p *Pulse) writeControl(value byte) {
	p.dutyMode = (value >> 6) & 3
	p.lengthEnabled = (value>>5)&1 == 0
	p.envelopeLoop = (value>>5)&1 == 1
	p.envelopeEnabled = (value>>4)&1 == 0
	p.envelopePeriod = value & 15
	p.constantVolume = value & 15
	p.envelopeStart = true
}

func (p *Pulse) writeSweep(value byte) {
	p.sweepEnabled = (value>>7)&1 == 1
	p.sweepPeriod = (value>>4)&7 + 1
	p.sweepNegate = (value>>3)&1 == 1
	p.sweepShift = value & 7
	p.sweepReload = true
}

func (p *Pulse) writeTimerLow(value byte) {
	p.timerPeriod = (p.timerPeriod & 0xFF00) | uint16(value)
}

func (p *Pulse) writeTimerHigh(value byte) {
	p.lengthValue = lengthTable[value>>3]
	p.timerPeriod = (p.timerPeriod & 0x00FF) | (uint16(value&7) << 8)
	p.envelopeStart = true
	p.dutyValue = 0
}

func (p *Pulse) stepTimer() {
	if p.timerValue == 0 {
		p.timerValue = p.timerPeriod
		p.dutyValue = (p.dutyValue + 1) % 8
	} else {
		p.timerValue--
	}
}

func (p *Pulse) stepEnvelope() {
	if p.envelopeStart {
		p.envelopeVolume = 15
		p.envelopeValue = p.envelopePeriod
		p.envelopeStart = false
	} else if p.envelopeValue > 0 {
		p.envelopeValue--
	} else {
		if p.envelopeVolume > 0 {
			p.envelopeVolume--
		} else if p.envelopeLoop {
			p.envelopeVolume = 15
		}
		p.envelopeValue = p.envelopePeriod
	}
}

func (p *Pulse) stepSweep() {
	if p.sweepReload {
		if p.sweepEnabled && p.sweepValue == 0 {
			p.sweep()
		}
		p.sweepValue = p.sweepPeriod
		p.sweepReload = false
	} else if p.sweepValue > 0 {
		p.sweepValue--
	} else {
		if p.sweepEnabled {
			p.sweep()
		}
		p.sweepValue = p.sweepPeriod
	}
}

func (p *Pulse) stepLength() {
	if p.lengthEnabled && p.lengthValue > 0 {
		p.lengthValue--
	}
}

func (p *Pulse) sweep() {
	delta := p.timerPeriod >> p.sweepShift
	if p.sweepNegate {
		p.timerPeriod -= delta
		if p.channel == 1 {
			p.timerPeriod--
		}
	} else {
		p.timerPeriod += delta
	}
}

func (p *Pulse) output() byte {
	if !p.enabled {
		return 0
	}
	if p.lengthValue == 0 {
		return 0
	}
	if dutyTable[p.dutyMode][p.dutyValue] == 0 {
		return 0
	}
	if p.timerPeriod < 8 || p.timerPeriod > 0x7FF {
		return 0
	}
	if p.envelopeEnabled {
		return p.envelopeVolume
	}
	return p.constantVolume
}

// --- Triangle ---

type Triangle struct {
	enabled       bool
	lengthEnabled bool
	lengthValue   byte
	timerPeriod   uint16
	timerValue    uint16
	dutyValue     byte
	counterPeriod byte
	counterValue  byte
	counterReload bool
}

func (t *Triangle) writeControl(value byte) {
	t.lengthEnabled = (value>>7)&1 == 0
	t.counterPeriod = value & 0x7F
}

func (t *Triangle) writeTimerLow(value byte) {
	t.timerPeriod = (t.timerPeriod & 0xFF00) | uint16(value)
}

func (t *Triangle) writeTimerHigh(value byte) {
	t.lengthValue = lengthTable[value>>3]
	t.timerPeriod = (t.timerPeriod & 0x00FF) | (uint16(value&7) << 8)
	t.timerValue = t.timerPeriod
	t.counterReload = true
}

func (t *Triangle) stepTimer() {
	if t.timerValue == 0 {
		t.timerValue = t.timerPeriod
		if t.lengthValue > 0 && t.counterValue > 0 {
			t.dutyValue = (t.dutyValue + 1) % 32
		}
	} else {
		t.timerValue--
	}
}

func (t *Triangle) stepLength() {
	if t.lengthEnabled && t.lengthValue > 0 {
		t.lengthValue--
	}
}

func (t *Triangle) stepCounter() {
	if t.counterReload {
		t.counterValue = t.counterPeriod
	} else if t.counterValue > 0 {
		t.counterValue--
	}
	if t.lengthEnabled {
		t.counterReload = false
	}
}

func (t *Triangle) output() byte {
	if !t.enabled {
		return 0
	}
	if t.timerPeriod < 3 {
		return 0
	}
	if t.lengthValue == 0 {
		return 0
	}
	if t.counterValue == 0 {
		return 0
	}
	return triangleTable[t.dutyValue]
}

// --- Noise ---

type Noise struct {
	enabled         bool
	mode            bool
	shiftRegister   uint16
	lengthEnabled   bool
	lengthValue     byte
	timerPeriod     uint16
	timerValue      uint16
	envelopeEnabled bool
	envelopeLoop    bool
	envelopeStart   bool
	envelopePeriod  byte
	envelopeValue   byte
	envelopeVolume  byte
	constantVolume  byte
}

func (n *Noise) writeControl(value byte) {
	n.lengthEnabled = (value>>5)&1 == 0
	n.envelopeLoop = (value>>5)&1 == 1
	n.envelopeEnabled = (value>>4)&1 == 0
	n.envelopePeriod = value & 15
	n.constantVolume = value & 15
	n.envelopeStart = true
}

func (n *Noise) writePeriod(value byte) {
	n.mode = value&0x80 == 0x80
	n.timerPeriod = noiseTable[value&0x0F]
}

func (n *Noise) writeLength(value byte) {
	n.lengthValue = lengthTable[value>>3]
	n.envelopeStart = true
}

func (n *Noise) stepTimer() {
	if n.timerValue == 0 {
		n.timerValue = n.timerPeriod
		var shift byte
		if n.mode {
			shift = 6
		} else {
			shift = 1
		}
		b1 := n.shiftRegister & 1
		b2 := (n.shiftRegister >> shift) & 1
		n.shiftRegister >>= 1
		n.shiftRegister |= (b1 ^ b2) << 14
	} else {
		n.timerValue--
	}
}

func (n *Noise) stepEnvelope() {
	if n.envelopeStart {
		n.envelopeVolume = 15
		n.envelopeValue = n.envelopePeriod
		n.envelopeStart = false
	} else if n.envelopeValue > 0 {
		n.envelopeValue--
	} else {
		if n.envelopeVolume > 0 {
			n.envelopeVolume--
		} else if n.envelopeLoop {
			n.envelopeVolume = 15
		}
		n.envelopeValue = n.envelopePeriod
	}
}

func (n *Noise) stepLength() {
	if n.lengthEnabled && n.lengthValue > 0 {
		n.lengthValue--
	}
}

func (n *Noise) output() byte {
	if !n.enabled {
		return 0
	}
	if n.lengthValue == 0 {
		return 0
	}
	if n.shiftRegister&1 == 1 {
		return 0
	}
	if n.envelopeEnabled {
		return n.envelopeVolume
	}
	return n.constantVolume
}

// --- DMC ---

type DMC struct {
	enabled        bool
	value          byte
	sampleAddress  uint16
	sampleLength   uint16
	currentAddress uint16
	currentLength  uint16
	shiftRegister  byte
	bitCount       byte
	tickPeriod     byte
	tickValue      byte
	loop           bool
	irq            bool
}

func (d *DMC) writeControl(value byte) {
	d.irq = value&0x80 == 0x80
	d.loop = value&0x40 == 0x40
	d.tickPeriod = dmcTable[value&0x0F]
}

func (d *DMC) writeValue(value byte) {
	d.value = value & 0x7F
}

func (d *DMC) writeAddress(value byte) {
	d.sampleAddress = 0xC000 | (uint16(value) << 6)
}

func (d *DMC) writeLength(value byte) {
	d.sampleLength = (uint16(value) << 4) | 1
}

func (d *DMC) restart() {
	d.currentAddress = d.sampleAddress
	d.currentLength = d.sampleLength
}

func (d *DMC) stepTimer(read MemRead) {
	if !d.enabled {
		return
	}
	d.stepReader(read)
	if d.tickValue == 0 {
		d.tickValue = d.tickPeriod
		d.stepShifter()
	} else {
		d.tickValue--
	}
}

func (d *DMC) stepReader(read MemRead) {
	if d.currentLength > 0 && d.bitCount == 0 {
		if read != nil {
			d.shiftRegister = read(d.currentAddress)
		} else {
			d.shiftRegister = 0
		}
		d.bitCount = 8
		d.currentAddress++
		if d.currentAddress == 0 {
			d.currentAddress = 0x8000
		}
		d.currentLength--
		if d.currentLength == 0 && d.loop {
			d.restart()
		}
	}
}

func (d *DMC) stepShifter() {
	if d.bitCount == 0 {
		return
	}
	if d.shiftRegister&1 == 1 {
		if d.value <= 125 {
			d.value += 2
		}
	} else {
		if d.value >= 2 {
			d.value -= 2
		}
	}
	d.shiftRegister >>= 1
	d.bitCount--
}

func (d *DMC) output() byte {
	return d.value
}

// FloatToPCM scales the non-linear DAC mix to int16 (mono).
func FloatToPCM(v float32) int16 {
	x := float64(v) * 32000
	if x > 32767 {
		x = 32767
	}
	if x < -32768 {
		x = -32768
	}
	return int16(math.Round(x))
}
