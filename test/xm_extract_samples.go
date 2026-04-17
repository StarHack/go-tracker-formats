package test

import (
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/StarHack/go-tracker-formats/formats/xm"
)

// extractXMSamplesFromSelectors extracts one or more XM samples to WAV files.
//
// Selector format:
//   Instrument 3/drum - orchestra
// Multiple selectors are passed comma-separated.
func extractXMSamplesFromSelectors(t *testing.T, selectorsCSV, outDir string) {
	t.Helper()
	if strings.TrimSpace(selectorsCSV) == "" {
		t.Fatalf("no selectors provided; set -args -xm_extract_samples \"Instrument 3/drum - orchestra\"")
	}

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

	if outDir == "" {
		outDir = filepath.Join(root, "sample-data", "extracted")
	}
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		t.Fatalf("create output dir: %v", err)
	}

	selectors := splitCSV(selectorsCSV)
	if len(selectors) == 0 {
		t.Fatalf("no valid selectors in input: %q", selectorsCSV)
	}

	for _, sel := range selectors {
		instIdx, sampleName, err := parseSelector(sel)
		if err != nil {
			t.Fatalf("invalid selector %q: %v", sel, err)
		}
		if instIdx < 1 || instIdx > instruments.Len() {
			t.Fatalf("selector %q: instrument index out of range (1..%d)", sel, instruments.Len())
		}

		inst := instruments.Index(instIdx - 1)
		instName := strings.TrimSpace(inst.FieldByName("name").String())
		if instName == "" {
			instName = fmt.Sprintf("Instrument %d", instIdx)
		}
		samples := inst.FieldByName("samples")
		if samples.Len() == 0 {
			t.Fatalf("selector %q: instrument %d has no samples", sel, instIdx)
		}

		matched := false
		for si := 0; si < samples.Len(); si++ {
			smp := samples.Index(si)
			curName := strings.TrimSpace(smp.FieldByName("name").String())
			if curName == "" {
				curName = fmt.Sprintf("Sample %d", si+1)
			}
			if !strings.EqualFold(strings.TrimSpace(sampleName), curName) {
				continue
			}

			dataVal := smp.FieldByName("data")
			if dataVal.Len() == 0 {
				t.Logf("skip empty sample: %s/%s", instName, curName)
				matched = true
				continue
			}

			is16 := smp.FieldByName("is16").Bool()
			frames := make([]int16, dataVal.Len())
			for i := 0; i < dataVal.Len(); i++ {
				frames[i] = int16(dataVal.Index(i).Int())
			}
			if !is16 {
				for i := range frames {
					frames[i] >>= 8
				}
			}

			fileName := fmt.Sprintf("inst_%02d_%s__%s.wav",
				instIdx, sanitizeFileName(instName), sanitizeFileName(curName))
			outPath := filepath.Join(outDir, fileName)
			if err := writePCM16MonoWAV(outPath, frames, 44100); err != nil {
				t.Fatalf("write wav for %q: %v", sel, err)
			}
			t.Logf("saved: %s", outPath)
			matched = true
		}

		if !matched {
			t.Fatalf("selector %q: sample not found in instrument %d", sel, instIdx)
		}
	}
}

func parseSelector(sel string) (int, string, error) {
	parts := strings.SplitN(sel, "/", 2)
	if len(parts) != 2 {
		return 0, "", fmt.Errorf("expected \"Instrument <n>/<sample name>\"")
	}
	left := strings.TrimSpace(parts[0])
	right := strings.TrimSpace(parts[1])
	if right == "" {
		return 0, "", fmt.Errorf("missing sample name")
	}

	re := regexp.MustCompile(`(?i)^instrument\s+(\d+)$`)
	m := re.FindStringSubmatch(left)
	if len(m) != 2 {
		return 0, "", fmt.Errorf("left side must be \"Instrument <n>\"")
	}
	n, err := strconv.Atoi(m[1])
	if err != nil {
		return 0, "", fmt.Errorf("invalid instrument index: %v", err)
	}
	return n, right, nil
}

func splitCSV(v string) []string {
	raw := strings.Split(v, ",")
	out := make([]string, 0, len(raw))
	for _, s := range raw {
		s = strings.TrimSpace(s)
		if s != "" {
			out = append(out, s)
		}
	}
	return out
}

func sanitizeFileName(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return "unnamed"
	}
	re := regexp.MustCompile(`[^a-zA-Z0-9._-]+`)
	s = re.ReplaceAllString(s, "_")
	s = strings.Trim(s, "_")
	if s == "" {
		return "unnamed"
	}
	return s
}

func writePCM16MonoWAV(path string, frames []int16, sampleRate int) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	const numChannels uint16 = 1
	const bitsPerSample uint16 = 16
	byteRate := uint32(sampleRate) * uint32(numChannels) * uint32(bitsPerSample/8)
	blockAlign := numChannels * bitsPerSample / 8
	dataSize := uint32(len(frames) * 2)
	riffSize := uint32(36) + dataSize

	if _, err := f.Write([]byte("RIFF")); err != nil {
		return err
	}
	if err := binary.Write(f, binary.LittleEndian, riffSize); err != nil {
		return err
	}
	if _, err := f.Write([]byte("WAVE")); err != nil {
		return err
	}

	if _, err := f.Write([]byte("fmt ")); err != nil {
		return err
	}
	if err := binary.Write(f, binary.LittleEndian, uint32(16)); err != nil {
		return err
	}
	if err := binary.Write(f, binary.LittleEndian, uint16(1)); err != nil { // PCM
		return err
	}
	if err := binary.Write(f, binary.LittleEndian, numChannels); err != nil {
		return err
	}
	if err := binary.Write(f, binary.LittleEndian, uint32(sampleRate)); err != nil {
		return err
	}
	if err := binary.Write(f, binary.LittleEndian, byteRate); err != nil {
		return err
	}
	if err := binary.Write(f, binary.LittleEndian, blockAlign); err != nil {
		return err
	}
	if err := binary.Write(f, binary.LittleEndian, bitsPerSample); err != nil {
		return err
	}

	if _, err := f.Write([]byte("data")); err != nil {
		return err
	}
	if err := binary.Write(f, binary.LittleEndian, dataSize); err != nil {
		return err
	}
	for _, v := range frames {
		if err := binary.Write(f, binary.LittleEndian, v); err != nil {
			return err
		}
	}
	return nil
}
