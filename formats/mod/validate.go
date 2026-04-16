// validate.go - MOD file validator.
// Supports ProTracker M.K., M!K!, FLT4/8, xCHN, xxCH formats.
package mod

import "fmt"

// numChannelsFromTag returns the channel count for a known MOD tag, or 0.
func numChannelsFromTag(tag [4]byte) int {
	s := string(tag[:])
	switch s {
	case "M.K.", "M!K!", "FLT4", "OCTA":
		return 4
	case "FLT8", "CD81":
		return 8
	}
	// xCHN: e.g. "4CHN","6CHN","8CHN"
	if tag[1] == 'C' && tag[2] == 'H' && tag[3] == 'N' && tag[0] >= '1' && tag[0] <= '9' {
		return int(tag[0] - '0')
	}
	// xxCH: e.g. "10CH","12CH"…"32CH"
	if tag[2] == 'C' && tag[3] == 'H' && tag[0] >= '0' && tag[0] <= '9' && tag[1] >= '0' && tag[1] <= '9' {
		return int(tag[0]-'0')*10 + int(tag[1]-'0')
	}
	return 0
}

// Validate checks a MOD file for basic validity.
// Returns "" if valid, or an error message.
func Validate(data []byte) string {
	// Minimum size: 20 (title) + 31*30 (samples) + 1 (songLen) + 1 (restart)
	// + 128 (orders) + 4 (tag) = 1084 bytes before any pattern data.
	if len(data) < 1084 {
		return "Not a MOD file: too short."
	}
	var tag [4]byte
	copy(tag[:], data[1080:1084])
	nch := numChannelsFromTag(tag)
	if nch == 0 {
		return fmt.Sprintf("Not a recognised MOD format tag: %q.", string(tag[:]))
	}

	// Song length
	songLen := int(data[950])
	if songLen == 0 || songLen > 128 {
		return "MOD song length is invalid."
	}

	// Find highest pattern number referenced
	maxPat := 0
	for i := 0; i < songLen; i++ {
		p := int(data[952+i])
		if p > maxPat {
			maxPat = p
		}
	}

	// Verify pattern data fits
	patternBytes := (maxPat + 1) * 64 * nch * 4
	if len(data) < 1084+patternBytes {
		return "MOD file is truncated (pattern data missing)."
	}

	// Verify sample data fits
	offset := 1084 + patternBytes
	for i := 0; i < 31; i++ {
		base := 20 + i*30
		wordLen := int(data[base+22])<<8 | int(data[base+23])
		byteLen := wordLen * 2
		if len(data) < offset+byteLen {
			return fmt.Sprintf("MOD file is truncated (sample %d data missing).", i+1)
		}
		offset += byteLen
	}
	return ""
}
