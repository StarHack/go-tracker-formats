package it

import "math"

// itLFOWave returns a waveform sample in [-1,1] for S3M/IT vibrato/tremolo/panbrello (OpenMPT).
func itLFOWave(wave uint8, pos int) float64 {
	p := pos & 63
	switch wave & 3 {
	case 0:
		return math.Sin(float64(p) * 2.0 * math.Pi / 64.0)
	case 1:
		return 1.0 - 2.0*float64(p)/63.0
	case 2:
		if p&32 != 0 {
			return -1.0
		}
		return 1.0
	default:
		return float64((p*1664525)&255)/127.5 - 1.0
	}
}

var itS2FineTab = [16]int8{0, 1, 2, 3, 4, 5, 6, 7, -8, -7, -6, -5, -4, -3, -2, -1}

// itQRetrigApplyVol adjusts channel volume from Qxy high nibble (OpenMPT Retrigger Volume table).
func itQRetrigApplyVol(vol int, hi int) int {
	switch hi {
	case 0, 8:
		return vol
	case 1:
		return vol - 1
	case 2:
		return vol - 2
	case 3:
		return vol - 4
	case 4:
		return vol - 8
	case 5:
		return vol - 16
	case 6:
		return int(float64(vol)*2.0/3.0 + 0.5)
	case 7:
		return vol / 2
	case 9:
		return vol + 1
	case 10:
		return vol + 2
	case 11:
		return vol + 4
	case 12:
		return vol + 8
	case 13:
		return vol + 16
	case 14:
		return int(float64(vol)*1.5 + 0.5)
	case 15:
		return vol * 2
	default:
		return vol
	}
}

func (p *Player) applyITRowEffects(ch *itChannel, cell itCell, tick0 bool) {
	if ch.disabled || !cell.HasCmd {
		return
	}
	p.applyITChannelEffect(ch, cell.Cmd, cell.Param, tick0)
}

