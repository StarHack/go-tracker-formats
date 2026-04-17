package radv1

import "errors"

// Validate checks a RAD v1 tune file for validity.
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
	if data[0x10] != 0x10 {
		return errors.New("not a version 1.0 RAD tune")
	}
	s := data[0x11:]
	if len(s) == 0 {
		return errors.New("tune file has been truncated and is incomplete")
	}
	flags := s[0]
	s = s[1:]
	if flags&0x20 != 0 {
		return errors.New("tune file has invalid flags")
	}
	if flags&0x80 != 0 {
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
		if len(s) < 11 {
			return errors.New("tune file has been truncated and is incomplete")
		}
		s = s[11:]
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
		} else if order >= 32 {
			return errors.New("order list entry is invalid")
		}
	}
	for i := 0; i < 32; i++ {
		if len(s) < 2 {
			return errors.New("tune file has been truncated and is incomplete")
		}
		pos := int(s[0]) | (int(s[1]) << 8)
		s = s[2:]
		if pos != 0 {
			if pos >= len(data) {
				return errors.New("tune file has been truncated and is incomplete")
			}
			if err := checkPatternOld(data[pos:]); err != nil {
				return err
			}
		}
	}
	return nil
}

func checkPatternOld(s []byte) error {
	for {
		if len(s) == 0 {
			return errors.New("tune file contains a truncated pattern")
		}
		lineDef := s[0]
		s = s[1:]
		if lineDef&0x7F >= 64 {
			return errors.New("tune file contains a pattern with a bad line definition")
		}
		for {
			if len(s) == 0 {
				return errors.New("tune file contains a truncated pattern")
			}
			chanDef := s[0]
			s = s[1:]
			if chanDef&0x0F >= 9 {
				return errors.New("tune file contains a pattern with a bad channel definition")
			}
			if len(s) < 2 {
				return errors.New("tune file contains a truncated pattern")
			}
			instEffect := s[1]
			s = s[2:]
			if instEffect&0x0f != 0 {
				if len(s) == 0 {
					return errors.New("tune file contains a truncated pattern")
				}
				s = s[1:]
			}
			if chanDef&0x80 != 0 {
				break
			}
		}
		if lineDef&0x80 != 0 {
			break
		}
	}
	return nil
}
