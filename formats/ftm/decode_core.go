package ftm

import "fmt"

func decodeHeaderBlock(data []byte, ver int, channels int) (*HeaderBlock, error) {
	r := newBlockReader(data)
	h := &HeaderBlock{Version: ver}

	switch ver {
	case 1:
		h.TrackCount = 1
		h.ChannelTypes = make([]byte, channels)
		h.EffectColumns = make([][]uint8, channels)
		for i := 0; i < channels; i++ {
			ct, err := r.readU8()
			if err != nil {
				return nil, err
			}
			if int(ct) > 32 { // CHANNELS-1 rough
				return nil, fmt.Errorf("HEADER: channel type %d", ct)
			}
			h.ChannelTypes[i] = ct
			ec, err := r.readU8()
			if err != nil {
				return nil, err
			}
			if int(ec) > maxEffectColumns-1 {
				return nil, fmt.Errorf("HEADER: effect columns %d", ec)
			}
			h.EffectColumns[i] = []uint8{ec}
		}
	case 2, 3, 4:
		tc, err := r.readU8()
		if err != nil {
			return nil, err
		}
		h.TrackCount = int(tc) + 1
		if h.TrackCount < 1 || h.TrackCount > maxTracks {
			return nil, fmt.Errorf("HEADER: track count %d", h.TrackCount)
		}
		if ver >= 3 {
			h.TrackTitles = make([]string, h.TrackCount)
			for i := 0; i < h.TrackCount; i++ {
				s, err := r.readCString(65536)
				if err != nil {
					return nil, err
				}
				h.TrackTitles[i] = s
			}
		}
		h.ChannelTypes = make([]byte, channels)
		h.EffectColumns = make([][]uint8, channels)
		for i := 0; i < channels; i++ {
			ct, err := r.readU8()
			if err != nil {
				return nil, err
			}
			h.ChannelTypes[i] = ct
			row := make([]uint8, h.TrackCount)
			for j := 0; j < h.TrackCount; j++ {
				ec, err := r.readU8()
				if err != nil {
					return nil, err
				}
				if int(ec) > maxEffectColumns-1 {
					return nil, fmt.Errorf("HEADER: effect columns %d at ch %d track %d", ec, i, j)
				}
				row[j] = ec
			}
			h.EffectColumns[i] = row
		}
		if ver >= 4 {
			for i := 0; i < h.TrackCount; i++ {
				f, err := r.readU8()
				if err != nil {
					return nil, err
				}
				s, err := r.readU8()
				if err != nil {
					return nil, err
				}
				if i == 0 {
					h.HighlightFirst = int(f)
					h.HighlightSecond = int(s)
				}
			}
		}
	default:
		return nil, fmt.Errorf("HEADER: unsupported block version %d", ver)
	}
	return h, nil
}

func decodeFramesBlock(data []byte, ver int, channels, tracks int, machine int) (*FramesBlock, error) {
	r := newBlockReader(data)
	out := &FramesBlock{}

	switch ver {
	case 1:
		fc, err := r.readI32()
		if err != nil {
			return nil, err
		}
		nCh, err := r.readI32()
		if err != nil {
			return nil, err
		}
		if int(nCh) != channels {
			return nil, fmt.Errorf("FRAMES v1: channel count %d != HEADER/PARAMS %d", nCh, channels)
		}
		if int(fc) < 1 || int(fc) > maxFrames {
			return nil, fmt.Errorf("FRAMES: frame count %d", fc)
		}
		tf := TrackFrames{
			FrameCount:    int(fc),
			Speed:         0,
			Tempo:         0,
			PatternLength: 0,
			Patterns:      make([][]byte, int(fc)),
		}
		for i := 0; i < int(fc); i++ {
			tf.Patterns[i] = make([]byte, channels)
			for j := 0; j < channels; j++ {
				p, err := r.readU8()
				if err != nil {
					return nil, err
				}
				if int(p) >= maxPattern {
					return nil, fmt.Errorf("FRAMES: pattern index %d", p)
				}
				tf.Patterns[i][j] = p
			}
		}
		out.Tracks = []TrackFrames{tf}
	default:
		if ver <= 1 {
			return nil, fmt.Errorf("FRAMES: unsupported version %d", ver)
		}
		out.Tracks = make([]TrackFrames, tracks)
		for t := 0; t < tracks; t++ {
			fc, err := r.readI32()
			if err != nil {
				return nil, err
			}
			sp, err := r.readI32()
			if err != nil {
				return nil, err
			}
			if int(fc) < 1 || int(fc) > maxFrames {
				return nil, fmt.Errorf("FRAMES: track %d frame count %d", t, fc)
			}
			tf := TrackFrames{FrameCount: int(fc)}
			if ver >= 3 {
				if int(sp) < 0 || int(sp) > maxTempo {
					return nil, fmt.Errorf("FRAMES: track %d speed %d", t, sp)
				}
				tf.Speed = int(sp)
				tmp, err := r.readI32()
				if err != nil {
					return nil, err
				}
				if int(tmp) < 0 || int(tmp) > maxTempo {
					return nil, fmt.Errorf("FRAMES: track %d tempo %d", t, tmp)
				}
				tf.Tempo = int(tmp)
			} else {
				// Version 2: single field doubles as speed or tempo (Dn ReadBlock_Frames).
				if int(sp) < 20 {
					tf.Speed = int(sp)
					if machine == 1 {
						tf.Tempo = defaultTempoPAL
					} else {
						tf.Tempo = defaultTempoNTSC
					}
				} else {
					tf.Tempo = int(sp)
					tf.Speed = defaultSpeed
				}
			}
			pl, err := r.readI32()
			if err != nil {
				return nil, err
			}
			if int(pl) < 1 || int(pl) > maxPatternRows {
				return nil, fmt.Errorf("FRAMES: pattern length %d", pl)
			}
			tf.PatternLength = int(pl)
			tf.Patterns = make([][]byte, int(fc))
			for i := 0; i < int(fc); i++ {
				tf.Patterns[i] = make([]byte, channels)
				for j := 0; j < channels; j++ {
					p, err := r.readU8()
					if err != nil {
						return nil, err
					}
					if int(p) >= maxPattern {
						return nil, fmt.Errorf("FRAMES: pattern index %d", p)
					}
					tf.Patterns[i][j] = p
				}
			}
			out.Tracks[t] = tf
		}
	}
	return out, nil
}

func decodeCommentsBlock(data []byte, ver int) (*CommentsBlock, error) {
	r := newBlockReader(data)
	_ = ver
	di, err := r.readI32()
	if err != nil {
		return nil, err
	}
	txt, err := r.readCString(1 << 20)
	if err != nil {
		return nil, err
	}
	return &CommentsBlock{Display: di == 1, Text: txt}, r.assertFullyConsumed("COMMENTS")
}
