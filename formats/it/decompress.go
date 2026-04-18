package it

import (
	"encoding/binary"
	"fmt"
)

const itBlockBytes = 0x8000

func changeWidth(cur *int, width int) {
	width++
	if width >= *cur {
		width++
	}
	*cur = width
}

type bitReader struct {
	data []byte
	bit  int
	pos  int
}

func (b *bitReader) readBits(width int) (int, error) {
	if width <= 0 || width > 32 {
		return 0, fmt.Errorf("invalid bit width %d", width)
	}
	v := 0
	got := 0
	for got < width {
		if b.pos >= len(b.data) {
			return 0, fmt.Errorf("unexpected EOF in compressed block")
		}
		if b.bit == 8 {
			b.pos++
			b.bit = 0
			if b.pos >= len(b.data) {
				return 0, fmt.Errorf("unexpected EOF in compressed block")
			}
		}
		avail := 8 - b.bit
		take := width - got
		if take > avail {
			take = avail
		}
		mask := (1 << take) - 1
		chunk := int(b.data[b.pos]>>b.bit) & mask
		v |= chunk << got
		got += take
		b.bit += take
		if b.bit == 8 {
			b.pos++
			b.bit = 0
		}
	}
	return v, nil
}

func decompressIT8(data []byte, length int, it215 bool) ([]int8, error) {
	const (
		defWidth = 9
		fetchA   = 3
		lowerB   = -4
		upperB   = 3
	)
	out := make([]int8, length)
	written := 0
	mem1, mem2 := 0, 0
	pos := 0
	for written < length {
		if pos+2 > len(data) {
			return nil, fmt.Errorf("truncated IT compressed 8-bit stream")
		}
		blk := int(binary.LittleEndian.Uint16(data[pos:]))
		pos += 2
		if blk == 0 {
			continue
		}
		if pos+blk > len(data) {
			return nil, fmt.Errorf("truncated IT compressed 8-bit block")
		}
		br := bitReader{data: data[pos : pos+blk]}
		pos += blk

		curLen := length - written
		if curLen > itBlockBytes {
			curLen = itBlockBytes
		}
		width := defWidth
		for curLen > 0 {
			if width > defWidth {
				return nil, fmt.Errorf("invalid IT8 bit width")
			}
			v, err := br.readBits(width)
			if err != nil {
				return out[:written], err
			}
			top := 1 << (width - 1)
			if width <= 6 {
				if v == top {
					nw, err := br.readBits(fetchA)
					if err != nil {
						return out[:written], err
					}
					changeWidth(&width, nw)
					continue
				}
				dv := v
				if dv&top != 0 {
					dv -= top << 1
				}
				mem1 += dv
				if it215 {
					mem2 += mem1
					out[written] = int8(mem2)
				} else {
					out[written] = int8(mem1)
				}
				written++
				curLen--
			} else if width < defWidth {
				if v >= top+lowerB && v <= top+upperB {
					changeWidth(&width, v-(top+lowerB))
					continue
				}
				dv := v
				if dv&top != 0 {
					dv -= top << 1
				}
				mem1 += dv
				if it215 {
					mem2 += mem1
					out[written] = int8(mem2)
				} else {
					out[written] = int8(mem1)
				}
				written++
				curLen--
			} else {
				if v&top != 0 {
					width = (v &^ top) + 1
					continue
				}
				dv := v &^ top
				mem1 += dv
				if it215 {
					mem2 += mem1
					out[written] = int8(mem2)
				} else {
					out[written] = int8(mem1)
				}
				written++
				curLen--
			}
		}
	}
	if written < length {
		return nil, fmt.Errorf("IT8 decompression short: got %d want %d", written, length)
	}
	return out, nil
}

func decompressIT16(data []byte, length int, it215 bool) ([]int16, error) {
	const (
		defWidth = 17
		fetchA   = 4
		lowerB   = -8
		upperB   = 7
	)
	out := make([]int16, length)
	written := 0
	mem1, mem2 := 0, 0
	pos := 0
	for written < length {
		if pos+2 > len(data) {
			return nil, fmt.Errorf("truncated IT compressed 16-bit stream")
		}
		blk := int(binary.LittleEndian.Uint16(data[pos:]))
		pos += 2
		if blk == 0 {
			continue
		}
		if pos+blk > len(data) {
			return nil, fmt.Errorf("truncated IT compressed 16-bit block")
		}
		br := bitReader{data: data[pos : pos+blk]}
		pos += blk

		curLen := length - written
		if curLen > itBlockBytes/2 {
			curLen = itBlockBytes / 2
		}
		width := defWidth
		for curLen > 0 {
			if width > defWidth {
				return nil, fmt.Errorf("invalid IT16 bit width")
			}
			v, err := br.readBits(width)
			if err != nil {
				return out[:written], err
			}
			top := 1 << (width - 1)
			if width <= 6 {
				if v == top {
					nw, err := br.readBits(fetchA)
					if err != nil {
						return out[:written], err
					}
					changeWidth(&width, nw)
					continue
				}
				dv := v
				if dv&top != 0 {
					dv -= top << 1
				}
				mem1 += dv
				if it215 {
					mem2 += mem1
					out[written] = int16(mem2)
				} else {
					out[written] = int16(mem1)
				}
				written++
				curLen--
			} else if width < defWidth {
				if v >= top+lowerB && v <= top+upperB {
					changeWidth(&width, v-(top+lowerB))
					continue
				}
				dv := v
				if dv&top != 0 {
					dv -= top << 1
				}
				mem1 += dv
				if it215 {
					mem2 += mem1
					out[written] = int16(mem2)
				} else {
					out[written] = int16(mem1)
				}
				written++
				curLen--
			} else {
				if v&top != 0 {
					width = (v &^ top) + 1
					continue
				}
				dv := v &^ top
				mem1 += dv
				if it215 {
					mem2 += mem1
					out[written] = int16(mem2)
				} else {
					out[written] = int16(mem1)
				}
				written++
				curLen--
			}
		}
	}
	if written < length {
		return nil, fmt.Errorf("IT16 decompression short: got %d want %d", written, length)
	}
	return out, nil
}

