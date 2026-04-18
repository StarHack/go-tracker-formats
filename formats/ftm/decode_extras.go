package ftm

import "fmt"

func decodeDetuneTablesBlock(data []byte, ver int) (*DetuneTablesBlock, error) {
	_ = ver
	r := newBlockReader(data)
	n, err := r.readU8()
	if err != nil {
		return nil, err
	}
	if int(n) < 0 || int(n) > 6 {
		return nil, fmt.Errorf("DETUNETABLES: count %d", n)
	}
	out := &DetuneTablesBlock{Version: ver, Tables: nil}
	for i := 0; i < int(n); i++ {
		chip, err := r.readU8()
		if err != nil {
			return nil, err
		}
		if int(chip) < 0 || int(chip) > 5 {
			return nil, fmt.Errorf("DETUNETABLES: chip %d", chip)
		}
		items, err := r.readU8()
		if err != nil {
			return nil, err
		}
		if int(items) < 0 || int(items) > noteCount {
			return nil, fmt.Errorf("DETUNETABLES: item count %d", items)
		}
		t := DetuneTable{Chip: int(chip), Notes: nil}
		for j := 0; j < int(items); j++ {
			note, err := r.readU8()
			if err != nil {
				return nil, err
			}
			if int(note) < 0 || int(note) >= noteCount {
				return nil, fmt.Errorf("DETUNETABLES: note %d", note)
			}
			off, err := r.readI32()
			if err != nil {
				return nil, err
			}
			t.Notes = append(t.Notes, DetuneNote{NoteIndex: int(note), Offset: off})
		}
		out.Tables = append(out.Tables, t)
	}
	return out, nil
}

func decodeGroovesBlock(data []byte, ver int, m *Module) (*GroovesBlock, error) {
	_ = ver
	r := newBlockReader(data)
	n, err := r.readU8()
	if err != nil {
		return nil, err
	}
	if int(n) < 0 || int(n) > maxGroove {
		return nil, fmt.Errorf("GROOVES: count %d", n)
	}
	out := &GroovesBlock{Version: ver, Grooves: nil}
	for i := 0; i < int(n); i++ {
		idx, err := r.readU8()
		if err != nil {
			return nil, err
		}
		if int(idx) >= maxGroove {
			return nil, fmt.Errorf("GROOVES: index %d", idx)
		}
		sz, err := r.readU8()
		if err != nil {
			return nil, err
		}
		if int(sz) < 1 || int(sz) > maxGrooveSize {
			return nil, fmt.Errorf("GROOVES: size %d", sz)
		}
		ent := make([]byte, int(sz))
		for j := range ent {
			b, err := r.readU8()
			if err != nil {
				return nil, err
			}
			if int(b) < 1 || int(b) > 0xff {
				return nil, fmt.Errorf("GROOVES: entry %d", b)
			}
			ent[j] = b
		}
		out.Grooves = append(out.Grooves, Groove{Index: int(idx), Entries: ent})
	}
	tracks, err := r.readU8()
	if err != nil {
		return nil, err
	}
	expect := byte(1)
	if m != nil && m.Header != nil && m.Header.TrackCount > 0 {
		expect = byte(m.Header.TrackCount)
	}
	if tracks != expect {
		return nil, fmt.Errorf("GROOVES: track flag count %d want %d", tracks, expect)
	}
	out.TrackFlags = make([]byte, int(tracks))
	for i := range out.TrackFlags {
		u, err := r.readU8()
		if err != nil {
			return nil, err
		}
		out.TrackFlags[i] = u
	}
	return out, r.assertFullyConsumed("GROOVES")
}

func decodeBookmarksBlock(data []byte, m *Module) (*BookmarksBlock, error) {
	r := newBlockReader(data)
	cnt, err := r.readI32()
	if err != nil {
		return nil, err
	}
	if int(cnt) < 0 || int(cnt) > 4096 {
		return nil, fmt.Errorf("BOOKMARKS: count %d", cnt)
	}
	out := &BookmarksBlock{Entries: nil}
	tracks := 1
	if m != nil && m.Header != nil && m.Header.TrackCount > 0 {
		tracks = m.Header.TrackCount
	}
	for i := 0; i < int(cnt); i++ {
		tv, err := r.readU8()
		if err != nil {
			return nil, err
		}
		if int(tv) < 0 || int(tv) >= tracks {
			return nil, fmt.Errorf("BOOKMARKS: track %d", tv)
		}
		fr, err := r.readU8()
		if err != nil {
			return nil, err
		}
		row, err := r.readU8()
		if err != nil {
			return nil, err
		}
		if m != nil && m.Frames != nil && int(tv) < len(m.Frames.Tracks) {
			tf := m.Frames.Tracks[int(tv)]
			if tf.FrameCount > 0 && (int(fr) < 0 || int(fr) > tf.FrameCount-1) {
				return nil, fmt.Errorf("BOOKMARKS: frame %d (track %d max %d)", fr, tv, tf.FrameCount-1)
			}
			if tf.PatternLength > 0 && (int(row) < 0 || int(row) > tf.PatternLength-1) {
				return nil, fmt.Errorf("BOOKMARKS: row %d (track %d max %d)", row, tv, tf.PatternLength-1)
			}
		}
		h1, err := r.readI32()
		if err != nil {
			return nil, err
		}
		h2, err := r.readI32()
		if err != nil {
			return nil, err
		}
		persist, err := r.readU8()
		if err != nil {
			return nil, err
		}
		name, err := r.readCString(4096)
		if err != nil {
			return nil, err
		}
		out.Entries = append(out.Entries, Bookmark{
			Track:      int(tv),
			Frame:      int(fr),
			Row:        int(row),
			Highlight1: int(h1),
			Highlight2: int(h2),
			Following:  persist != 0,
			Name:       name,
		})
	}
	return out, r.assertFullyConsumed("BOOKMARKS")
}

func decodeParamsExtraBlock(data []byte, ver int) (*ParamsExtraBlock, error) {
	r := newBlockReader(data)
	lin, err := r.readI32()
	if err != nil {
		return nil, err
	}
	out := &ParamsExtraBlock{Version: ver, LinearPitch: lin != 0}
	if ver >= 2 {
		s, err := r.readI8()
		if err != nil {
			return nil, err
		}
		if s < -12 || s > 12 {
			return nil, fmt.Errorf("PARAMS_EXTRA: semitone %d", s)
		}
		out.Semitone = s
		c, err := r.readI8()
		if err != nil {
			return nil, err
		}
		if c < -100 || c > 100 {
			return nil, fmt.Errorf("PARAMS_EXTRA: cent %d", c)
		}
		out.Cent = c
	}
	return out, r.assertFullyConsumed("PARAMS_EXTRA")
}
