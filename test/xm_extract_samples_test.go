package test

import (
	"flag"
	"testing"
)

var (
	xmExtractSamples = flag.String("xm_extract_samples", "", "Comma-separated selectors: \"Instrument 3/drum - orchestra\"")
	xmExtractOutDir  = flag.String("xm_extract_out_dir", "", "Output directory for extracted WAV files (default: ../sample-data/extracted)")
)

// TestXM_ExtractSamples extracts selected XM samples to WAV files.
//
// Example:
// go test -run TestXM_ExtractSamples -v ./test -args \
//   -xm_extract_samples "Instrument 3/drum - orchestra,Instrument 41/drum - core" \
//   -xm_extract_out_dir "../sample-data/extracted"
func TestXM_ExtractSamples(t *testing.T) {
	if *xmExtractSamples == "" {
		t.Skip("set -args -xm_extract_samples \"Instrument 3/drum - orchestra\" to run extraction")
	}
	extractXMSamplesFromSelectors(t, *xmExtractSamples, *xmExtractOutDir)
}
