package main

import (
	"bytes"
	"encoding/binary"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/StarHack/go-tracker-formats/formats"
	"github.com/StarHack/go-tracker-formats/formats/it"
	"github.com/StarHack/go-tracker-formats/formats/mod"
	radv1 "github.com/StarHack/go-tracker-formats/formats/rad-v1"
	radv2 "github.com/StarHack/go-tracker-formats/formats/rad-v2"
	"github.com/StarHack/go-tracker-formats/formats/s3m"
	"github.com/StarHack/go-tracker-formats/formats/xm"
	"github.com/StarHack/go-tracker-formats/opal"
)

const defaultSampleRate = 44100

func main() {
	outFlag := flag.String("o", "", "output file (.wav or .mp3; default: input basename + .wav)")
	rateFlag := flag.Int("rate", defaultSampleRate, "output sample rate in Hz")
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: module-to-stream [options] input.{rad,mod,s3m,xm,it}\n\nOptions:\n")
		flag.PrintDefaults()
	}
	flag.Parse()

	args := flag.Args()
	if len(args) == 0 {
		flag.Usage()
		os.Exit(1)
	}
	inputFile := args[0]

	outFile := *outFlag
	if outFile == "" {
		base := filepath.Base(inputFile)
		ext := filepath.Ext(base)
		outFile = strings.TrimSuffix(base, ext) + ".wav"
	}

	sampleRate := *rateFlag
	if sampleRate <= 0 || sampleRate > 192000 {
		fmt.Fprintf(os.Stderr, "Error: invalid sample rate %d\n", sampleRate)
		os.Exit(1)
	}

	tune, err := os.ReadFile(inputFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading %s: %v\n", inputFile, err)
		os.Exit(1)
	}

	tracker, errMsg := detect(tune)
	if errMsg != "" {
		fmt.Fprintf(os.Stderr, "Unsupported or invalid file: %s\n", errMsg)
		os.Exit(1)
	}

	printInfo(tracker)

	fmt.Printf("Rendering to %s at %d Hz...\n", outFile, sampleRate)
	samples := renderToSamples(tune, sampleRate, tracker)
	fmt.Printf("Done: %d samples (%.2f seconds)\n", len(samples)/2, float64(len(samples)/2)/float64(sampleRate))

	if err := writeOutput(outFile, samples, sampleRate); err != nil {
		fmt.Fprintf(os.Stderr, "Error writing output: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Written: %s\n", outFile)
}

func detect(tune []byte) (interface{}, string) {
	if len(tune) >= 17 {
		hdr := "RAD by REALiTY!!"
		match := true
		for i := 0; i < 16; i++ {
			if tune[i] != hdr[i] {
				match = false
				break
			}
		}
		if match {
			switch tune[0x10] {
			case 0x10:
				if e := radv1.Validate(tune); e != nil {
					return nil, e.Error()
				}
				return &radv1.Player{}, ""
			case 0x21:
				if e := radv2.Validate(tune); e != nil {
					return nil, e.Error()
				}
				return &radv2.Player{}, ""
			default:
				return nil, "Not a recognised RAD version."
			}
		}
	}
	if e := it.Validate(tune); e == nil {
		return &it.Player{}, ""
	}
	if e := xm.Validate(tune); e == nil {
		return &xm.Player{}, ""
	}
	if e := s3m.Validate(tune); e == nil {
		return &s3m.Player{}, ""
	}
	if e := mod.Validate(tune); e == nil {
		return &mod.Player{}, ""
	}
	return nil, "Unrecognised file format (not RAD v1/v2, IT, XM, S3M, or MOD)."
}

func renderToSamples(tune []byte, sampleRate int, tracker interface{}) []int16 {
	switch t := tracker.(type) {
	case formats.Tracker:
		return renderOPL(tune, sampleRate, t)
	case formats.PCMTracker:
		return renderPCM(tune, sampleRate, t)
	}
	return nil
}

