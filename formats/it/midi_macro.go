package it

import "strings"

// IT embedded MIDI macro configuration (OpenMPT MIDIMacroConfigData: 4896 bytes on disk).
const (
	itMidiGlobalCount = 9
	itMidiSFxCount    = 16
	itMidiZxxCount    = 128
	itMidiMacroBytes  = 32
	itMidiConfigSize  = itMidiGlobalCount*itMidiMacroBytes + itMidiSFxCount*itMidiMacroBytes + itMidiZxxCount*itMidiMacroBytes // 4896
)

// ITMidiMacroConfig holds raw 32-byte C strings from an IT file (when embedded).
type ITMidiMacroConfig struct {
	Global [itMidiGlobalCount][itMidiMacroBytes]byte
	SFx    [itMidiSFxCount][itMidiMacroBytes]byte
	Zxx    [itMidiZxxCount][itMidiMacroBytes]byte
	Valid  bool // read successfully from file
}

type itSFxKind byte

const (
	sfxUnused itSFxKind = iota
	sfxCutoff
	sfxResonance
	sfxFilterMode
	sfxDryWet
	sfxPlugParam
	sfxMIDIcc
	sfxChannelAT
	sfxPolyAT
	sfxPitch
	sfxProgChange
	sfxCustom
)

func itMacroNorm(m [itMidiMacroBytes]byte) string {
	end := 0
	for end < len(m) && m[end] != 0 {
		end++
	}
	s := string(m[:end])
	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c == ' ' || c == '\t' {
			continue
		}
		if c >= 'a' && c <= 'z' {
			c = c - 'a' + 'A'
		}
		b.WriteByte(c)
	}
	return b.String()
}

func itHexVal(c byte) (int, bool) {
	switch {
	case c >= '0' && c <= '9':
		return int(c - '0'), true
	case c >= 'A' && c <= 'F':
		return int(c-'A') + 10, true
	default:
		return 0, false
	}
}

func itParseHexByte(a, b byte) (int, bool) {
	ha, ok1 := itHexVal(a)
	hb, ok2 := itHexVal(b)
	if !ok1 || !ok2 {
		return 0, false
	}
	return ha*16 + hb, true
}

// itParseHexNibble3 parses three hex nibbles (12-bit value used in F0FhhhZ macros).
func itParseHexNibble3(a, b, c byte) (int, bool) {
	na, ok1 := itHexVal(a)
	nb, ok2 := itHexVal(b)
	nc, ok3 := itHexVal(c)
	if !ok1 || !ok2 || !ok3 {
		return 0, false
	}
	return (na << 8) | (nb << 4) | nc, true
}

// classifySFxMacro returns the OpenMPT parametered macro kind for one SFx slot.
func classifySFxMacro(m [itMidiMacroBytes]byte) itSFxKind {
	s := itMacroNorm(m)
	if s == "" {
		return sfxUnused
	}
	switch s {
	case "F0F000Z":
		return sfxCutoff
	case "F0F001Z":
		return sfxResonance
	case "F0F002Z":
		return sfxFilterMode
	case "F0F003Z":
		return sfxDryWet
	case "DCZ":
		return sfxChannelAT
	case "CCZ":
		return sfxProgChange
	}
	if len(s) >= 5 && strings.HasPrefix(s, "EC00") && s[4] == 'Z' {
		return sfxPitch
	}
	if len(s) >= 6 && strings.HasPrefix(s, "ACN") && s[5] == 'Z' {
		return sfxPolyAT
	}
	if len(s) == 7 && strings.HasPrefix(s, "F0F") && s[6] == 'Z' {
		if h, ok := itParseHexNibble3(s[3], s[4], s[5]); ok && h >= 0x80 {
			return sfxPlugParam
		}
	}
	if len(s) >= 5 && s[0] == 'B' && s[1] == 'C' {
		return sfxMIDIcc
	}
	return sfxCustom
}

func itHexDigit(c byte) bool {
	return (c >= '0' && c <= '9') || (c >= 'A' && c <= 'F')
}

func itReadMidiConfigBlock(b []byte) (ITMidiMacroConfig, bool) {
	var cfg ITMidiMacroConfig
	if len(b) < itMidiConfigSize {
		return cfg, false
	}
	o := 0
	for i := 0; i < itMidiGlobalCount; i++ {
		copy(cfg.Global[i][:], b[o:o+itMidiMacroBytes])
		o += itMidiMacroBytes
	}
	for i := 0; i < itMidiSFxCount; i++ {
		copy(cfg.SFx[i][:], b[o:o+itMidiMacroBytes])
		o += itMidiMacroBytes
	}
	for i := 0; i < itMidiZxxCount; i++ {
		copy(cfg.Zxx[i][:], b[o:o+itMidiMacroBytes])
		o += itMidiMacroBytes
	}
	cfg.Valid = true
	return cfg, true
}

