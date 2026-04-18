package it

import (
	"encoding/binary"
	"fmt"
)

type itCell struct {
	HasNote   bool
	Note      uint8
	HasInst   bool
	Inst      uint8
	HasVolPan bool
	VolPan    uint8
	HasCmd    bool
	Cmd       uint8
	Param     uint8
}

type itPattern struct {
	Rows int
	Data [][]itCell
}

func unpackPattern(data []byte, off int) (itPattern, error) {
	var p itPattern
	if off == 0 {
		p.Rows = 64
		p.Data = make([][]itCell, 64)
		for r := range p.Data {
			p.Data[r] = make([]itCell, 64)
		}
		return p, nil
	}
	if off+8 > len(data) {
		return p, fmt.Errorf("pattern header truncated")
	}
	packedLen := int(binary.LittleEndian.Uint16(data[off:]))
	rows := int(binary.LittleEndian.Uint16(data[off+2:]))
	if rows < 1 {
		rows = 64
	}
	if rows > 200 {
		return p, fmt.Errorf("invalid IT pattern rows %d", rows)
	}
	end := off + 8 + packedLen
	if end > len(data) {
		return p, fmt.Errorf("pattern data truncated")
	}
	p.Rows = rows
	p.Data = make([][]itCell, rows)
	for r := range p.Data {
		p.Data[r] = make([]itCell, 64)
	}

	var lastMask [64]byte
	var lastN, lastI, lastVP, lastC, lastP [64]uint8

	pos := off + 8
	for row := 0; row < rows; {
		if pos >= end {
			return p, fmt.Errorf("pattern row %d truncated", row)
		}
		chv := data[pos]
		pos++
		if chv == 0 {
			row++
			continue
		}
		ch := int((chv - 1) & 63)
		if ch < 0 || ch > 63 {
			return p, fmt.Errorf("invalid IT channel marker")
		}
		mask := lastMask[ch]
		if chv&0x80 != 0 {
			if pos >= end {
				return p, fmt.Errorf("pattern mask truncated")
			}
			mask = data[pos]
			pos++
			lastMask[ch] = mask
		}

		if mask&1 != 0 {
			if pos >= end {
				return p, fmt.Errorf("pattern note truncated")
			}
			lastN[ch] = data[pos]
			pos++
		}
		if mask&2 != 0 {
			if pos >= end {
				return p, fmt.Errorf("pattern inst truncated")
			}
			lastI[ch] = data[pos]
			pos++
		}
		if mask&4 != 0 {
			if pos >= end {
				return p, fmt.Errorf("pattern vol truncated")
			}
			lastVP[ch] = data[pos]
			pos++
		}
		if mask&8 != 0 {
			if pos+2 > end {
				return p, fmt.Errorf("pattern cmd truncated")
			}
			lastC[ch] = data[pos]
			lastP[ch] = data[pos+1]
			pos += 2
		}

		var c itCell
		if mask&(1|16) != 0 {
			c.HasNote = true
			c.Note = lastN[ch]
		}
		if mask&(2|32) != 0 {
			c.HasInst = true
			c.Inst = lastI[ch]
		}
		if mask&(4|64) != 0 {
			c.HasVolPan = true
			c.VolPan = lastVP[ch]
		}
		if mask&(8|128) != 0 {
			c.HasCmd = true
			c.Cmd = lastC[ch]
			c.Param = lastP[ch]
		}
		p.Data[row][ch] = c
	}
	return p, nil
}
