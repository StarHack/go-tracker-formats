package ftm

import "fmt"

// LoadModule parses a full .ftm file and decodes all recognized blocks into m.
// Blocks are applied in file order; later blocks overwrite earlier ones of the same ID.
func LoadModule(data []byte) (*Module, error) {
	dn, fv, off, err := parseFileHeader(data)
	if err != nil {
		return nil, err
	}
	if fv < fileVerMin || fv > fileVerMax {
		return nil, fmt.Errorf("FTM: unsupported file version 0x%04X (supported 0x%04X–0x%04X)", fv, fileVerMin, fileVerMax)
	}
	chunks, err := parseChunks(data, off)
	if err != nil {
		return nil, err
	}
	m := &Module{DnModule: dn, FileVersion: fv}
	for _, ch := range chunks {
		if ch.ID == blockEnd {
			continue
		}
		v := int(ch.Version)
		switch ch.ID {
		case blockParams:
			m.Params, err = decodeParamsBlock(ch.Data, v)
		case blockTuning:
			m.Tuning, err = decodeTuningBlock(ch.Data, v)
		case blockInfo:
			m.Info, err = decodeInfoBlock(ch.Data, v)
		case blockHeader:
			if m.Params == nil {
				err = fmt.Errorf("HEADER: require PARAMS before HEADER")
				break
			}
			m.Header, err = decodeHeaderBlock(ch.Data, v, m.Params.Channels)
		case blockFrames:
			if m.Params == nil {
				err = fmt.Errorf("FRAMES: require PARAMS before FRAMES")
				break
			}
			tracks := 1
			if m.Header != nil && m.Header.TrackCount > 0 {
				tracks = m.Header.TrackCount
			}
			m.Frames, err = decodeFramesBlock(ch.Data, v, m.Params.Channels, tracks, m.Params.Machine)
		case blockPatterns:
			m.Patterns, err = decodePatternsBlock(ch.Data, v, m.FileVersion, m)
		case blockDSamples:
			m.DSamples, err = decodeDSamplesBlock(ch.Data, v)
		case blockComments:
			m.Comments, err = decodeCommentsBlock(ch.Data, v)
		case blockSequences:
			m.Sequences, err = decodeSequences2A03Block(ch.Data, v)
		case blockSeqVRC6:
			m.SeqVRC6, err = decodeSequencesVRC6Block(ch.Data, v)
		case blockSeqN163, blockSeqN106:
			m.SeqN163, err = decodeSequencesN163Block(ch.Data, v)
		case blockSeqS5B:
			m.SeqS5B, err = decodeSequencesS5BBlock(ch.Data, v)
		case blockInstruments:
			m.Instruments, err = decodeInstrumentsBlock(ch.Data, v)
		case blockDetune:
			m.DetuneTables, err = decodeDetuneTablesBlock(ch.Data, v)
		case blockGrooves:
			m.Grooves, err = decodeGroovesBlock(ch.Data, v, m)
		case blockBookmarks:
			m.Bookmarks, err = decodeBookmarksBlock(ch.Data, m)
		case blockParamsExtra:
			m.ParamsExtra, err = decodeParamsExtraBlock(ch.Data, v)
		case blockJSON:
			m.JSONBlock = append([]byte(nil), ch.Data...)
		case blockParamsEmu:
			m.ParamsEmu = append([]byte(nil), ch.Data...)
		default:
			m.Unknown = append(m.Unknown, ch)
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("FTM block %q v%d: %w", ch.ID, ch.Version, err)
		}
	}
	return m, nil
}
