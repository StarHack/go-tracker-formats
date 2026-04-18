package ftm

// Chunk is one on-disk FTM block (16-byte ID, version, payload).
type Chunk struct {
	ID      string // up to 16 bytes, ASCII, trimmed of NULs
	Version uint32
	Data    []byte
}

// Module is a decoded FamiTracker module (file version 0x0200+).
type Module struct {
	DnModule    bool
	FileVersion uint32

	Params      *ParamsBlock
	Tuning      *TuningBlock
	Info        *InfoBlock
	Header      *HeaderBlock
	Frames      *FramesBlock
	Patterns    *PatternsBlock
	DSamples    *DSamplesBlock
	Comments    *CommentsBlock
	Sequences   *Sequences2A03Block
	SeqVRC6     *SequencesChipBlock
	SeqN163     *SequencesChipBlock
	SeqS5B      *SequencesChipBlock
	Instruments *InstrumentsBlock

	DetuneTables *DetuneTablesBlock
	Grooves      *GroovesBlock
	Bookmarks    *BookmarksBlock
	ParamsExtra  *ParamsExtraBlock
	JSONBlock    []byte // raw JSON payload
	ParamsEmu    []byte // raw PARAMS_EMU payload

	// Chunks with unknown or reserved IDs (excluding END).
	Unknown []Chunk
}

type ParamsBlock struct {
	Version           int
	ExpansionChip     int
	Channels          int
	Machine           int // 0 NTSC, 1 PAL
	EngineSpeed       int
	PlaybackRateType  int // v>=7
	PlaybackRateUS    int // v>=7, microseconds
	VibratoStyle      int
	N163ChannelCount  int // when N163 enabled
	SpeedSplitPoint   int
	DetuneSemitone    int8 // v==8
	DetuneCent        int8 // v==8
	SongSpeedLegacyV1 int  // PARAMS version 1 only
	// PARAMS block versions 4–6 stored global row highlight (superseded by HEADER v4+).
	HighlightFirst  int
	HighlightSecond int
}

type TuningBlock struct {
	Version  int
	Semitone int8
	Cent     int8
}

type InfoBlock struct {
	Name      string
	Artist    string
	Copyright string
}

type HeaderBlock struct {
	Version         int
	TrackCount      int
	TrackTitles     []string  // v>=3
	ChannelTypes    []byte    // len == Channels from PARAMS
	EffectColumns   [][]uint8 // [channel][track] count per (ch, track)
	HighlightFirst  int       // v>=4, track0
	HighlightSecond int
}

type FramesBlock struct {
	Tracks []TrackFrames
}

type TrackFrames struct {
	FrameCount    int
	Speed         int
	Tempo         int // v>=3 of FRAMES block
	PatternLength int
	Patterns      [][]byte // [frame][channel] pattern index
}

type PatternsBlock struct {
	Version             int
	LegacyPatternLength int           // PATTERNS block version 1 only: default row count for track 0
	Rows                []PatternCell // sparse entries in file order
}

type PatternCell struct {
	Track    int
	Channel  int
	Pattern  int
	Row      int
	Note     byte
	Octave   byte
	Inst     byte
	Vol      byte
	EffNum   []byte
	EffParam []byte
}

type DSample struct {
	Index int
	Name  string
	Data  []byte
}

type DSamplesBlock struct {
	Samples []DSample
}

type CommentsBlock struct {
	Display bool
	Text    string
}

type Sequence struct {
	Chip         string // "2A03", "VRC6", "N163", "S5B"
	Index        int
	Type         int
	Items        []int8
	LoopPoint    int32
	ReleasePoint int32
	Setting      int32
}

type Sequences2A03Block struct {
	Version   int
	Sequences []Sequence
}

type SequencesChipBlock struct {
	Version   int
	Chip      string
	Sequences []Sequence
}

type SeqInstrumentData struct {
	Enabled [seqCount]bool
	Index   [seqCount]byte
}

type Instrument2A03Data struct {
	BlockVersion int
	Assignments  []DPCMAssignment // sparse when block v>=7
	// For block version <7, grid [octave][note] — flattened in AssignmentsLegacy optional
	GridSample [octaveRange][noteRange]byte
	GridPitch  [octaveRange][noteRange]byte
	GridDelta  [octaveRange][noteRange]int8
}

type DPCMAssignment struct {
	Octave, Note int
	Sample       byte
	Pitch        byte
	Delta        int8
}

type InstrumentFDSData struct {
	BlockVersion int
	Wave         [fdsWaveSize]byte
	Modulation   [fdsModSize]byte
	ModSpeed     int32
	ModDepth     int32
	ModDelay     int32
	Sequences    [fdsSequenceCount]FDSInlineSequence
}

type FDSInlineSequence struct {
	Count        int
	LoopPoint    int32
	ReleasePoint int32
	Setting      int32
	Values       []int8
}

type InstrumentN163Data struct {
	BlockVersion int
	WaveSize     int
	WavePos      int
	AutoWavePos  bool // v>=8
	WaveCount    int
	Samples      [][]byte // [wave][pos] nibble stored as byte 0-15
}

type InstrumentVRC7Data struct {
	Patch int32
	Regs  [8]byte
}

type Instrument struct {
	Index  int
	Type   int
	Name   string
	Seq    *SeqInstrumentData // 2A03, VRC6, FDS, N163, S5B (seq-based)
	TwoA03 *Instrument2A03Data
	FDS    *InstrumentFDSData
	N163   *InstrumentN163Data
	VRC7   *InstrumentVRC7Data
}

type InstrumentsBlock struct {
	Version     int
	Instruments []Instrument
}

type DetuneTablesBlock struct {
	Version int
	Tables  []DetuneTable
}

type DetuneTable struct {
	Chip  int
	Notes []DetuneNote
}

type DetuneNote struct {
	NoteIndex int
	Offset    int32
}

type GroovesBlock struct {
	Version    int
	Grooves    []Groove
	TrackFlags []byte // USEGROOVE: 0 = default speed, 1 = groove per track
}

type Groove struct {
	Index   int
	Entries []byte
}

type BookmarksBlock struct {
	Entries []Bookmark
}

type Bookmark struct {
	Track      int
	Frame      int
	Row        int
	Highlight1 int
	Highlight2 int
	Following  bool
	Name       string
}

type ParamsExtraBlock struct {
	Version     int
	LinearPitch bool
	Semitone    int8
	Cent        int8
}
