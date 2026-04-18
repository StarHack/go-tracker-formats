package ftm

import (
	"fmt"
)

func decodeSeqInstrumentData(r *blockReader) (*SeqInstrumentData, error) {
	if _, err := r.readI32(); err != nil { // count (always SEQ_COUNT in Dn)
		return nil, err
	}
	var out SeqInstrumentData
	for i := 0; i < seqCount; i++ {
		en, err := r.readU8()
		if err != nil {
			return nil, err
		}
		out.Enabled[i] = en != 0
		ix, err := r.readU8()
		if err != nil {
			return nil, err
		}
		if int(ix) >= maxSequences {
			return nil, fmt.Errorf("instrument: sequence index %d", ix)
		}
		out.Index[i] = ix
	}
	return &out, nil
}

func decodeFDSInlineSequence(r *blockReader) (FDSInlineSequence, error) {
	var fs FDSInlineSequence
	sc, err := r.readU8()
	if err != nil {
		return fs, err
	}
	lp, err := r.readI32()
	if err != nil {
		return fs, err
	}
	if int(lp) < -1 || int(lp) > int(sc)-1 {
		return fs, fmt.Errorf("FDS sequence: loop %d", lp)
	}
	rp, err := r.readI32()
	if err != nil {
		return fs, err
	}
	if int(rp) < -1 || int(rp) > int(sc)-1 {
		return fs, fmt.Errorf("FDS sequence: release %d", rp)
	}
	set, err := r.readI32()
	if err != nil {
		return fs, err
	}
	fs.Count = int(sc)
	fs.LoopPoint = lp
	fs.ReleasePoint = rp
	fs.Setting = set
	fs.Values = make([]int8, 0, int(sc))
	for j := 0; j < int(sc); j++ {
		v, err := r.readI8()
		if err != nil {
			return fs, err
		}
		fs.Values = append(fs.Values, v)
	}
	return fs, nil
}

func decodeInstrument2A03(r *blockReader, instVer int) (*Instrument2A03Data, error) {
	d := &Instrument2A03Data{BlockVersion: instVer}
	octaves := octaveRange
	if instVer == 1 {
		octaves = 6
	}
	readAssign := func(oct, note int) (DPCMAssignment, error) {
		var a DPCMAssignment
		a.Octave, a.Note = oct, note
		smp, err := r.readU8()
		if err != nil {
			return a, err
		}
		if int(smp) > maxDSamples {
			return a, fmt.Errorf("2A03 DPCM sample index %d", smp)
		}
		a.Sample = smp
		pitch, err := r.readU8()
		if err != nil {
			return a, err
		}
		if int(pitch&0x7f) > 0x0f {
			return a, fmt.Errorf("2A03 DPCM pitch")
		}
		a.Pitch = pitch
		if instVer > 5 {
			dv, err := r.readI8()
			if err != nil {
				return a, err
			}
			a.Delta = dv
		} else {
			a.Delta = -1
		}
		return a, nil
	}
	if instVer >= 7 {
		n, err := r.readI32()
		if err != nil {
			return nil, err
		}
		if int(n) < 0 || int(n) > noteCount {
			return nil, fmt.Errorf("2A03 DPCM assignment count %d", n)
		}
		for i := 0; i < int(n); i++ {
			midiNote, err := r.readU8()
			if err != nil {
				return nil, err
			}
			if int(midiNote) < 1 || int(midiNote) > noteCount {
				return nil, fmt.Errorf("2A03 DPCM note index %d", midiNote)
			}
			oct := (int(midiNote) - 1) / noteRange
			note := (int(midiNote) - 1) % noteRange
			a, err := readAssign(oct, note)
			if err != nil {
				return nil, err
			}
			d.Assignments = append(d.Assignments, a)
		}
	} else {
		for o := 0; o < octaves; o++ {
			for n := 0; n < noteRange; n++ {
				a, err := readAssign(o, n)
				if err != nil {
					return nil, err
				}
				d.GridSample[o][n] = a.Sample
				d.GridPitch[o][n] = a.Pitch
				d.GridDelta[o][n] = a.Delta
			}
		}
	}
	return d, nil
}

func decodeInstrumentFDS(r *blockReader, instVer int) (*InstrumentFDSData, error) {
	d := &InstrumentFDSData{BlockVersion: instVer}
	for i := range d.Wave {
		b, err := r.readU8()
		if err != nil {
			return nil, err
		}
		d.Wave[i] = b
	}
	for i := range d.Modulation {
		b, err := r.readU8()
		if err != nil {
			return nil, err
		}
		d.Modulation[i] = b
	}
	var err error
	d.ModSpeed, err = r.readI32()
	if err != nil {
		return nil, err
	}
	d.ModDepth, err = r.readI32()
	if err != nil {
		return nil, err
	}
	d.ModDelay, err = r.readI32()
	if err != nil {
		return nil, err
	}
	for i := 0; i < fdsSequenceCount; i++ {
		if instVer > 2 || i < 2 { // SEQ_PITCH index 2 omitted in very old FDS block versions
			s, err := decodeFDSInlineSequence(r)
			if err != nil {
				return nil, err
			}
			d.Sequences[i] = s
		}
	}
	return d, nil
}

