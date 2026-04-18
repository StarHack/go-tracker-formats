package ftm

// File header strings (DocumentFile.cpp).
const (
	headerFT = "FamiTracker Module"
	headerDn = "Dn-FamiTracker Module"
)

// File and block limits (aligned with Dn-FamiTracker CDocumentFile / pattern data).
const (
	blockPayloadMax = 50_000_000

	fileVerMin = 0x0200 // block-based modules (OpenDocumentNew)
	fileVerMax = 0x0450 // Dn-FamiTracker FILE_VER / COMPATIBLE_FORWARD_VER

	maxChannels      = 128
	maxTracks        = 64
	maxPattern       = 256
	maxPatternRows   = 256
	maxFrames        = 256
	maxTempo         = 255 // Dn MAX_TEMPO
	maxInstruments   = 64  // Dn MAX_INSTRUMENTS
	maxEffectColumns = 4   // Dn MAX_EFFECT_COLUMNS (pattern cell width)
	maxDSamples      = 64  // Dn MAX_DSAMPLES
	seqCount         = 5   // volume, arp, pitch, hi-pitch, duty/noise (per chip)
	maxSequences     = 128
	maxSequenceItems = 252 // Dn MAX_SEQUENCE_ITEMS
	noteCount        = 96  // MIDI note span used by 2A03 DPCM map
	octaveRange      = 8
	noteRange        = 12
	holdInstrument   = 0xff // Dn HOLD_INSTRUMENT
	noteNone         = 0
	noteEcho         = 15 // note_t::ECHO (NONE..ECHO inclusive)
	defaultTempoNTSC = 150
	defaultTempoPAL  = 125
	defaultSpeed     = 6
	maxGroove        = 32
	maxGrooveSize    = 128
	instNameMax      = 128
	dsampleNameMax   = 256
	maxPatternVolume = 0x10 // Dn MAX_VOLUME
	maxEffectID      = 250  // upper bound for effect_t values in files
	fdsWaveSize      = 64
	fdsModSize       = 32
	fdsSequenceCount = 3
	n163MaxWaveSize  = 240
	n163MaxWaveCount = 64
)

// Block IDs (16-byte, NUL-padded) written by Dn-FamiTracker.
const (
	blockParams      = "PARAMS"
	blockTuning      = "TUNING"
	blockInfo        = "INFO"
	blockInstruments = "INSTRUMENTS"
	blockSequences   = "SEQUENCES"
	blockFrames      = "FRAMES"
	blockPatterns    = "PATTERNS"
	blockDSamples    = "DPCM SAMPLES"
	blockHeader      = "HEADER"
	blockComments    = "COMMENTS"
	blockSeqVRC6     = "SEQUENCES_VRC6"
	blockSeqN163     = "SEQUENCES_N163"
	blockSeqN106     = "SEQUENCES_N106"
	blockSeqS5B      = "SEQUENCES_S5B"
	blockDetune      = "DETUNETABLES"
	blockGrooves     = "GROOVES"
	blockBookmarks   = "BOOKMARKS"
	blockParamsExtra = "PARAMS_EXTRA"
	blockJSON        = "JSON"
	blockParamsEmu   = "PARAMS_EMU"
	blockEnd         = "END"
)

// inst_type_t (Instrument.h)
const (
	InstNone = 0
	Inst2A03 = 1
	InstVRC6 = 2
	InstVRC7 = 3
	InstFDS  = 4
	InstN163 = 5
	InstS5B  = 6
)