func decompressIT16Stereo(data []byte, frames int, it215 bool) ([]int16, error) {
	const (
		defWidth = 17
		fetchA   = 4
		lowerB   = -8
		upperB   = 7
	)
	out := make([]int16, frames*2)
	pos := 0
	for ch := 0; ch < 2; ch++ {
		written := 0
		mem1, mem2 := 0, 0
		for written < frames {
			if pos+2 > len(data) {
				return nil, fmt.Errorf("truncated IT stereo compressed 16-bit stream")
			}
			blk := int(binary.LittleEndian.Uint16(data[pos:]))
			pos += 2
			if blk == 0 {
				continue
			}
			if pos+blk > len(data) {
				return nil, fmt.Errorf("truncated IT stereo compressed 16-bit block")
			}
			br := bitReader{data: data[pos : pos+blk]}
			pos += blk
			curLen := frames - written
			if curLen > itBlockBytes/2 {
				curLen = itBlockBytes / 2
			}
			width := defWidth
			for curLen > 0 {
				if width > defWidth {
					return nil, fmt.Errorf("invalid IT16 stereo bit width")
				}
				v, err := br.readBits(width)
				if err != nil {
					return nil, err
				}
				top := 1 << (width - 1)
				if width <= 6 {
					if v == top {
						nw, err := br.readBits(fetchA)
						if err != nil {
							return nil, err
						}
						changeWidth(&width, nw)
						continue
					}
					dv := v
					if dv&top != 0 {
						dv -= top << 1
					}
					mem1 += dv
					var sv int16
					if it215 {
						mem2 += mem1
						sv = int16(mem2)
					} else {
						sv = int16(mem1)
					}
					out[written*2+ch] = sv
					written++
					curLen--
				} else if width < defWidth {
					if v >= top+lowerB && v <= top+upperB {
						changeWidth(&width, v-(top+lowerB))
						continue
					}
					dv := v
					if dv&top != 0 {
						dv -= top << 1
					}
					mem1 += dv
					var sv int16
					if it215 {
						mem2 += mem1
						sv = int16(mem2)
					} else {
						sv = int16(mem1)
					}
					out[written*2+ch] = sv
					written++
					curLen--
				} else {
					if v&top != 0 {
						width = (v &^ top) + 1
						continue
					}
					dv := v &^ top
					mem1 += dv
					var sv int16
					if it215 {
						mem2 += mem1
						sv = int16(mem2)
					} else {
						sv = int16(mem1)
					}
					out[written*2+ch] = sv
					written++
					curLen--
				}
			}
		}
	}
	return out, nil
}

func decompressIT8Stereo(data []byte, frames int, it215 bool) ([]int16, error) {
	const (
		defWidth = 9
		fetchA   = 3
		lowerB   = -4
		upperB   = 3
	)
	out := make([]int16, frames*2)
	pos := 0
	for ch := 0; ch < 2; ch++ {
		written := 0
		mem1, mem2 := 0, 0
		for written < frames {
			if pos+2 > len(data) {
				return nil, fmt.Errorf("truncated IT stereo compressed 8-bit stream")
			}
			blk := int(binary.LittleEndian.Uint16(data[pos:]))
			pos += 2
			if blk == 0 {
				continue
			}
			if pos+blk > len(data) {
				return nil, fmt.Errorf("truncated IT stereo compressed 8-bit block")
			}
			br := bitReader{data: data[pos : pos+blk]}
			pos += blk
			curLen := frames - written
			if curLen > itBlockBytes {
				curLen = itBlockBytes
			}
			width := defWidth
			for curLen > 0 {
				if width > defWidth {
					return nil, fmt.Errorf("invalid IT8 stereo bit width")
				}
				v, err := br.readBits(width)
				if err != nil {
					return nil, err
				}
				top := 1 << (width - 1)
				var sv int8
				if width <= 6 {
					if v == top {
						nw, err := br.readBits(fetchA)
						if err != nil {
							return nil, err
						}
						changeWidth(&width, nw)
						continue
					}
					dv := v
					if dv&top != 0 {
						dv -= top << 1
					}
					mem1 += dv
					if it215 {
						mem2 += mem1
						sv = int8(mem2)
					} else {
						sv = int8(mem1)
					}
					out[written*2+ch] = int16(int32(sv) << 8)
					written++
					curLen--
					continue
				} else if width < defWidth {
					if v >= top+lowerB && v <= top+upperB {
						changeWidth(&width, v-(top+lowerB))
						continue
					}
					dv := v
					if dv&top != 0 {
						dv -= top << 1
					}
					mem1 += dv
					if it215 {
						mem2 += mem1
						sv = int8(mem2)
					} else {
						sv = int8(mem1)
					}
				} else {
					if v&top != 0 {
						width = (v &^ top) + 1
						continue
					}
					dv := v &^ top
					mem1 += dv
					if it215 {
						mem2 += mem1
						sv = int8(mem2)
					} else {
						sv = int8(mem1)
					}
				}
				out[written*2+ch] = int16(int32(sv) << 8)
				written++
				curLen--
			}
		}
	}
	return out, nil
}
