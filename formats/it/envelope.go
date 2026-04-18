package it

// envelopeAmp returns a linear volume scale (typically 0..1) from the IT volume
// envelope at module tick tick. sustainReleased is true after IT key-off (note
// 255): the sustain segment is no longer held and the envelope may run past
// the sustain end toward the terminal nodes.
func envelopeAmp(e *itEnvelopeData, tick int, sustainReleased bool) float64 {
	if e == nil || !e.Valid || e.Num < 2 {
		return 1
	}
	n := int(e.Num)
	if n > 25 {
		n = 25
	}
	t := tick
	if t < 0 {
		t = 0
	}
	if e.Flags&4 != 0 && !sustainReleased {
		susEnd := int(e.NodeTick[e.SusEnd])
		if susEnd >= 0 && t > susEnd {
			t = susEnd
		}
	}
	lastTick := int(e.NodeTick[n-1])
	if e.Flags&2 != 0 && lastTick > 0 && t > lastTick {
		loopStart := int(e.NodeTick[e.LoopStart])
		loopEnd := int(e.NodeTick[e.LoopEnd])
		if loopEnd > loopStart && loopEnd <= lastTick {
			span := loopEnd - loopStart
			if span > 0 {
				t = loopStart + ((t - loopStart) % span)
			}
		}
	}
	if t <= int(e.NodeTick[0]) {
		return float64(e.NodeY[0]) / 64.0
	}
	for i := 0; i < n-1; i++ {
		t0 := int(e.NodeTick[i])
		t1 := int(e.NodeTick[i+1])
		if t1 < t0 {
			continue
		}
		if t >= t0 && t <= t1 {
			if t1 == t0 {
				return float64(e.NodeY[i]) / 64.0
			}
			frac := float64(t-t0) / float64(t1-t0)
			y0 := float64(e.NodeY[i])
			y1 := float64(e.NodeY[i+1])
			return (y0*(1-frac) + y1*frac) / 64.0
		}
	}
	return float64(e.NodeY[n-1]) / 64.0
}