func (p *Player) applyITChannelEffect(ch *itChannel, cmd, prm uint8, tick0 bool) {
	if ch.disabled {
		return
	}
	x, y := int(prm>>4), int(prm&0x0F)
	switch cmd {
	case 4:
		if ch.sample == nil {
			return
		}
		if !tick0 {
			p.itVolSlide(ch, prm)
		}
	case 5:
		if ch.sample == nil {
			return
		}
		if tick0 && (prm&0xF0) == 0xF0 {
			p.itFinePortDown(ch, int(prm&0x0F))
			return
		}
		if prm != 0 && (prm&0xF0) != 0xF0 {
			ch.portDownMem = prm
		}
		mem := prm
		if mem == 0 || (mem&0xF0) == 0xF0 {
			mem = ch.portDownMem
		}
		if (mem & 0xF0) == 0xF0 {
			return
		}
		if !tick0 {
			p.itPortDown(ch, mem)
		}
	case 6:
		if ch.sample == nil {
			return
		}
		if tick0 && (prm&0xF0) == 0xF0 {
			p.itFinePortUp(ch, int(prm&0x0F))
			return
		}
		if prm != 0 && (prm&0xF0) != 0xF0 {
			ch.portUpMem = prm
		}
		mem := prm
		if mem == 0 || (mem&0xF0) == 0xF0 {
			mem = ch.portUpMem
		}
		if (mem & 0xF0) == 0xF0 {
			return
		}
		if !tick0 {
			p.itPortUp(ch, mem)
		}
	case 7:
		if ch.sample == nil {
			return
		}
		if prm != 0 {
			ch.toneMem = prm
		}
		if !tick0 {
			p.itTonePorta(ch)
		}
	case 8:
		if ch.sample == nil {
			return
		}
		if tick0 {
			if x != 0 {
				ch.vibSpd = uint8(x)
			}
			if y != 0 {
				ch.vibDep = uint8(y)
			}
		} else {
			p.itVibrato(ch)
		}
	case 9:
		if ch.sample == nil {
			return
		}
		if tick0 {
			if prm != 0 {
				ch.tremorMem = prm
			} else {
				prm = ch.tremorMem
			}
			ch.tremorOnLen = int(prm >> 4)
			ch.tremorOffLen = int(prm & 0x0F)
			if ch.tremorOnLen <= 0 {
				ch.tremorOnLen = 1
			}
			if ch.tremorOffLen <= 0 {
				ch.tremorOffLen = 1
			}
			ch.tremorPhase = 0
		}
	case 10:
		if ch.sample == nil {
			return
		}
		if tick0 && prm != 0 {
			ch.arpMem = prm
		}
	case 11:
		if ch.sample == nil {
			return
		}
		if tick0 {
			if x != 0 {
				ch.vibSpd = uint8(x)
			}
			if y != 0 {
				ch.vibDep = uint8(y)
			}
			if prm != 0 {
				ch.volSlideMem = prm
			}
		} else {
			p.itVibrato(ch)
			p.itVolSlide(ch, ch.volSlideMem)
		}
	case 12:
		if ch.sample == nil {
			return
		}
		if tick0 {
			if prm != 0 {
				ch.toneMem = prm
				ch.volSlideMem = prm
			}
		} else {
			p.itTonePorta(ch)
			p.itVolSlide(ch, ch.volSlideMem)
		}
	case 13:
		if tick0 {
			ch.volume = clampInt(int(prm), 0, 64)
			ch.mixVol = ch.volume
		}
	case 14:
		if tick0 {
			if prm != 0 {
				ch.nVolMem = prm
			}
			return
		}
		mem := prm
		if mem == 0 {
			mem = ch.nVolMem
		}
		if mem == 0 {
			return
		}
		x := int(mem >> 4)
		y := int(mem & 0x0F)
		ch.volume = clampInt(ch.volume+x-y, 0, 64)
		ch.mixVol = ch.volume
	case 15:
		if ch.sample == nil {
			return
		}
		if tick0 {
			p.itSampleOffset(ch, prm)
		}
	case 16:
		if tick0 && prm != 0 {
			ch.panSlideMem = prm
		}
		if !tick0 {
			p.itPanSlide(ch, prm)
		}
	case 17:
		if ch.sample == nil {
			return
		}
		if tick0 {
			if prm != 0 {
				ch.retrigMem = prm
			}
			ch.retrigCount = 0
		} else {
			p.itRetrig(ch)
		}
	case 18:
		if ch.sample == nil {
			return
		}
		if tick0 {
			if x != 0 {
				ch.tremSpd = uint8(x)
			}
			if y != 0 {
				ch.tremDep = uint8(y)
			}
		} else {
			p.itTremolo(ch)
		}
	case 19:
		if tick0 {
			p.itSpecial(ch, prm)
		}
	case 21:
		if ch.sample == nil {
			return
		}
		if tick0 {
			if x != 0 {
				ch.uVibSpd = uint8(x)
			}
			if y != 0 {
				ch.uVibDep = uint8(y)
			}
		} else {
			p.itFineVibrato(ch)
		}
	case 24:
		if tick0 {
			pv := int(prm)
			if pv > 64 {
				pv = 64
			}
			if pv < 0 {
				pv = 0
			}
			ch.panBase = pv * 255 / 64
			if ch.panBase > 255 {
				ch.panBase = 255
			}
		}
	case 25:
		if tick0 {
			if x != 0 {
				ch.panBSpd = uint8(x)
			}
			if y != 0 {
				ch.panBDep = uint8(y)
			}
		}
	case 26:
		if tick0 {
			p.itMacroZ(ch, prm)
		}
	case 32, 92: // smooth MIDI macro (\xx); 92 = ASCII '\'
		if tick0 {
			p.itMacroSmooth(ch, prm)
		}
	}
}

func (p *Player) itArpeggioSetOffset(ch *itChannel, cell itCell, tick int) {
	if ch.sample == nil || !cell.HasCmd || cell.Cmd != 10 {
		ch.arpeggOff = 0
		return
	}
	prm := cell.Param
	if prm != 0 {
		ch.arpMem = prm
	} else {
		prm = ch.arpMem
	}
	if prm == 0 {
		ch.arpeggOff = 0
		return
	}
	x := int(prm >> 4)
	y := int(prm & 0x0F)
	switch tick % 3 {
	case 0:
		ch.arpeggOff = 0
	case 1:
		ch.arpeggOff = x
	case 2:
		ch.arpeggOff = y
	}
}

func (p *Player) itApplySCNoteCut(ch *itChannel, cell itCell) {
	if !cell.HasCmd || cell.Cmd != 19 {
		return
	}
	if cell.Param>>4 != 0x0C {
		return
	}
	x := int(cell.Param & 0x0F)
	ch.s3CutAfter = x
	if x == 0 {
		ch.active = false
		ch.s3CutAfter = -1
	}
}

func (p *Player) itTremorTick(ch *itChannel) {
	if ch.sample == nil || !ch.active {
		return
	}
	if ch.tremorOnLen <= 0 || ch.tremorOffLen <= 0 {
		return
	}
	on := ch.tremorOnLen
	off := ch.tremorOffLen
	cycle := on + off
	ph := ch.tremorPhase % cycle
	if ph < on {
		ch.mixVol = ch.volume
	} else {
		ch.mixVol = 0
	}
	ch.tremorPhase++
}