// itSkipEditHistoryIfPresent skips the IT edit-history block when valid (same guard as OpenMPT).
func itSkipEditHistoryIfPresent(data []byte, off int, special uint16, minPtr int) int {
	if special&0x02 == 0 {
		return off
	}
	if off+2 > len(data) {
		return off
	}
	nhist := int(uint16(data[off]) | uint16(data[off+1])<<8)
	need := off + 2 + nhist*8
	if nhist < 0 || need > len(data) || need > minPtr {
		return off
	}
	return need
}

// itTryLoadMidiConfig reads embedded MIDIMacroConfigData after pointer tables and optional edit history.
func itTryLoadMidiConfig(data []byte, off int, flags, special uint16, ins, smp, pat []uint32, msgOff uint32, msgLen uint16) (ITMidiMacroConfig, int) {
	minPtr := int(^uint32(0) >> 1)
	for _, p := range ins {
		if p > 0 && int(p) < minPtr {
			minPtr = int(p)
		}
	}
	for _, p := range smp {
		if p > 0 && int(p) < minPtr {
			minPtr = int(p)
		}
	}
	for _, p := range pat {
		if p > 0 && int(p) < minPtr {
			minPtr = int(p)
		}
	}
	if special&0x01 != 0 && msgLen > 0 && msgOff > 0 && int(msgOff) < minPtr {
		minPtr = int(msgOff)
	}
	if minPtr < 0 || minPtr > len(data) {
		minPtr = len(data)
	}

	cur := itSkipEditHistoryIfPresent(data, off, special, minPtr)
	hasMidi := (flags&0x80) != 0 || (special&0x08) != 0
	if !hasMidi || cur+itMidiConfigSize > len(data) {
		return ITMidiMacroConfig{}, cur
	}
	cfg, ok := itReadMidiConfigBlock(data[cur : cur+itMidiConfigSize])
	if !ok {
		return ITMidiMacroConfig{}, cur
	}
	return cfg, cur + itMidiConfigSize
}

// itApplySFxMacro applies Z00–Z7F using the active SFx slot (parameter z = prm).
func itApplySFxMacro(p *Player, ch *itChannel, kind itSFxKind, macro [itMidiMacroBytes]byte, prm uint8) {
	switch kind {
	case sfxCutoff, sfxResonance, sfxFilterMode, sfxDryWet, sfxPlugParam:
		itApplyInternalMacroScan(p, ch, macro, prm)
	case sfxMIDIcc:
		cc, ok := itMacroParseBcCC(macro)
		if ok {
			itApplyMIDICC(p, ch, cc, int(prm))
		}
	case sfxChannelAT:
		itApplyChannelAftertouch(ch, int(prm))
	case sfxPitch:
		itApplyPitchBendMacro(ch, prm)
	case sfxPolyAT:
		itApplyPolyAftertouchMacro(ch, macro, prm)
	case sfxProgChange:
		p.itApplyMacroProgramChange(ch, prm)
	case sfxUnused:
		return
	case sfxCustom:
		itApplyInternalMacroScan(p, ch, macro, prm)
	}
}

