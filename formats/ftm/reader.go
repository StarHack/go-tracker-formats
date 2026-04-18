package ftm

import (
	"encoding/binary"
	"fmt"
)

// blockReader reads little-endian fields from a single FTM block payload.
type blockReader struct {
	b   []byte
	off int
}

func newBlockReader(b []byte) *blockReader { return &blockReader{b: b} }

func (r *blockReader) done() bool { return r.off >= len(r.b) }

func (r *blockReader) remaining() int {
	if r.off > len(r.b) {
		return 0
	}
	return len(r.b) - r.off
}

func (r *blockReader) need(n int) error {
	if r.off+n > len(r.b) {
		return fmt.Errorf("FTM block truncated at offset %d (need %d bytes)", r.off, n)
	}
	return nil
}

func (r *blockReader) readU32() (uint32, error) {
	if err := r.need(4); err != nil {
		return 0, err
	}
	v := binary.LittleEndian.Uint32(r.b[r.off:])
	r.off += 4
	return v, nil
}

func (r *blockReader) readI32() (int32, error) {
	u, err := r.readU32()
	if err != nil {
		return 0, err
	}
	return int32(u), nil
}

func (r *blockReader) readU8() (byte, error) {
	if err := r.need(1); err != nil {
		return 0, err
	}
	v := r.b[r.off]
	r.off++
	return v, nil
}

func (r *blockReader) readI8() (int8, error) {
	u, err := r.readU8()
	return int8(u), err
}

func (r *blockReader) readBytes(n int) ([]byte, error) {
	if err := r.need(n); err != nil {
		return nil, err
	}
	out := append([]byte(nil), r.b[r.off:r.off+n]...)
	r.off += n
	return out, nil
}

// readCString reads bytes until NUL or maxBytes (exclusive of terminator).
func (r *blockReader) readCString(maxBytes int) (string, error) {
	start := r.off
	for i := 0; i < maxBytes; i++ {
		if r.off >= len(r.b) {
			return "", fmt.Errorf("unterminated string in FTM block")
		}
		if r.b[r.off] == 0 {
			s := string(r.b[start:r.off])
			r.off++
			return s, nil
		}
		r.off++
	}
	return "", fmt.Errorf("FTM string exceeds %d bytes", maxBytes)
}

func (r *blockReader) skip(n int) error {
	if err := r.need(n); err != nil {
		return err
	}
	r.off += n
	return nil
}

func (r *blockReader) assertFullyConsumed(context string) error {
	if r.off != len(r.b) {
		return fmt.Errorf("%s: %d trailing bytes in block", context, len(r.b)-r.off)
	}
	return nil
}
