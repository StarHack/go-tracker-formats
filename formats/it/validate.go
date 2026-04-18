package it

import "fmt"

func Validate(data []byte) error {
	_, err := loadModule(data)
	if err != nil {
		return fmt.Errorf("IT: %w", err)
	}
	return nil
}
