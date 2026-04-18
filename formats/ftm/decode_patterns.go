package ftm

import (
	"fmt"
)

const effectSpeed = 1 // effect_t::EF_SPEED (after EF_NONE)

func effectColumnCount(m *Module, channel, track int) int {
	if m == nil || m.Header == nil {
		return 0
	}
	if channel < 0 || channel >= len(m.Header.EffectColumns) {
		return 0
	}
	row := m.Header.EffectColumns[channel]
	if track < 0 || track >= len(row) {
		return 0
	}
	return int(row[track])
}

func patternEffectWidth(fileVer uint32, patVer int, m *Module, track, channel int) int {
	if fileVer == 0x0200 {
		return 1
	}
	if patVer >= 6 {
		return maxEffectColumns
	}
	return effectColumnCount(m, channel, track) + 1
}

func decodePatternsBlock(data []byte, patVer int, fileVer uint32, m *Module) (*PatternsBlock, error) {
	if m == nil || m.Params == nil {
		return nil, fmt.Errorf("PATTERNS: require PARAMS before PATTERNS")
	}
	channels := m.Params.Channels
	tracks := 1
	if m.Header != nil && m.Header.TrackCount > 0 {
		tracks = m.Header.TrackCount
	}

	r := newBlockReader(data)
	out := &PatternsBlock{Version: patVer, Rows: nil}

	if patVer == 1 {
		pl, err := r.readI32()
		if err != nil {
			return nil, err
		}
		if int(pl) < 0 || int(pl) > maxPatternRows {
			return nil, fmt.Errorf("PATTERNS v1: pattern length %d", pl)
		}
		out.LegacyPatternLength = int(pl)
	}

	for !r.done() {
		track := 0
		if patVer > 1 {
			tv, err := r.readI32()
			if err != nil {
				return nil, err
			}
			track = int(tv)
			if track < 0 || track >= tracks {
				return nil, fmt.Errorf("PATTERNS: track index %d", track)
			}
		}
		chv, err := r.readI32()
		if err != nil {
			return nil, err
		}
		channel := int(chv)
		if channel < 0 || channel >= channels {
			return nil, fmt.Errorf("PATTERNS: channel index %d", channel)
		}
		patIdx, err := r.readI32()
		if err != nil {
			return nil, err
		}
		if int(patIdx) < 0 || int(patIdx) >= maxPattern {
			return nil, fmt.Errorf("PATTERNS: pattern index %d", patIdx)
		}
		items, err := r.readI32()
		if err != nil {
			return nil, err
		}
		if int(items) < 0 || int(items) > maxPatternRows {
			return nil, fmt.Errorf("PATTERNS: item count %d", items)
		}

		fx := patternEffectWidth(fileVer, patVer, m, track, channel)

		for i := 0; i < int(items); i++ {
			var row int
			if fileVer == 0x0200 || patVer >= 6 {
				rv, err := r.readU8()
				if err != nil {
					return nil, err
				}
				row = int(rv)
			} else {
				riv, err := r.readI32()
				if err != nil {
					return nil, err
				}
				if riv < 0 || riv > 0xff {
					return nil, fmt.Errorf("PATTERNS: row index %d", riv)
				}
				row = int(riv)
			}
			if row < 0 || row >= maxPatternRows {
				return nil, fmt.Errorf("PATTERNS: row %d", row)
			}

			note, err := r.readU8()
			if err != nil {
				return nil, err
			}
			if note > noteEcho {
				return nil, fmt.Errorf("PATTERNS: note value %d", note)
			}
			oct, err := r.readU8()
			if err != nil {
				return nil, err
			}
			if oct > byte(octaveRange-1) {
				return nil, fmt.Errorf("PATTERNS: octave %d", oct)
			}
			inst, err := r.readU8()
			if err != nil {
				return nil, err
			}
			if inst != byte(holdInstrument) && int(inst) > maxInstruments {
				return nil, fmt.Errorf("PATTERNS: instrument %d", inst)
			}
			vol, err := r.readU8()
			if err != nil {
				return nil, err
			}
			if vol > maxPatternVolume {
				return nil, fmt.Errorf("PATTERNS: volume %d", vol)
			}

			effN := make([]byte, 0, fx)
			effP := make([]byte, 0, fx)
			for n := 0; n < fx; n++ {
				en, err := r.readU8()
				if err != nil {
					return nil, err
				}
				var ep byte
				if en != 0 {
					if en > maxEffectID {
						return nil, fmt.Errorf("PATTERNS: effect type %d", en)
					}
					ep, err = r.readU8()
					if err != nil {
						return nil, err
					}
					if patVer < 3 {
						if en == 9 { // EF_PORTAOFF legacy
							en = 8 // EF_PORTAMENTO
							ep = 0
						} else if en == 8 && ep < 0xff {
							ep++
						}
					}
				} else if patVer < 6 {
					if _, err := r.readU8(); err != nil {
						return nil, err
					}
				}
				effN = append(effN, en)
				effP = append(effP, ep)
			}

			if fileVer == 0x0200 {
				if note == noteNone {
					inst = byte(maxInstruments)
				}
				if len(effN) > 0 && effN[0] == effectSpeed && effP[0] < 20 {
					effP[0]++
				}
				if vol == 0 {
					vol = maxPatternVolume
				} else {
					vol--
					vol &= 0x0f
				}
			}

			out.Rows = append(out.Rows, PatternCell{
				Track:    track,
				Channel:  channel,
				Pattern:  int(patIdx),
				Row:      row,
				Note:     note,
				Octave:   oct,
				Inst:     inst,
				Vol:      vol,
				EffNum:   effN,
				EffParam: effP,
			})
		}
	}
	return out, nil
}