func (p *Player) itPanbrelloTick(ch *itChannel, cell itCell) {
	if ch.panBDep == 0 {
		ch.panBrelloOff = 0
		return
	}
	delta := itLFOWave(ch.panBWave, ch.panBPos) * float64(ch.panBDep) * 4.0
	ch.panBrelloOff = int(delta)
	ch.panBPos = (ch.panBPos + int(ch.panBSpd)) & 63
}

func (p *Player) itVolSlide(ch *itChannel, prm uint8) {
	if prm != 0 {
		ch.volSlideMem = prm
	} else {
		prm = ch.volSlideMem
	}
	x, y := int(prm>>4), int(prm&0x0F)
	if x > 0 && y == 0 {
		ch.volume = clampInt(ch.volume+x, 0, 64)
	} else if y > 0 && x == 0 {
		ch.volume = clampInt(ch.volume-y, 0, 64)
	}
	ch.mixVol = ch.volume
}

func (p *Player) itFinePortDown(ch *itChannel, low int) {
	if low <= 0 {
		return
	}
	ch.freqMul /= math.Pow(2.0, float64(low)/(768.0*4.0))
	if ch.freqMul < 0.01 {
		ch.freqMul = 0.01
	}
}

func (p *Player) itFinePortUp(ch *itChannel, low int) {
	if low <= 0 {
		return
	}
	ch.freqMul *= math.Pow(2.0, float64(low)/(768.0*4.0))
}

func (p *Player) itPortDown(ch *itChannel, mem uint8) {
	if p.mod != nil && p.mod.LinearSlides {
		ch.freqMul -= float64(mem) * (1.0 / 768.0)
		if ch.freqMul < 0.01 {
			ch.freqMul = 0.01
		}
		return
	}
	ch.freqMul /= math.Pow(2.0, float64(mem)/768.0)
}

func (p *Player) itPortUp(ch *itChannel, mem uint8) {
	if p.mod != nil && p.mod.LinearSlides {
		ch.freqMul += float64(mem) * (1.0 / 768.0)
		return
	}
	ch.freqMul *= math.Pow(2.0, float64(mem)/768.0)
}

func (p *Player) itTonePorta(ch *itChannel) {
	if ch.toneTargetNote < 0 || ch.toneTargetNote > 119 || ch.sample == nil {
		return
	}
	tgt := itFreq(ch.sample.C5Speed, ch.toneTargetNote, ch.refNote)
	cur := itFreq(ch.sample.C5Speed, ch.note, ch.refNote) * ch.freqMul
	if tgt <= 0 || cur <= 0 {
		return
	}
	stepMem := ch.toneMem
	if stepMem == 0 {
		stepMem = 4
	}
	var step float64
	if ch.glissando {
		step = math.Pow(2.0, 1.0/12.0)
	} else {
		step = math.Pow(2.0, float64(stepMem)/768.0)
	}
	if cur < tgt {
		ch.freqMul *= step
		if itFreq(ch.sample.C5Speed, ch.note, ch.refNote)*ch.freqMul >= tgt {
			ch.freqMul = tgt / itFreq(ch.sample.C5Speed, ch.note, ch.refNote)
			ch.note = ch.toneTargetNote
			ch.toneTargetNote = -1
		}
	} else if cur > tgt {
		ch.freqMul /= step
		if itFreq(ch.sample.C5Speed, ch.note, ch.refNote)*ch.freqMul <= tgt {
			ch.freqMul = tgt / itFreq(ch.sample.C5Speed, ch.note, ch.refNote)
			ch.note = ch.toneTargetNote
			ch.toneTargetNote = -1
		}
	} else {
		ch.toneTargetNote = -1
	}
}

func (p *Player) itVibrato(ch *itChannel) {
	if ch.vibDep == 0 {
		ch.vibMul = 1
		return
	}
	delta := itLFOWave(ch.vibWave, ch.vibPos) * float64(ch.vibDep) / 64.0
	ch.vibMul = math.Pow(2.0, delta/12.0)
	ch.vibPos = (ch.vibPos + int(ch.vibSpd)) & 63
}

func (p *Player) itFineVibrato(ch *itChannel) {
	if ch.uVibDep == 0 {
		ch.uVibMul = 1
		return
	}
	delta := itLFOWave(ch.vibWave, ch.uVibPos) * float64(ch.uVibDep) / 128.0
	ch.uVibMul = math.Pow(2.0, delta/12.0)
	ch.uVibPos = (ch.uVibPos + int(ch.uVibSpd)) & 63
}

