package ftm

import (
	"fmt"

	"github.com/StarHack/go-tracker-formats/formats"
)

// Player implements formats.PCMTracker using a real Ricoh 2A03 APU (see nes2a03) driven from
// decoded pattern rows. Expansion chips (VRC6, FDS, N163, …) are not emulated; only the first
// five logical channels are routed to pulse1, pulse2, triangle, and noise.
type Player struct {
	desc                  []byte
	loadErr               bool
	chip                  *chipPlayer
	stereoSamplesOnePass0 int // full track 0 duration (stereo pairs); 0 if unknown
}

var _ formats.PCMTracker = (*Player)(nil)

// Init decodes the tune and prepares chip playback.
func (p *Player) Init(tune []byte, sampleRate int) string {
	if p == nil {
		return "nil player"
	}
	p.loadErr = false
	p.chip = nil
	p.stereoSamplesOnePass0 = 0
	if sampleRate <= 0 {
		sampleRate = 44100
	}
	m, err := LoadModule(tune)
	if err != nil {
		p.loadErr = true
		p.desc = []byte(fmt.Sprintf("FTM load error: %v", err))
		return string(p.desc)
	}
	title := ""
	if m.Info != nil {
		title = m.Info.Name
	}
	chip, err := newChipPlayer(m, sampleRate)
	if err != nil {
		p.loadErr = true
		p.desc = []byte(fmt.Sprintf("FTM chip init: %v", err))
		return string(p.desc)
	}
	p.chip = chip
	p.stereoSamplesOnePass0 = StereoSamplesOnePassTrack0(m, sampleRate)
	var warn string
	if m.Params != nil && m.Params.ExpansionChip != 0 {
		warn = " (expansion audio not emulated)"
	}
	p.desc = []byte(fmt.Sprintf("FTM (2A03) —%s %s", warn, title))
	return string(p.desc)
}

// Sample writes one stereo sample pair. Returns false for continuous play (looping song).
func (p *Player) Sample(left, right *int16) bool {
	if p == nil {
		return true
	}
	if p.loadErr || p.chip == nil {
		*left, *right = 0, 0
		return true
	}
	s := p.chip.nextSample()
	*left, *right = s, s
	return false
}

// FullPassStereoSamplePairs returns stereo sample pairs for one full track 0 traversal
// (0 if Init failed or layout is unknown). See StereoSamplesOnePassTrack0.
func (p *Player) FullPassStereoSamplePairs() int {
	if p == nil {
		return 0
	}
	return p.stereoSamplesOnePass0
}

// Stop resets the player.
func (p *Player) Stop() {
	if p != nil {
		p.loadErr = false
		p.desc = nil
		p.chip = nil
		p.stereoSamplesOnePass0 = 0
	}
}

// GetDescription returns the last init status or error text.
func (p *Player) GetDescription() []byte {
	if p == nil || p.desc == nil {
		return nil
	}
	out := make([]byte, len(p.desc))
	copy(out, p.desc)
	return out
}
