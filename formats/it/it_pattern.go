package it

// noteDelayTicks returns SDx note-delay ticks (OpenMPT IT), or -1 if absent.
// SD0 is treated as 1 tick (same as SC0 for cut).
func noteDelayTicks(cell itCell) int {
	if !cell.HasCmd || cell.Cmd != 19 || cell.Param>>4 != 0x0D {
		return -1
	}
	v := int(cell.Param & 0x0F)
	if v == 0 {
		return 1
	}
	return v
}

// scanPatternRowGlobals reads IT row-global S commands (S6, SB, SE).
// Call after applyGlobalRowEffects so speed is final for rowTickLimit.
// skipSERescan is true on SEx row repetitions: S6/SB are re-read, but SE counter is not reset.
func (p *Player) scanPatternRowGlobals(pat *itPattern, row int, skipSERescan bool) {
	p.patternDelayExtra = 0
	p.jumpLoopAfterRow = false
	if !skipSERescan {
		p.seRepeatRemain = -1
	}

	for ci := 0; ci < 64; ci++ {
		c := pat.Data[row][ci]
		if !c.HasCmd || c.Cmd != 19 {
			continue
		}
		sub := c.Param >> 4
		lo := int(c.Param & 0x0F)
		switch sub {
		case 0x6:
			p.patternDelayExtra += lo
		case 0xB:
			if lo == 0 {
				p.loopStartOrder = p.order
				p.loopStartRow = row
				p.loopActive = true
				p.sbArmOrder, p.sbArmRow = -1, -1
			} else if p.loopActive {
				p.jumpLoopAfterRow = true
				if p.order != p.sbArmOrder || row != p.sbArmRow {
					p.sbJumpBudget = lo
					p.sbArmOrder = p.order
					p.sbArmRow = row
				}
			}
		case 0xE:
			if !skipSERescan && p.seRepeatRemain < 0 {
				p.seRepeatRemain = lo
			}
		}
	}
	if p.patternDelayExtra > 255 {
		p.patternDelayExtra = 255
	}
	p.rowTickLimit = p.speed + p.patternDelayExtra
	if p.rowTickLimit < 1 {
		p.rowTickLimit = 1
	}
	if p.rowTickLimit > 255 {
		p.rowTickLimit = 255
	}
}
