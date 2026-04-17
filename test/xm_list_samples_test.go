package test

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/StarHack/go-tracker-formats/formats/xm"
)

// TestXM_ListSamples prints all instruments and samples in a readable tree.
//
// Run with:
//   go test -run TestXM_ListSamples -v ./test
func TestXM_ListSamples(t *testing.T) {
	root := filepath.Clean(filepath.Join(".."))
	xmPath := filepath.Join(root, "sample-data", "DT_DD.XM")

	moduleData, err := os.ReadFile(xmPath)
	if err != nil {
		t.Fatalf("read xm fixture: %v", err)
	}

	player := &xm.Player{}
	if err := player.Init(moduleData, 44100); err != "" {
		t.Fatalf("init xm player: %s", err)
	}

	pv := reflect.ValueOf(player).Elem()
	instruments := pv.FieldByName("instruments")
	sampleRate := int(pv.FieldByName("sampleRate").Int())
	if sampleRate <= 0 {
		sampleRate = 44100
	}

	for i := 0; i < instruments.Len(); i++ {
		inst := instruments.Index(i)
		instName := strings.TrimSpace(inst.FieldByName("name").String())
		if instName == "" {
			instName = fmt.Sprintf("Instrument %d", i+1)
		}
		t.Logf("- %s", instName)

		samples := inst.FieldByName("samples")
		if samples.Len() == 0 {
			t.Logf("  - (no samples)")
			continue
		}

		for si := 0; si < samples.Len(); si++ {
			smp := samples.Index(si)
			smpName := strings.TrimSpace(smp.FieldByName("name").String())
			if smpName == "" {
				smpName = fmt.Sprintf("Sample %d", si+1)
			}

			length := int(smp.FieldByName("length").Int())
			bits := 8
			if smp.FieldByName("is16").Bool() {
				bits = 16
			}
			if length <= 0 {
				t.Logf("  - %s (%d Hz, %d-bit, empty)", smpName, sampleRate, bits)
				continue
			}

			durationSec := float64(length) / float64(sampleRate)
			// Show short samples with enough precision so they are not collapsed to 0.00s.
			t.Logf("  - %s (%d Hz, %d-bit, %d frames, %.5fs)", smpName, sampleRate, bits, length, durationSec)
		}
	}
}
