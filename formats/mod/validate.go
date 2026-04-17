package mod

import "fmt"

type modLayout struct {
	sampleCount int
	numChannels int
	headerSize  int
	formatName  string
}

// Validate checks a MOD file for basic validity.
// Returns nil if valid, or an error.
func Validate(data []byte) error {
	layout, ok := detectLayout(data)
	if !ok {
		if len(data) < 600 {
			return fmt.Errorf("not a MOD file: too short")
		}
		if len(data) >= 1084 {
			var tag [4]byte
			copy(tag[:], data[1080:1084])
			return fmt.Errorf("not a recognised MOD format tag: %q", string(tag[:]))
		}
		return fmt.Errorf("not a recognised MOD file")
	}
	return validateWithLayout(data, layout)
}

func validateWithLayout(data []byte, layout modLayout) error {
	songLenOff := 20 + layout.sampleCount*30
	ordersOff := songLenOff + 2

	if len(data) < layout.headerSize {
		return fmt.Errorf("not a MOD file: too short")
	}

	songLen := int(data[songLenOff])
	if songLen == 0 || songLen > 128 {
		return fmt.Errorf("MOD song length is invalid")
	}

	maxPat := 0
	for i := 0; i < songLen; i++ {
		p := int(data[ordersOff+i])
		if p > maxPat {
			maxPat = p
		}
	}

	patternBytes := (maxPat + 1) * 64 * layout.numChannels * 4
	if len(data) < layout.headerSize+patternBytes {
		return fmt.Errorf("MOD file is truncated (pattern data missing)")
	}

	offset := layout.headerSize + patternBytes
	for i := 0; i < layout.sampleCount; i++ {
		base := 20 + i*30
		if data[base+25] > 64 {
			return fmt.Errorf("MOD sample %d has invalid volume", i+1)
		}
		byteLen := (int(data[base+22])<<8 | int(data[base+23])) * 2
		if len(data) < offset+byteLen {
			return fmt.Errorf("MOD file is truncated (sample %d data missing)", i+1)
		}
		offset += byteLen
	}
	return nil
}

func detectLayout(data []byte) (modLayout, bool) {
	if len(data) >= 1084 {
		var tag [4]byte
		copy(tag[:], data[1080:1084])
		if nch := numChannelsFromTag(tag); nch != 0 {
			return modLayout{
				sampleCount: 31,
				numChannels: nch,
				headerSize:  1084,
				formatName:  string(tag[:]),
			}, true
		}
	}
	if looksLike15SampleMOD(data) {
		return modLayout{
			sampleCount: 15,
			numChannels: 4,
			headerSize:  600,
			formatName:  "15-sample",
		}, true
	}
	return modLayout{}, false
}

func looksLike15SampleMOD(data []byte) bool {
	if len(data) < 600 {
		return false
	}
	for i := 0; i < 15; i++ {
		base := 20 + i*30
		if data[base+25] > 64 {
			return false
		}
	}
	songLen := int(data[470])
	if songLen == 0 || songLen > 128 {
		return false
	}
	maxPat := 0
	for i := 0; i < songLen; i++ {
		p := int(data[472+i])
		if p > 127 {
			return false
		}
		if p > maxPat {
			maxPat = p
		}
	}
	patternBytes := (maxPat + 1) * 64 * 4 * 4
	if len(data) < 600+patternBytes {
		return false
	}
	offset := 600 + patternBytes
	for i := 0; i < 15; i++ {
		base := 20 + i*30
		byteLen := (int(data[base+22])<<8 | int(data[base+23])) * 2
		if len(data) < offset+byteLen {
			return false
		}
		offset += byteLen
	}
	return true
}

func numChannelsFromTag(tag [4]byte) int {
	s := string(tag[:])
	switch s {
	case "M.K.", "M!K!", "FLT4", "OCTA":
		return 4
	case "FLT8", "CD81":
		return 8
	}
	if tag[1] == 'C' && tag[2] == 'H' && tag[3] == 'N' && tag[0] >= '1' && tag[0] <= '9' {
		return int(tag[0] - '0')
	}
	if tag[2] == 'C' && tag[3] == 'H' && tag[0] >= '0' && tag[0] <= '9' && tag[1] >= '0' && tag[1] <= '9' {
		return int(tag[0]-'0')*10 + int(tag[1]-'0')
	}
	return 0
}
