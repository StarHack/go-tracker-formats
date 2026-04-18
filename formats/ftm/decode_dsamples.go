package ftm

import "fmt"

func decodeDSamplesBlock(data []byte, ver int) (*DSamplesBlock, error) {
	_ = ver
	r := newBlockReader(data)
	n, err := r.readU8()
	if err != nil {
		return nil, err
	}
	if int(n) > maxDSamples {
		return nil, fmt.Errorf("DPCM SAMPLES: count %d", n)
	}
	out := &DSamplesBlock{Samples: make([]DSample, 0, int(n))}
	for i := 0; i < int(n); i++ {
		idx, err := r.readU8()
		if err != nil {
			return nil, err
		}
		if int(idx) >= maxDSamples {
			return nil, fmt.Errorf("DPCM SAMPLES: index %d", idx)
		}
		nameLen, err := r.readI32()
		if err != nil {
			return nil, err
		}
		if int(nameLen) < 0 || int(nameLen) >= dsampleNameMax {
			return nil, fmt.Errorf("DPCM SAMPLES: name length %d", nameLen)
		}
		nameBytes, err := r.readBytes(int(nameLen))
		if err != nil {
			return nil, err
		}
		sz, err := r.readI32()
		if err != nil {
			return nil, err
		}
		if int(sz) < 0 || int(sz) > 0x7fff {
			return nil, fmt.Errorf("DPCM SAMPLES: size %d", sz)
		}
		if int(sz) > 0xff1 || int(sz)%0x10 != 1 {
			return nil, fmt.Errorf("DPCM SAMPLES: invalid size %d (must be ≡1 mod 16 and ≤0xFF1)", sz)
		}
		raw, err := r.readBytes(int(sz))
		if err != nil {
			return nil, err
		}
		out.Samples = append(out.Samples, DSample{
			Index: int(idx),
			Name:  string(nameBytes),
			Data:  raw,
		})
	}
	return out, r.assertFullyConsumed("DPCM SAMPLES")
}
