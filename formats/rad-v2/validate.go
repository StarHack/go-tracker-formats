package radv2

import "errors"

// Validate checks a RAD v2.1 tune file for validity.
// Returns nil if valid, or an error otherwise.
func Validate(data []byte) error {
	if len(data) < 17 {
		return errors.New("not a RAD tune file")
	}
	hdr := "RAD by REALiTY!!"
	for i := 0; i < 16; i++ {
		if data[i] != hdr[i] {
			return errors.New("not a RAD tune file")
		}
	}
	if data[0x10] != 0x21 {
		return errors.New("not a version 2.1 RAD tune")
	}
	s := data[0x11:]
	if len(s) == 0 {
		return errors.New("tune file has been truncated and is incomplete")
	}
	flags := s[0]
	s = s[1:]
	if flags&0x80 != 0 {
		return errors.New("tune file has invalid flags")
	}
	if flags&0x20 != 0 {
		if len(s) < 2 {
			return errors.New("tune file has been truncated and is incomplete")
		}
		bpm := int(s[0]) | (int(s[1]) << 8)
		s = s[2:]
		if bpm < 46 || bpm > 300 {
			return errors.New("tune's BPM value is out of range")
		}
	}
	for {
		if len(s) == 0 {
			return errors.New("tune file has been truncated and is incomplete")
		}
		c := s[0]
		s = s[1:]
		if c == 0 {
			break
		}
	}
	lastInst := uint8(0)
	for {
		if len(s) == 0 {
			return errors.New("tune file has been truncated and is incomplete")
		}
		inst := s[0]
		s = s[1:]
		if inst == 0 {
			break
		}
		if inst > 127 || inst <= lastInst {
			return errors.New("tune file contains a bad instrument definition")
		}
		lastInst = inst
		if len(s) == 0 {
			return errors.New("tune file has been truncated and is incomplete")
		}
		nameLen := int(s[0])
		s = s[1:]
		if len(s) < nameLen {
			return errors.New("tune file has been truncated and is incomplete")
		}
		s = s[nameLen:]
		if len(s) == 0 {
			return errors.New("tune file has been truncated and is incomplete")
		}
		alg := s[0]
		if alg&7 == 7 {
			if len(s) < 6 {
				return errors.New("tune file has been truncated and is incomplete")
			}
			if s[2]>>4 != 0 {
				return errors.New("tune file contains an unknown MIDI instrument version")
			}
			s = s[6:]
		} else {
			s = s[1:]
			if len(s) < 23 {
				return errors.New("tune file has been truncated and is incomplete")
			}
			s = s[23:]
		}
		if alg&0x80 != 0 {
			var err error
			s, err = checkPattern(s, false)
			if err != nil {
				return err
			}
		}
	}
	if len(s) == 0 {
		return errors.New("tune file has been truncated and is incomplete")
	}
	orderSize := int(s[0])
	s = s[1:]
	if orderSize > 128 {
		return errors.New("order list in tune file is an invalid size")
	}
	if len(s) < orderSize {
		return errors.New("tune file has been truncated and is incomplete")
	}
	orderList := s[:orderSize]
	s = s[orderSize:]
	for i := 0; i < orderSize; i++ {
		order := orderList[i]
		if order&0x80 != 0 {
			order &= 0x7F
			if int(order) >= orderSize {
				return errors.New("order list jump marker is invalid")
			}
		} else if order >= 100 {
			return errors.New("order list entry is invalid")
		}
	}
	for {
		if len(s) == 0 {
			return errors.New("tune file has been truncated and is incomplete")
		}
		pattNum := s[0]
		s = s[1:]
		if pattNum == 0xFF {
			break
		}
		if pattNum >= 100 {
			return errors.New("tune file contains a bad pattern index")
		}
		var err error
		s, err = checkPattern(s, false)
		if err != nil {
			return err
		}
	}
	for {
		if len(s) == 0 {
			return errors.New("tune file has been truncated and is incomplete")
		}
		riffNum := s[0]
		s = s[1:]
		if riffNum == 0xFF {
			break
		}
		riffPatt := riffNum >> 4
		riffChan := riffNum & 15
		if riffPatt > 9 || riffChan == 0 || riffChan > 9 {
			return errors.New("tune file contains a bad riff index")
		}
		var err error
		s, err = checkPattern(s, true)
		if err != nil {
			return err
		}
	}
	if len(s) != 0 {
		return errors.New("tune file contains extra bytes")
	}
	return nil
}

func checkPattern(s []byte, riff bool) ([]byte, error) {
	if len(s) < 2 {
		return s, errors.New("tune file has been truncated and is incomplete")
	}
	pattSize := int(s[0]) | (int(s[1]) << 8)
	s = s[2:]
	if len(s) < pattSize {
		return s, errors.New("tune file has been truncated and is incomplete")
	}
	pe := s[:pattSize]
	s = s[pattSize:]
	for {
		if len(pe) == 0 {
			return s, errors.New("tune file contains a truncated pattern")
		}
		lineDef := pe[0]
		pe = pe[1:]
		if lineDef&0x7F >= 64 {
			return s, errors.New("tune file contains a pattern with a bad line definition")
		}
		for {
			if len(pe) == 0 {
				return s, errors.New("tune file contains a truncated pattern")
			}
			chanDef := pe[0]
			pe = pe[1:]
			if !riff && chanDef&0x0F >= 9 {
				return s, errors.New("tune file contains a pattern with a bad channel definition")
			}
			if chanDef&0x40 != 0 {
				if len(pe) == 0 {
					return s, errors.New("tune file contains a truncated pattern")
				}
				note := pe[0]
				pe = pe[1:]
				noteNum := note & 15
				if noteNum == 0 || noteNum == 13 || noteNum == 14 {
					return s, errors.New("pattern contains a bad note number")
				}
			}
			if chanDef&0x20 != 0 {
				if len(pe) == 0 {
					return s, errors.New("tune file contains a truncated pattern")
				}
				inst := pe[0]
				pe = pe[1:]
				if inst == 0 || inst >= 128 {
					return s, errors.New("pattern contains a bad instrument number")
				}
			}
			if chanDef&0x10 != 0 {
				if len(pe) < 2 {
					return s, errors.New("tune file contains a truncated pattern")
				}
				effect := pe[0]
				param := pe[1]
				pe = pe[2:]
				if effect > 31 || param > 99 {
					return s, errors.New("pattern contains a bad effect and/or parameter")
				}
			}
			if chanDef&0x80 != 0 {
				break
			}
		}
		if lineDef&0x80 != 0 {
			break
		}
	}
	if len(pe) != 0 {
		return s, errors.New("tune file contains a pattern with extraneous data")
	}
	return s, nil
}