func decodeInstrumentN163(r *blockReader, instVer int) (*InstrumentN163Data, error) {
	d := &InstrumentN163Data{BlockVersion: instVer}
	var err error
	ws, err := r.readI32()
	if err != nil {
		return nil, err
	}
	d.WaveSize = int(ws)
	if d.WaveSize < 4 || d.WaveSize > n163MaxWaveSize {
		return nil, fmt.Errorf("N163 wave size %d", d.WaveSize)
	}
	wp, err := r.readI32()
	if err != nil {
		return nil, err
	}
	d.WavePos = int(wp)
	if d.WavePos < 0 || d.WavePos > n163MaxWaveSize-1 {
		return nil, fmt.Errorf("N163 wave pos %d", d.WavePos)
	}
	if instVer >= 8 {
		aw, err := r.readI32()
		if err != nil {
			return nil, err
		}
		d.AutoWavePos = aw != 0
	}
	wc, err := r.readI32()
	if err != nil {
		return nil, err
	}
	d.WaveCount = int(wc)
	if d.WaveCount < 1 || d.WaveCount > n163MaxWaveCount {
		return nil, fmt.Errorf("N163 wave count %d", d.WaveCount)
	}
	d.Samples = make([][]byte, d.WaveCount)
	for wi := 0; wi < d.WaveCount; wi++ {
		row := make([]byte, d.WaveSize)
		for j := 0; j < d.WaveSize; j++ {
			b, err := r.readU8()
			if err != nil {
				return nil, err
			}
			if int(b) > 15 {
				return nil, fmt.Errorf("N163 sample value %d", b)
			}
			row[j] = b
		}
		d.Samples[wi] = row
	}
	return d, nil
}

func decodeInstrumentVRC7(r *blockReader) (*InstrumentVRC7Data, error) {
	var d InstrumentVRC7Data
	var err error
	d.Patch, err = r.readI32()
	if err != nil {
		return nil, err
	}
	if d.Patch < 0 || d.Patch > 0x0f {
		return nil, fmt.Errorf("VRC7 patch %d", d.Patch)
	}
	for i := range d.Regs {
		d.Regs[i], err = r.readU8()
		if err != nil {
			return nil, err
		}
	}
	return &d, nil
}

func decodeInstrumentsBlock(data []byte, ver int) (*InstrumentsBlock, error) {
	r := newBlockReader(data)
	n, err := r.readI32()
	if err != nil {
		return nil, err
	}
	if int(n) < 0 || int(n) > maxInstruments {
		return nil, fmt.Errorf("INSTRUMENTS: count %d", n)
	}
	out := &InstrumentsBlock{Version: ver, Instruments: make([]Instrument, 0, int(n))}
	for i := 0; i < int(n); i++ {
		idx, err := r.readI32()
		if err != nil {
			return nil, err
		}
		if int(idx) < 0 || int(idx) >= maxInstruments {
			return nil, fmt.Errorf("INSTRUMENTS: index %d", idx)
		}
		typ, err := r.readU8()
		if err != nil {
			return nil, err
		}
		inst := Instrument{Index: int(idx), Type: int(typ)}
		switch int(typ) {
		case InstNone:
			return nil, fmt.Errorf("INSTRUMENTS: instrument type NONE at index %d", idx)
		case Inst2A03:
			inst.Seq, err = decodeSeqInstrumentData(r)
			if err != nil {
				return nil, err
			}
			inst.TwoA03, err = decodeInstrument2A03(r, ver)
			if err != nil {
				return nil, err
			}
		case InstVRC6, InstS5B:
			inst.Seq, err = decodeSeqInstrumentData(r)
			if err != nil {
				return nil, err
			}
		case InstVRC7:
			inst.VRC7, err = decodeInstrumentVRC7(r)
			if err != nil {
				return nil, err
			}
		case InstFDS:
			inst.FDS, err = decodeInstrumentFDS(r, ver)
			if err != nil {
				return nil, err
			}
		case InstN163:
			inst.Seq, err = decodeSeqInstrumentData(r)
			if err != nil {
				return nil, err
			}
			inst.N163, err = decodeInstrumentN163(r, ver)
			if err != nil {
				return nil, err
			}
		default:
			return nil, fmt.Errorf("INSTRUMENTS: unknown type %d at index %d", typ, idx)
		}
		nsz, err := r.readI32()
		if err != nil {
			return nil, err
		}
		if int(nsz) < 0 || int(nsz) > instNameMax {
			return nil, fmt.Errorf("INSTRUMENTS: name length %d", nsz)
		}
		nb, err := r.readBytes(int(nsz))
		if err != nil {
			return nil, err
		}
		inst.Name = string(nb)
		out.Instruments = append(out.Instruments, inst)
	}
	return out, nil
}
