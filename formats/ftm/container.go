package ftm

import (
	"bytes"
	"encoding/binary"
	"fmt"
)

func normalizeBlockID(id []byte) string {
	end := len(id)
	for i := range id {
		if id[i] == 0 {
			end = i
			break
		}
	}
	return string(id[:end])
}

func parseFileHeader(data []byte) (dn bool, fileVer uint32, bodyOff int, err error) {
	if len(data) < 4 {
		return false, 0, 0, fmt.Errorf("FTM: file too short")
	}
	switch {
	case bytes.HasPrefix(data, []byte(headerDn)):
		dn = true
		bodyOff = len(headerDn)
	case bytes.HasPrefix(data, []byte(headerFT)):
		bodyOff = len(headerFT)
	default:
		return false, 0, 0, fmt.Errorf("FTM: not a FamiTracker module (missing header id)")
	}
	if bodyOff+4 > len(data) {
		return false, 0, 0, fmt.Errorf("FTM: truncated after header")
	}
	fileVer = binary.LittleEndian.Uint32(data[bodyOff : bodyOff+4])
	return dn, fileVer, bodyOff + 4, nil
}

// isLegacyASCIIEndTerminator reports a short EOF marker used by some older
// modules: ASCII "END" (optionally NUL-padded) without a full 24-byte block header.
func isLegacyASCIIEndTerminator(tail []byte) bool {
	tail = bytes.TrimRight(tail, "\x00")
	if len(tail) < 3 || !bytes.Equal(tail[:3], []byte("END")) {
		return false
	}
	return len(bytes.Trim(tail[3:], "\x00")) == 0
}

// parseChunks reads all blocks after the file version dword until END (inclusive of END chunk).
func parseChunks(data []byte, off int) ([]Chunk, error) {
	var out []Chunk
	for {
		if off >= len(data) {
			break
		}
		remaining := len(data) - off
		if remaining < 24 {
			// Short ASCII "END" after last block (no version/size/payload).
			if isLegacyASCIIEndTerminator(data[off:]) {
				out = append(out, Chunk{ID: blockEnd, Version: 0, Data: nil})
				return out, nil
			}
			// Trailing bytes after a well-formed END chunk (Dn tolerates junk here).
			if len(out) > 0 && out[len(out)-1].ID == blockEnd {
				return out, nil
			}
			return nil, fmt.Errorf("FTM: truncated block header at %d (need %d bytes, have %d)", off, 24, remaining)
		}
		var rawID [16]byte
		copy(rawID[:], data[off:off+16])
		id := normalizeBlockID(rawID[:])
		ver := binary.LittleEndian.Uint32(data[off+16 : off+20])
		sz := binary.LittleEndian.Uint32(data[off+20 : off+24])
		if sz > blockPayloadMax {
			return nil, fmt.Errorf("FTM: block %q claims size %d (too large)", id, sz)
		}
		payloadOff := off + 24
		if payloadOff+int(sz) > len(data) {
			return nil, fmt.Errorf("FTM: block %q payload extends past EOF", id)
		}
		payload := append([]byte(nil), data[payloadOff:payloadOff+int(sz)]...)
		out = append(out, Chunk{ID: id, Version: ver, Data: payload})
		off = payloadOff + int(sz)
		if id == blockEnd {
			break
		}
	}
	if len(out) == 0 || out[len(out)-1].ID != blockEnd {
		return nil, fmt.Errorf("FTM: missing END block")
	}
	return out, nil
}