func renderOPL(tune []byte, sampleRate int, player formats.Tracker) []int16 {
	adlib := opal.New(sampleRate)
	if e := player.Init(tune, adlib.Port); e != "" {
		fmt.Fprintf(os.Stderr, "Error: player init failed: %s\n", e)
		os.Exit(1)
	}
	hertz := player.GetHertz()
	if hertz <= 0 {
		fmt.Fprintf(os.Stderr, "Error: invalid player hertz value\n")
		os.Exit(1)
	}
	samPerTick := sampleRate / hertz
	samCnt := 0
	var out []int16
	for {
		var l, r int16
		adlib.Sample(&l, &r)
		out = append(out, l, r)
		samCnt++
		if samCnt >= samPerTick {
			samCnt = 0
			if player.Update() {
				break
			}
		}
	}
	return out
}

func renderPCM(tune []byte, sampleRate int, player formats.PCMTracker) []int16 {
	if e := player.Init(tune, sampleRate); e != "" {
		fmt.Fprintf(os.Stderr, "Error: player init failed: %s\n", e)
		os.Exit(1)
	}
	var out []int16
	for {
		var l, r int16
		if player.Sample(&l, &r) {
			break
		}
		out = append(out, l, r)
	}
	return out
}

func writeOutput(path string, samples []int16, sampleRate int) error {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".wav":
		return writeWAV(path, samples, sampleRate)
	case ".mp3":
		return writeMP3(path, samples, sampleRate)
	default:
		return fmt.Errorf("unsupported output extension %q (use .wav or .mp3)", filepath.Ext(path))
	}
}

func writeWAV(path string, samples []int16, sampleRate int) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	var (
		numChannels   = uint16(2)
		bitsPerSample = uint16(16)
		byteRate      = uint32(sampleRate) * uint32(numChannels) * uint32(bitsPerSample/8)
		blockAlign    = numChannels * bitsPerSample / 8
		dataSize      = uint32(len(samples)) * uint32(bitsPerSample/8)
		riffSize      = 36 + dataSize
	)
	le := binary.LittleEndian
	w := func(v interface{}) { _ = binary.Write(f, le, v) }
	_, _ = f.WriteString("RIFF")
	w(riffSize)
	_, _ = f.WriteString("WAVE")
	_, _ = f.WriteString("fmt ")
	w(uint32(16))
	w(uint16(1))
	w(numChannels)
	w(uint32(sampleRate))
	w(byteRate)
	w(blockAlign)
	w(bitsPerSample)
	_, _ = f.WriteString("data")
	w(dataSize)
	w(samples)
	return nil
}

func writeMP3(path string, samples []int16, sampleRate int) error {
	cmd := exec.Command(
		"ffmpeg",
		"-y",
		"-f", "s16le",
		"-ar", fmt.Sprintf("%d", sampleRate),
		"-ac", "2",
		"-i", "-",
		"-vn",
		"-codec:a", "libmp3lame",
		path,
	)
	var pcm bytes.Buffer
	if err := binary.Write(&pcm, binary.LittleEndian, samples); err != nil {
		return err
	}
	cmd.Stdin = &pcm
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("ffmpeg mp3 encode failed (is ffmpeg installed?): %w", err)
	}
	return nil
}

func printInfo(tracker interface{}) {
	var desc []byte
	var label string
	switch t := tracker.(type) {
	case *radv1.Player:
		label = "RAD v1"
		desc = t.GetDescription()
	case *radv2.Player:
		label = "RAD v2.1"
		desc = t.GetDescription()
	case *mod.Player:
		label = "MOD"
		desc = t.GetDescription()
	case *s3m.Player:
		label = "S3M"
		desc = t.GetDescription()
	case *xm.Player:
		label = "XM"
		desc = t.GetDescription()
	case *it.Player:
		label = "IT (Impulse Tracker)"
		desc = t.GetDescription()
	}
	fmt.Printf("Format: %s\n", label)
	if len(desc) == 0 {
		return
	}
	switch tracker.(type) {
	case *mod.Player, *s3m.Player, *xm.Player, *it.Player:
		fmt.Printf("Title:  %s\n", string(desc))
	default:
		printRADDescription(desc)
	}
	fmt.Println()
}

func printRADDescription(desc []byte) {
	s := desc
	var line []byte
	fmt.Println("Description:")
	for len(s) > 0 {
		c := s[0]
		s = s[1:]
		if c == 1 {
			fmt.Println(string(line))
			line = line[:0]
			continue
		}
		if c < 32 {
			for i := 0; i < int(c); i++ {
				line = append(line, ' ')
			}
			continue
		}
		line = append(line, c)
	}
	if len(line) > 0 {
		fmt.Println(string(line))
	}
}