// itApplyInternalMacroScan walks concatenated OpenMPT internal filter / dry-wet nibbles (F0F0…).
func itApplyInternalMacroScan(p *Player, ch *itChannel, macro [itMidiMacroBytes]byte, zParam uint8) {
	s := itMacroNorm(macro)
	i := 0
	for i < len(s) {
		if strings.HasPrefix(s[i:], "F0F000Z") {
			ch.fltCut = int(zParam)
			if ch.fltCut > 127 {
				ch.fltCut = 127
			}
			ch.itFilterMarkDirty()
			ch.itFilterResetState()
			i += 7
			continue
		}
		if strings.HasPrefix(s[i:], "F0F001Z") {
			ch.fltRes = int(zParam)
			if ch.fltRes > 127 {
				ch.fltRes = 127
			}
			ch.itFilterMarkDirty()
			ch.itFilterResetState()
			i += 7
			continue
		}
		if strings.HasPrefix(s[i:], "F0F002Z") {
			itSetFilterModeFromByte(ch, zParam)
			i += 7
			continue
		}
		if strings.HasPrefix(s[i:], "F0F003Z") {
			itApplyMacroDryWetFromMIDI(ch, int(zParam))
			i += 7
			continue
		}
		if len(s)-i >= 8 && strings.HasPrefix(s[i:], "F0F003") && itHexDigit(s[i+6]) && itHexDigit(s[i+7]) {
			if v, ok := itParseHexByte(s[i+6], s[i+7]); ok {
				itApplyMacroDryWetFromMIDI(ch, v)
			}
			i += 8
			continue
		}
		if len(s)-i >= 7 && strings.HasPrefix(s[i:], "F0F") && s[i+6] == 'Z' &&
			itHexDigit(s[i+3]) && itHexDigit(s[i+4]) && itHexDigit(s[i+5]) {
			if h, ok := itParseHexNibble3(s[i+3], s[i+4], s[i+5]); ok && h >= 0x80 {
				itApplyPluginParamByIndex(ch, h-0x80, zParam)
				i += 7
				continue
			}
		}
		if len(s)-i >= 6 && strings.HasPrefix(s[i:], "ACN") && s[i+5] == 'Z' {
			if note, ok := itParseHexByte(s[i+3], s[i+4]); ok {
				itApplyPolyAftertouchAtNote(ch, note, int(zParam))
			}
			i += 6
			continue
		}
		if strings.HasPrefix(s[i:], "EC00Z") {
			itApplyPitchBendMacro(ch, zParam)
			i += 5
			continue
		}
		if strings.HasPrefix(s[i:], "CCZ") {
			p.itApplyMacroProgramChange(ch, zParam)
			i += 3
			continue
		}
		if len(s)-i >= 8 && strings.HasPrefix(s[i:], "F0F000") && itHexDigit(s[i+6]) && itHexDigit(s[i+7]) {
			if v, ok := itParseHexByte(s[i+6], s[i+7]); ok {
				ch.fltCut = clampInt(v, 0, 127)
				ch.itFilterMarkDirty()
				ch.itFilterResetState()
			}
			i += 8
			continue
		}
		if len(s)-i >= 8 && strings.HasPrefix(s[i:], "F0F001") && itHexDigit(s[i+6]) && itHexDigit(s[i+7]) {
			if v, ok := itParseHexByte(s[i+6], s[i+7]); ok {
				ch.fltRes = clampInt(v, 0, 127)
				ch.itFilterMarkDirty()
				ch.itFilterResetState()
			}
			i += 8
			continue
		}
		if len(s)-i >= 8 && strings.HasPrefix(s[i:], "F0F002") && itHexDigit(s[i+6]) && itHexDigit(s[i+7]) {
			if v, ok := itParseHexByte(s[i+6], s[i+7]); ok {
				if v == 0 {
					ch.fltHiPass = false
				} else if v == 0x10 {
					ch.fltHiPass = true
				}
				ch.itFilterMarkDirty()
				ch.itFilterResetState()
			}
			i += 8
			continue
		}
		i++
	}
}

func itSetFilterModeFromByte(ch *itChannel, prm uint8) {
	v := int(prm)
	if v == 0 {
		ch.fltHiPass = false
	} else if v == 0x10 {
		ch.fltHiPass = true
	}
	ch.itFilterMarkDirty()
	ch.itFilterResetState()
}

// itApplyMacroDryWetFromMIDI applies F0 F0 03 xx semantics: xx 0 = 100% dry, 7F = 100% wet (OpenMPT).
func itApplyMacroDryWetFromMIDI(ch *itChannel, wetMIDI int) {
	wetMIDI = clampInt(wetMIDI, 0, 127)
	ch.macroDry = float64(127-wetMIDI) / 127.0
	ch.macroWet = float64(wetMIDI) / 127.0
	if ch.macroDry < 0 {
		ch.macroDry = 0
	}
	if ch.macroDry > 1 {
		ch.macroDry = 1
	}
	if ch.macroWet < 0 {
		ch.macroWet = 0
	}
	if ch.macroWet > 1 {
		ch.macroWet = 1
	}
}