func (p *Player) itTremolo(ch *itChannel) {
	if ch.tremDep == 0 {
		ch.tremMul = 1
		return
	}
	w := itLFOWave(ch.tremWave, ch.tremPos) * float64(ch.tremDep) / 64.0
	ch.tremMul = 1.0 + w*0.35
	if ch.tremMul < 0.05 {
		ch.tremMul = 0.05
	}
	if ch.tremMul > 2.0 {
		ch.tremMul = 2.0
	}
	ch.tremPos = (ch.tremPos + int(ch.tremSpd)) & 63
}

func (p *Player) itPanSlide(ch *itChannel, prm uint8) {
	if prm != 0 {
		ch.panSlideMem = prm
	} else {
		prm = ch.panSlideMem
	}
	x, y := int(prm>>4), int(prm&0x0F)
	if x > 0 && y == 0 {
		ch.panBase = clampInt(ch.panBase+x*4, 0, 255)
	} else if y > 0 && x == 0 {
		ch.panBase = clampInt(ch.panBase-y*4, 0, 255)
	}
}

func (p *Player) itSampleOffset(ch *itChannel, prm uint8) {
	if !ch.active || ch.sample == nil {
		return
	}
	ch.pos += float64(prm) * 256
}

// itMacroZApply runs one Zxx / fixed-macro byte without updating smooth-memory state.
func (p *Player) itMacroZApply(ch *itChannel, prm uint8) {
	if ch.disabled || p.mod == nil {
		return
	}
	cfg := &p.mod.MidiMacros
	idx := int(ch.zMacroIdx)
	if idx < 0 || idx >= itMidiSFxCount {
		idx = 0
	}

	if prm <= 0x7F {
		if !cfg.Valid {
			if ch.zMacroIdx != 0 {
				return
			}
			ch.fltCut = int(prm)
			if ch.fltCut > 127 {
				ch.fltCut = 127
			}
			ch.itFilterMarkDirty()
			ch.itFilterResetState()
			return
		}
		kind := classifySFxMacro(cfg.SFx[idx])
		itApplySFxMacro(p, ch, kind, cfg.SFx[idx], prm)
		return
	}

	slot := int(prm) - 0x80
	if slot < 0 || slot >= itMidiZxxCount {
		return
	}
	if cfg.Valid && !itFixedMacroIsEmpty(cfg, slot) {
		itApplyFixedZMacro(p, ch, cfg.Zxx[slot])
		return
	}
	if prm <= 0x8F {
		itDefaultFixedMacroReso4(ch, prm)
	}
}

func (p *Player) itMacroZ(ch *itChannel, prm uint8) {
	p.itMacroZApply(ch, prm)
	ch.lastMidiZParam = prm
}

func (p *Player) itMacroSmooth(ch *itChannel, prm uint8) {
	if ch.disabled || p.mod == nil {
		return
	}
	ch.zSmoothFrom = ch.lastMidiZParam
	ch.zSmoothTo = prm
	ch.zSmoothActive = true
	p.itMacroZApply(ch, ch.zSmoothFrom)
}

func (p *Player) itRetrig(ch *itChannel) {
	if ch.retrigMem == 0 {
		return
	}
	interval := int(ch.retrigMem & 0x0F)
	if interval <= 0 {
		return
	}
	ch.retrigCount++
	if ch.retrigCount >= interval {
		ch.retrigCount = 0
		ch.pos = 0
		hi := int(ch.retrigMem >> 4)
		if hi < 0 {
			hi = 0
		}
		if hi > 15 {
			hi = 15
		}
		ch.volume = clampInt(itQRetrigApplyVol(ch.volume, hi), 0, 64)
		ch.mixVol = ch.volume
	}
}

func (p *Player) itSpecial(ch *itChannel, prm uint8) {
	x := prm >> 4
	switch x {
	case 1:
		switch prm & 0x0F {
		case 0:
			ch.glissando = false
		case 1:
			ch.glissando = true
		}
	case 2:
		ch.s2Fine = float64(itS2FineTab[prm&0x0F]) / 96.0
	case 3:
		ch.vibWave = prm & 0x0F
	case 4:
		ch.tremWave = prm & 0x0F
	case 5:
		ch.panBWave = prm & 0x0F
	case 8:
		v := int(prm&0x0F) * 17
		if v > 255 {
			v = 255
		}
		if v < 0 {
			v = 0
		}
		ch.panBase = v
	case 9:
		// S9x sound control (OpenMPT); only global/local filter flags are used here.
		switch prm & 0x0F {
		case 0xC:
			p.localFilters = false
		case 0xD:
			p.localFilters = true
		}
	case 10:
		if !ch.active || ch.sample == nil {
			return
		}
		ch.pos += float64(prm&0x0F) * 65536
	case 15:
		// SFx — select parametered Z macro (0–15).
		ch.zMacroIdx = prm & 0x0F
	default:
		return
	}
}
