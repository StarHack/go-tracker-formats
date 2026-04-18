package ftm

// Validate checks that data is a well-formed FamiTracker module (same checks as LoadModule).
func Validate(data []byte) error {
	_, err := LoadModule(data)
	return err
}