// itApplyPluginParamByIndex handles F0FnnnZ “plugin parameter” macros without a plugin host.
// Indices 0–3 map to filter + dry/wet; 4–7 map to vibrato/tremolo; other indices are stored in macroPlugParam.
func itApplyPluginParamByIndex(ch *itChannel, idx int, val uint8) {
	v := int(val)
	ch.macroPlugParam[idx&31] = val
	switch idx {
	case 0:
		ch.fltCut = clampInt(v, 0, 127)
		ch.itFilterMarkDirty()
		ch.itFilterResetState()
	case 1:
		ch.fltRes = clampInt(v, 0, 127)
		ch.itFilterMarkDirty()
		ch.itFilterResetState()
	case 2:
		itSetFilterModeFromByte(ch, val)
	case 3:
		itApplyMacroDryWetFromMIDI(ch, v)
	case 4:
		ch.vibSpd = uint8(clampInt(v/2, 0, 64))
	case 5:
		ch.vibDep = uint8(clampInt(v/2, 0, 64))
	case 6:
		ch.tremSpd = uint8(clampInt(v/2, 0, 64))
	case 7:
		ch.tremDep = uint8(clampInt(v/2, 0, 64))
	default:
	}
}

func itApplyPitchBendMacro(ch *itChannel, prm uint8) {
	// ±2 semitones at Z extremes (coarse IT-style bend from macro z).
	ch.macroPitchSemis = (float64(prm) - 64.0) / 64.0 * 2.0
}

func itApplyPolyAftertouchMacro(ch *itChannel, macro [itMidiMacroBytes]byte, prm uint8) {
	s := itMacroNorm(macro)
	if len(s) < 6 || !strings.HasPrefix(s, "ACN") || s[5] != 'Z' {
		return
	}
	note, ok := itParseHexByte(s[3], s[4])
	if !ok {
		return
	}
	itApplyPolyAftertouchAtNote(ch, note, int(prm))
}

func itApplyPolyAftertouchAtNote(ch *itChannel, note int, pressure int) {
	if note < 0 || note > 119 || ch.note != note {
		return
	}
	itApplyChannelAftertouch(ch, pressure)
}

func itMacroParseBcCC(macro [itMidiMacroBytes]byte) (cc int, ok bool) {
	s := itMacroNorm(macro)
	if len(s) < 5 || s[0] != 'B' || s[1] != 'C' {
		return 0, false
	}
	if s[len(s)-1] != 'Z' {
		return 0, false
	}
	v, ok2 := itParseHexByte(s[2], s[3])
	return v, ok2
}

func itApplyMIDICC(p *Player, ch *itChannel, cc, val int) {
	val = clampInt(val, 0, 127)
	switch cc {
	case 0: // bank select MSB
		ch.macroBankMSB = uint8(val)
	case 1: // mod wheel → vibrato depth (coarse mapping)
		ch.vibDep = uint8(clampInt(val/4, 0, 64))
	case 7: // channel volume
		ch.volume = clampInt(val*64/127, 0, 64)
		ch.mixVol = ch.volume
	case 10: // pan
		ch.panBase = clampInt(val*255/127, 0, 255)
	case 32: // bank select LSB
		ch.macroBankLSB = uint8(val)
	case 71, 74, 91, 93: // resonance / brightness / reverb / chorus — map common filter CCs to cutoff
		ch.fltCut = clampInt(val, 0, 127)
		ch.itFilterMarkDirty()
		ch.itFilterResetState()
	default:
		if p != nil {
			p.itMacroExternalCC(ch, cc, val)
		}
	}
}

func itApplyChannelAftertouch(ch *itChannel, pressure int) {
	pressure = clampInt(pressure, 0, 127)
	if pressure <= 0 {
		return
	}
	ch.mixVol = clampInt(ch.volume*pressure/127, 0, 64)
}

// itApplyFixedZMacro runs one Z80–ZFF fixed macro string (literals and/or chained internal messages).
func itApplyFixedZMacro(p *Player, ch *itChannel, macro [itMidiMacroBytes]byte) {
	itApplyInternalMacroScan(p, ch, macro, 0)
}

// itDefaultFixedMacroReso4 applies built-in Z80–Z8F resonance mapping when fixed macros are empty / unknown.
func itDefaultFixedMacroReso4(ch *itChannel, prm uint8) {
	k := int(prm - 0x80)
	if k < 0 || k > 15 {
		return
	}
	ch.fltRes = k * 127 / 15
	if ch.fltRes > 127 {
		ch.fltRes = 127
	}
	ch.itFilterMarkDirty()
	ch.itFilterResetState()
}

func itFixedMacroIsEmpty(cfg *ITMidiMacroConfig, slot int) bool {
	if cfg == nil || !cfg.Valid || slot < 0 || slot >= itMidiZxxCount {
		return true
	}
	for _, b := range cfg.Zxx[slot] {
		if b != 0 && b != ' ' {
			return false
		}
	}
	return true
}
