// tracker.go - Common Tracker interfaces for all supported formats.
package formats

// OPLFunc is called for every OPL3 register write.
type OPLFunc = func(reg uint16, val uint8)

// Tracker is implemented by OPL-synthesis-based formats (RAD v1, RAD v2).
type Tracker interface {
	// Init prepares the tune for playback using the given OPL callback.
	// Returns an empty string on success, or an error message on failure.
	Init(tune []byte, opl OPLFunc) string

	// Update advances playback by one tick.
	// Returns true when the tune has started repeating.
	Update() bool

	// Stop halts all sound and resets the player to the beginning.
	Stop()

	// GetHertz returns the required Update() call frequency in Hz.
	GetHertz() int

	// GetDescription returns the raw embedded description bytes from the tune.
	GetDescription() []byte
}

// PCMTracker is implemented by sample-based formats (MOD, S3M, XM…).
// It generates PCM audio directly without OPL synthesis.
// Sample() manages its own internal tick timing at the given sample rate.
type PCMTracker interface {
	// Init prepares the tune for playback at the given sample rate.
	// Returns an empty string on success, or an error message on failure.
	Init(tune []byte, sampleRate int) string

	// Sample generates one stereo output sample pair.
	// It internally calls the tick function at the correct intervals.
	// Returns true when the tune has started repeating.
	Sample(left, right *int16) bool

	// Stop halts all sound and resets the player.
	Stop()

	// GetDescription returns a human-readable description/title string.
	GetDescription() []byte
}
