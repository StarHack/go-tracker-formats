package it

import "math"

// IT Zxx / default OpenMPT-style resonant filter (see Manual: Zxx Macros).
// Z00–Z7F with macro 0: cutoff; Z80–Z8F: resonance; SFx selects parametered macro index.

func (ch *itChannel) itFilterBypass() bool {
	return ch.fltCut >= 127 && ch.fltRes <= 0 && !ch.fltHiPass
}

func (ch *itChannel) itFilterResetState() {
	ch.fltX1L, ch.fltX2L, ch.fltY1L, ch.fltY2L = 0, 0, 0, 0
	ch.fltX1R, ch.fltX2R, ch.fltY1R, ch.fltY2R = 0, 0, 0, 0
}

func (ch *itChannel) itFilterMarkDirty() {
	ch.fltDirty = true
}

func rbjLowpassCoefs(fcHz, q, sampleRate float64) (b0, b1, b2, a1, a2 float64) {
	if fcHz <= 0 || sampleRate <= 0 || q <= 0 {
		return 1, 0, 0, 0, 0
	}
	if fcHz >= sampleRate*0.49 {
		fcHz = sampleRate * 0.49
	}
	w0 := 2 * math.Pi * fcHz / sampleRate
	cosw0 := math.Cos(w0)
	sinw0 := math.Sin(w0)
	alpha := sinw0 / (2 * q)
	a0 := 1 + alpha
	b0 = ((1 - cosw0) / 2) / a0
	b1 = (1 - cosw0) / a0
	b2 = ((1 - cosw0) / 2) / a0
	a1 = (-2 * cosw0) / a0
	a2 = (1 - alpha) / a0
	return b0, b1, b2, a1, a2
}

func rbjHighpassCoefs(fcHz, q, sampleRate float64) (b0, b1, b2, a1, a2 float64) {
	if fcHz <= 0 || sampleRate <= 0 || q <= 0 {
		return 1, 0, 0, 0, 0
	}
	if fcHz >= sampleRate*0.49 {
		fcHz = sampleRate * 0.49
	}
	w0 := 2 * math.Pi * fcHz / sampleRate
	cosw0 := math.Cos(w0)
	sinw0 := math.Sin(w0)
	alpha := sinw0 / (2 * q)
	a0 := 1 + alpha
	b0 = ((1 + cosw0) / 2) / a0
	b1 = -(1 + cosw0) / a0
	b2 = ((1 + cosw0) / 2) / a0
	a1 = (-2 * cosw0) / a0
	a2 = (1 - alpha) / a0
	return b0, b1, b2, a1, a2
}

func (ch *itChannel) itFilterUpdateCoefs(sampleRate int) {
	sr := float64(sampleRate)
	if sampleRate <= 0 {
		ch.fltB0, ch.fltB1, ch.fltB2, ch.fltA1, ch.fltA2 = 1, 0, 0, 0, 0
		ch.fltDirty = false
		return
	}
	if ch.itFilterBypass() {
		ch.fltB0, ch.fltB1, ch.fltB2, ch.fltA1, ch.fltA2 = 1, 0, 0, 0, 0
		ch.fltDirty = false
		return
	}
	// Map IT cutoff 0..127 (dark..bright) to corner frequency.
	fcMin := 130.0
	fcMax := sr * 0.46
	if ch.fltExtRange {
		fcMin = 45.0
		fcMax = sr * 0.49
	}
	t := float64(ch.fltCut) / 127.0
	if t < 0 {
		t = 0
	}
	if t > 1 {
		t = 1
	}
	fc := fcMin * math.Pow(fcMax/fcMin, t)
	q := 0.5 + float64(ch.fltRes)/127.0*14.0
	if q < 0.51 {
		q = 0.51
	}
	if ch.fltHiPass {
		ch.fltB0, ch.fltB1, ch.fltB2, ch.fltA1, ch.fltA2 = rbjHighpassCoefs(fc, q, sr)
	} else {
		ch.fltB0, ch.fltB1, ch.fltB2, ch.fltA1, ch.fltA2 = rbjLowpassCoefs(fc, q, sr)
	}
	ch.fltDirty = false
}

func (ch *itChannel) itFilterProcessStereo(inL, inR float64, sampleRate int) (outL, outR float64) {
	if ch.itFilterBypass() {
		return inL, inR
	}
	if ch.fltDirty {
		ch.itFilterUpdateCoefs(sampleRate)
	}
	b0, b1, b2, a1, a2 := ch.fltB0, ch.fltB1, ch.fltB2, ch.fltA1, ch.fltA2
	outL = b0*inL + b1*ch.fltX1L + b2*ch.fltX2L - a1*ch.fltY1L - a2*ch.fltY2L
	ch.fltX2L, ch.fltX1L = ch.fltX1L, inL
	ch.fltY2L, ch.fltY1L = ch.fltY1L, outL

	outR = b0*inR + b1*ch.fltX1R + b2*ch.fltX2R - a1*ch.fltY1R - a2*ch.fltY2R
	ch.fltX2R, ch.fltX1R = ch.fltX1R, inR
	ch.fltY2R, ch.fltY1R = ch.fltY1R, outR
	return outL, outR
}
