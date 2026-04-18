package formats

// OPLFunc is called for every OPL3 register write.
type OPLFunc = func(reg uint16, val uint8)

// Tracker is implemented by OPL-synthesis-based formats (RAD v1, RAD v2).
type Tracker interface {
	Init(tune []byte, opl OPLFunc) string

	Update() bool

	Stop()

	GetHertz() int

	GetDescription() []byte
}

// PCMTracker is implemented by sample-based formats (MOD, S3M, XM, IT…).
// It generates PCM audio directly without OPL synthesis.
// Sample() manages its own internal tick timing at the given sample rate.
type PCMTracker interface {
	Init(tune []byte, sampleRate int) string

	Sample(left, right *int16) bool

	Stop()

	GetDescription() []byte
}
