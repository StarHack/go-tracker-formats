package ftm

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestValidateRejectBadHeader(t *testing.T) {
	err := Validate([]byte("not a module"))
	if err == nil || !strings.Contains(err.Error(), "FamiTracker") {
		t.Fatalf("expected FamiTracker header error, got %v", err)
	}
}

func TestLoadLegacyASCIIEndFTM(t *testing.T) {
	path := filepath.Join("..", "..", "sample-data", "thegreatmightypoofds.ftm")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Skip("sample file not present:", err)
	}
	if _, err := LoadModule(data); err != nil {
		t.Fatalf("LoadModule: %v", err)
	}
}

func TestParseFileHeader(t *testing.T) {
	data := append([]byte(headerFT), 0x00, 0x02, 0x00, 0x00) // version 0x0200 LE
	dn, fv, off, err := parseFileHeader(data)
	if err != nil {
		t.Fatal(err)
	}
	if dn {
		t.Fatal("expected classic header")
	}
	if fv != 0x0200 {
		t.Fatalf("file version 0x%x", fv)
	}
	if off != len(headerFT)+4 {
		t.Fatalf("body offset %d", off)
	}
}
