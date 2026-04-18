package ftm

import "fmt"

func decodeSequences2A03Block(data []byte, ver int) (*Sequences2A03Block, error) {
	r := newBlockReader(data)
	cnt, err := r.readI32()
	if err != nil {
		return nil, err
	}
	if int(cnt) < 0 || int(cnt) > maxSequences*seqCount {
		return nil, fmt.Errorf("SEQUENCES: count %d", cnt)
	}

	out := &Sequences2A03Block{Version: ver, Sequences: nil}

	switch {
	case ver == 1:
		for i := 0; i < int(cnt); i++ {
			idx, err := r.readI32()
			if err != nil {
				return nil, err
			}
			if int(idx) < 0 || int(idx) >= maxSequences {
				return nil, fmt.Errorf("SEQUENCES v1: index %d", idx)
			}
			sc, err := r.readU8()
			if err != nil {
				return nil, err
			}
			if int(sc) > maxSequenceItems-1 {
				return nil, fmt.Errorf("SEQUENCES v1: item count %d", sc)
			}
			items := make([]int8, 0, int(sc)*2)
			for j := 0; j < int(sc); j++ {
				a, err := r.readI8()
				if err != nil {
					return nil, err
				}
				b, err := r.readI8()
				if err != nil {
					return nil, err
				}
				items = append(items, a, b)
			}
			out.Sequences = append(out.Sequences, Sequence{
				Chip: "2A03", Index: int(idx), Type: -1, Items: items,
				LoopPoint: -1, ReleasePoint: -1, Setting: 0,
			})
		}
	case ver == 2:
		for i := 0; i < int(cnt); i++ {
			idx, err := r.readI32()
			if err != nil {
				return nil, err
			}
			if int(idx) < 0 || int(idx) >= maxSequences {
				return nil, fmt.Errorf("SEQUENCES v2: index %d", idx)
			}
			typ, err := r.readI32()
			if err != nil {
				return nil, err
			}
			if int(typ) < 0 || int(typ) >= seqCount {
				return nil, fmt.Errorf("SEQUENCES v2: type %d", typ)
			}
			sc, err := r.readU8()
			if err != nil {
				return nil, err
			}
			if int(sc) > maxSequenceItems-1 {
				return nil, fmt.Errorf("SEQUENCES v2: item count %d", sc)
			}
			items := make([]int8, 0, int(sc)*2)
			for j := 0; j < int(sc); j++ {
				a, err := r.readI8()
				if err != nil {
					return nil, err
				}
				b, err := r.readI8()
				if err != nil {
					return nil, err
				}
				items = append(items, a, b)
			}
			out.Sequences = append(out.Sequences, Sequence{
				Chip: "2A03", Index: int(idx), Type: int(typ), Items: items,
				LoopPoint: -1, ReleasePoint: -1, Setting: 0,
			})
		}
	case ver >= 3:
		type idxType struct{ idx, typ int }
		order := make([]idxType, 0, int(cnt))
		for i := 0; i < int(cnt); i++ {
			idx, err := r.readI32()
			if err != nil {
				return nil, err
			}
			if int(idx) < 0 || int(idx) >= maxSequences {
				return nil, fmt.Errorf("SEQUENCES: index %d", idx)
			}
			typ, err := r.readI32()
			if err != nil {
				return nil, err
			}
			if int(typ) < 0 || int(typ) >= seqCount {
				return nil, fmt.Errorf("SEQUENCES: type %d", typ)
			}
			sc, err := r.readU8()
			if err != nil {
				return nil, err
			}
			loop, err := r.readI32()
			if err != nil {
				return nil, err
			}
			if int(loop) < -1 || int(loop) > int(sc) {
				return nil, fmt.Errorf("SEQUENCES: loop %d (len %d)", loop, sc)
			}
			var loopPt int32 = -1
			if int(loop) != int(sc) {
				loopPt = loop
			}
			var rel int32 = -1
			var set int32
			if ver == 4 {
				rel, err = r.readI32()
				if err != nil {
					return nil, err
				}
				if int(rel) < -1 || int(rel) > int(sc)-1 {
					return nil, fmt.Errorf("SEQUENCES: release %d", rel)
				}
				set, err = r.readI32()
				if err != nil {
					return nil, err
				}
			}
			items := make([]int8, 0, int(sc))
			for j := 0; j < int(sc); j++ {
				v, err := r.readI8()
				if err != nil {
					return nil, err
				}
				if j < maxSequenceItems {
					items = append(items, v)
				}
			}
			s := Sequence{
				Chip: "2A03", Index: int(idx), Type: int(typ),
				Items: items, LoopPoint: loopPt, ReleasePoint: rel, Setting: set,
			}
			out.Sequences = append(out.Sequences, s)
			order = append(order, idxType{int(idx), int(typ)})
		}
		if ver == 5 {
			for idx := 0; idx < maxSequences; idx++ {
				for typ := 0; typ < seqCount; typ++ {
					rel, err := r.readI32()
					if err != nil {
						return nil, err
					}
					set, err := r.readI32()
					if err != nil {
						return nil, err
					}
					s := findSequence(out.Sequences, idx, typ)
					if s == nil || len(s.Items) == 0 {
						continue
					}
					if int(rel) < -1 || int(rel) > len(s.Items)-1 {
						return nil, fmt.Errorf("SEQUENCES v5 patch: release %d", rel)
					}
					s.ReleasePoint = rel
					s.Setting = set
				}
			}
		} else if ver >= 6 {
			for i := 0; i < int(cnt); i++ {
				rel, err := r.readI32()
				if err != nil {
					return nil, err
				}
				set, err := r.readI32()
				if err != nil {
					return nil, err
				}
				s := findSequence(out.Sequences, order[i].idx, order[i].typ)
				if s == nil {
					return nil, fmt.Errorf("SEQUENCES v6: missing sequence")
				}
				if int(rel) < -1 || int(rel) > len(s.Items)-1 {
					return nil, fmt.Errorf("SEQUENCES v6: release %d", rel)
				}
				s.ReleasePoint = rel
				s.Setting = set
			}
		}
	default:
		return nil, fmt.Errorf("SEQUENCES: unsupported block version %d", ver)
	}
	return out, nil
}

