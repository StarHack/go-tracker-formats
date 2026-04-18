// Package nes2a03 is a stand-alone Ricoh 2A03 (NES APU) emulator core used for FamiTracker playback.
//
// The pulse, triangle, noise, and DMC channel logic and non-linear mixer are derived from the
// MIT-licensed NES emulator by Michael Fogleman (github.com/fogleman/nes, nes/apu.go), adapted
// to remove console/CPU coupling: DPCM memory reads use a callback, and the optional high-pass
// chain from the original is omitted for simplicity (raw DAC mix).
package nes2a03
