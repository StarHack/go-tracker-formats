package ftm

import "fmt"

func decodeParamsBlock(data []byte, ver int) (*ParamsBlock, error) {
	r := newBlockReader(data)
	p := &ParamsBlock{Version: ver}

	if ver == 1 {
		v, err := r.readI32()
		if err != nil {
			return nil, err
		}
		p.SongSpeedLegacyV1 = int(v)
	} else {
		c, err := r.readU8()
		if err != nil {
			return nil, err
		}
		p.ExpansionChip = int(int8(c))
	}

	ch, err := r.readI32()
	if err != nil {
		return nil, err
	}
	p.Channels = int(ch)
	if p.Channels < 1 || p.Channels > maxChannels {
		return nil, fmt.Errorf("PARAMS: channel count %d out of range", p.Channels)
	}

	mach, err := r.readI32()
	if err != nil {
		return nil, err
	}
	p.Machine = int(mach)
	if p.Machine != 0 && p.Machine != 1 {
		return nil, fmt.Errorf("PARAMS: unknown machine type %d", p.Machine)
	}

	if ver >= 7 {
		rt, err := r.readI32()
		if err != nil {
			return nil, err
		}
		p.PlaybackRateType = int(rt)
		if p.PlaybackRateType < 0 || p.PlaybackRateType > 2 {
			return nil, fmt.Errorf("PARAMS: playback rate type %d", p.PlaybackRateType)
		}
		pr, err := r.readI32()
		if err != nil {
			return nil, err
		}
		p.PlaybackRateUS = int(pr)
		if p.PlaybackRateUS < 0 || p.PlaybackRateUS > 0xffff {
			return nil, fmt.Errorf("PARAMS: playback rate out of range")
		}
		switch p.PlaybackRateType {
		case 1:
			if p.PlaybackRateUS > 0 {
				p.EngineSpeed = int(1000000.0/float64(p.PlaybackRateUS) + 0.5)
			}
		default:
			p.EngineSpeed = 0
		}
	} else {
		es, err := r.readI32()
		if err != nil {
			return nil, err
		}
		p.EngineSpeed = int(es)
	}

	if ver > 2 {
		vs, err := r.readI32()
		if err != nil {
			return nil, err
		}
		p.VibratoStyle = int(vs)
	} else {
		p.VibratoStyle = 0
	}

	if ver >= 9 {
		if _, err := r.readI32(); err != nil { // sweep reset flag (ignored in Go)
			return nil, err
		}
	}

	if ver > 3 && ver <= 6 {
		h1, err := r.readI32()
		if err != nil {
			return nil, err
		}
		h2, err := r.readI32()
		if err != nil {
			return nil, err
		}
		p.HighlightFirst = int(h1)
		p.HighlightSecond = int(h2)
	}

	if p.Channels == 5 {
		p.ExpansionChip = 0
	}

	if ver >= 5 && (p.ExpansionChip&16) != 0 { // SNDCHIP_N163
		n, err := r.readI32()
		if err != nil {
			return nil, err
		}
		p.N163ChannelCount = int(n)
		if p.N163ChannelCount < 1 || p.N163ChannelCount > 8 {
			return nil, fmt.Errorf("PARAMS: N163 channel count %d", p.N163ChannelCount)
		}
	}

	if ver >= 6 {
		sp, err := r.readI32()
		if err != nil {
			return nil, err
		}
		p.SpeedSplitPoint = int(sp)
	} else {
		p.SpeedSplitPoint = 32 // OLD_SPEED_SPLIT_POINT
	}

	if p.ExpansionChip < 0 || p.ExpansionChip > 0x3f {
		return nil, fmt.Errorf("PARAMS: expansion chip 0x%X out of range", p.ExpansionChip&0xff)
	}

	if ver == 8 {
		s, err := r.readI8()
		if err != nil {
			return nil, err
		}
		p.DetuneSemitone = s
		c, err := r.readI8()
		if err != nil {
			return nil, err
		}
		p.DetuneCent = c
	}

	return p, nil
}