func findSequence(seqs []Sequence, index, typ int) *Sequence {
	for i := range seqs {
		if seqs[i].Index == index && seqs[i].Type == typ {
			return &seqs[i]
		}
	}
	return nil
}

func decodeSequencesVRC6Block(data []byte, ver int) (*SequencesChipBlock, error) {
	r := newBlockReader(data)
	cnt, err := r.readI32()
	if err != nil {
		return nil, err
	}
	if int(cnt) < 0 || int(cnt) > maxSequences*seqCount {
		return nil, fmt.Errorf("SEQUENCES_VRC6: count %d", cnt)
	}
	type idxType struct{ idx, typ int }
	order := make([]idxType, 0, int(cnt))
	out := &SequencesChipBlock{Version: ver, Chip: "VRC6", Sequences: nil}

	for i := 0; i < int(cnt); i++ {
		idx, err := r.readI32()
		if err != nil {
			return nil, err
		}
		if int(idx) < 0 || int(idx) >= maxSequences {
			return nil, fmt.Errorf("SEQUENCES_VRC6: index %d", idx)
		}
		typ, err := r.readI32()
		if err != nil {
			return nil, err
		}
		if int(typ) < 0 || int(typ) >= seqCount {
			return nil, fmt.Errorf("SEQUENCES_VRC6: type %d", typ)
		}
		sc, err := r.readU8()
		if err != nil {
			return nil, err
		}
		loop, err := r.readI32()
		if err != nil {
			return nil, err
		}
		if int(loop) < -1 || int(loop) > int(sc)-1 {
			return nil, fmt.Errorf("SEQUENCES_VRC6: loop %d", loop)
		}
		var rel int32 = -1
		var set int32
		if ver == 4 {
			rel, err = r.readI32()
			if err != nil {
				return nil, err
			}
			if int(rel) < -1 || int(rel) > int(sc)-1 {
				return nil, fmt.Errorf("SEQUENCES_VRC6: release %d", rel)
			}
			set, err = r.readI32()
			if err != nil {
				return nil, err
			}
		}
		items := make([]int8, 0, int(sc))
		for j := 0; j < int(sc); j++ {
			v, err := r.readI8()
			if err != nil {
				return nil, err
			}
			if j < maxSequenceItems {
				items = append(items, v)
			}
		}
		out.Sequences = append(out.Sequences, Sequence{
			Chip: "VRC6", Index: int(idx), Type: int(typ),
			Items: items, LoopPoint: loop, ReleasePoint: rel, Setting: set,
		})
		order = append(order, idxType{int(idx), int(typ)})
	}
	if ver == 5 {
		for idx := 0; idx < maxSequences; idx++ {
			for typ := 0; typ < seqCount; typ++ {
				rel, err := r.readI32()
				if err != nil {
					return nil, err
				}
				set, err := r.readI32()
				if err != nil {
					return nil, err
				}
				s := findSequence(out.Sequences, idx, typ)
				if s == nil || len(s.Items) == 0 {
					continue
				}
				if int(rel) < -1 || int(rel) > len(s.Items)-1 {
					return nil, fmt.Errorf("SEQUENCES_VRC6 v5 patch: release %d", rel)
				}
				s.ReleasePoint = rel
				s.Setting = set
			}
		}
	} else if ver >= 6 {
		for i := 0; i < int(cnt); i++ {
			rel, err := r.readI32()
			if err != nil {
				return nil, err
			}
			set, err := r.readI32()
			if err != nil {
				return nil, err
			}
			s := findSequence(out.Sequences, order[i].idx, order[i].typ)
			if s == nil {
				return nil, fmt.Errorf("SEQUENCES_VRC6 v6: missing sequence")
			}
			if int(rel) < -1 || int(rel) > len(s.Items)-1 {
				return nil, fmt.Errorf("SEQUENCES_VRC6 v6: release %d", rel)
			}
			s.ReleasePoint = rel
			s.Setting = set
		}
	}
	return out, nil
}

