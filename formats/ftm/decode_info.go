package ftm

import (
	"bytes"
	"fmt"
	"strings"
)

func decodeInfoBlock(data []byte, ver int) (*InfoBlock, error) {
	_ = ver
	if len(data) < 96 {
		return nil, fmt.Errorf("INFO: need 96 bytes, got %d", len(data))
	}
	trim := func(b []byte) string {
		if i := bytes.IndexByte(b, 0); i >= 0 {
			b = b[:i]
		}
		return strings.TrimRight(string(b), " \x00")
	}
	return &InfoBlock{
		Name:      trim(data[0:32]),
		Artist:    trim(data[32:64]),
		Copyright: trim(data[64:96]),
	}, nil
}

func decodeTuningBlock(data []byte, ver int) (*TuningBlock, error) {
	if ver != 1 {
		return nil, fmt.Errorf("TUNING: unsupported block version %d", ver)
	}
	r := newBlockReader(data)
	semi, err := r.readI8()
	if err != nil {
		return nil, err
	}
	cent, err := r.readI8()
	if err != nil {
		return nil, err
	}
	if semi < -12 || semi > 12 {
		return nil, fmt.Errorf("TUNING: semitone %d", semi)
	}
	if cent < -100 || cent > 100 {
		return nil, fmt.Errorf("TUNING: cent %d", cent)
	}
	return &TuningBlock{Version: ver, Semitone: semi, Cent: cent}, r.assertFullyConsumed("TUNING")
}
