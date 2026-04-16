// tracker.go - Common Tracker interface for RAD tune players.
package formats

// OPLFunc is called for every OPL3 register write.
type OPLFunc = func(reg uint16, val uint8)

// Tracker is the common interface implemented by every RAD format player.
// Each method mirrors the original RAD player API.
type Tracker interface {
	// Init prepares the tune for playback using the given OPL callback.
	// Returns an empty string on success, or an error message on failure.
	Init(tune []byte, opl OPLFunc) string

	// Update advances playback by one tick.
	// Returns true when the tune has started repeating (i.e. playback is done).
	Update() bool

	// Stop halts all sound and resets the player to the beginning of the tune.
	Stop()

	// GetHertz returns the required Update() call frequency in Hz.
	// Returns a negative value if Init() has not been called or failed.
	GetHertz() int

	// GetDescription returns the raw embedded description bytes from the tune.
	GetDescription() []byte
}