func decodeSequencesN163Block(data []byte, ver int) (*SequencesChipBlock, error) {
	_ = ver
	r := newBlockReader(data)
	cnt, err := r.readI32()
	if err != nil {
		return nil, err
	}
	if int(cnt) < 0 || int(cnt) > maxSequences*seqCount {
		return nil, fmt.Errorf("SEQUENCES_N163: count %d", cnt)
	}
	out := &SequencesChipBlock{Version: ver, Chip: "N163", Sequences: nil}
	for i := 0; i < int(cnt); i++ {
		idx, err := r.readI32()
		if err != nil {
			return nil, err
		}
		if int(idx) < 0 || int(idx) >= maxSequences {
			return nil, fmt.Errorf("SEQUENCES_N163: index %d", idx)
		}
		typ, err := r.readI32()
		if err != nil {
			return nil, err
		}
		if int(typ) < 0 || int(typ) >= seqCount {
			return nil, fmt.Errorf("SEQUENCES_N163: type %d", typ)
		}
		sc, err := r.readU8()
		if err != nil {
			return nil, err
		}
		loop, err := r.readI32()
		if err != nil {
			return nil, err
		}
		if int(loop) < -1 || int(loop) > int(sc)-1 {
			return nil, fmt.Errorf("SEQUENCES_N163: loop %d", loop)
		}
		rel, err := r.readI32()
		if err != nil {
			return nil, err
		}
		if int(rel) < -1 || int(rel) > int(sc)-1 {
			return nil, fmt.Errorf("SEQUENCES_N163: release %d", rel)
		}
		set, err := r.readI32()
		if err != nil {
			return nil, err
		}
		items := make([]int8, 0, int(sc))
		for j := 0; j < int(sc); j++ {
			v, err := r.readI8()
			if err != nil {
				return nil, err
			}
			if j < maxSequenceItems {
				items = append(items, v)
			}
		}
		out.Sequences = append(out.Sequences, Sequence{
			Chip: "N163", Index: int(idx), Type: int(typ),
			Items: items, LoopPoint: loop, ReleasePoint: rel, Setting: set,
		})
	}
	return out, nil
}

func decodeSequencesS5BBlock(data []byte, ver int) (*SequencesChipBlock, error) {
	b, err := decodeSequencesN163Block(data, ver)
	if err != nil {
		return nil, err
	}
	b.Chip = "S5B"
	return b, nil
}
