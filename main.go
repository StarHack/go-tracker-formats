// main.go - rad2wav: convert RAD, MOD, and XM tune files to WAV audio.
// Pure Go, no CGO.
//
// Usage:
//
//	rad2wav input.{rad,mod,xm} [-o output.wav] [-rate 44100]
package main

import (
	"encoding/binary"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"rad2wav/formats"
	"rad2wav/formats/mod"
	radv1 "rad2wav/formats/rad-v1"
	radv2 "rad2wav/formats/rad-v2"
	"rad2wav/formats/xm"
	"rad2wav/opal"
)

const defaultSampleRate = 44100

func main() {
	outFlag := flag.String("o", "", "output WAV file (default: input basename + .wav)")
	rateFlag := flag.Int("rate", defaultSampleRate, "output sample rate in Hz")
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: rad2wav [options] input.{rad,mod,xm}\n\nOptions:\n")
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

	if err := writeWAV(outFile, samples, sampleRate); err != nil {
		fmt.Fprintf(os.Stderr, "Error writing WAV: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Written: %s\n", outFile)
}

// detect returns either a formats.Tracker (OPL) or formats.PCMTracker (PCM),
// or an error string.
func detect(tune []byte) (interface{}, string) {
	// RAD v1 / v2
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
				if e := radv1.Validate(tune); e != "" {
					return nil, e
				}
				return &radv1.Player{}, ""
			case 0x21:
				if e := radv2.Validate(tune); e != "" {
					return nil, e
				}
				return &radv2.Player{}, ""
			default:
				return nil, "Not a recognised RAD version."
			}
		}
	}
	// XM
	if e := xm.Validate(tune); e == "" {
		return &xm.Player{}, ""
	}
	// MOD
	if e := mod.Validate(tune); e == "" {
		return &mod.Player{}, ""
	}
	return nil, "Unrecognised file format (not RAD v1/v2, XM, or MOD)."
}

// renderToSamples dispatches to the appropriate render path.
func renderToSamples(tune []byte, sampleRate int, tracker interface{}) []int16 {
	switch t := tracker.(type) {
	case formats.Tracker:
		return renderOPL(tune, sampleRate, t)
	case formats.PCMTracker:
		return renderPCM(tune, sampleRate, t)
	}
	return nil
}

// renderOPL renders an OPL-based Tracker (RAD v1/v2) to samples.
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

// renderPCM renders a PCMTracker (MOD etc.) to samples.
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

// writeWAV writes interleaved stereo int16 PCM to a WAV file.
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
	w := func(v interface{}) { binary.Write(f, le, v) }
	f.WriteString("RIFF")
	w(riffSize)
	f.WriteString("WAVE")
	f.WriteString("fmt ")
	w(uint32(16))
	w(uint16(1))
	w(numChannels)
	w(uint32(sampleRate))
	w(byteRate)
	w(blockAlign)
	w(bitsPerSample)
	f.WriteString("data")
	w(dataSize)
	w(samples)
	return nil
}

// printInfo prints the format name and description/title.
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
	case *xm.Player:
		label = "XM"
		desc = t.GetDescription()
	}
	fmt.Printf("Format: %s\n", label)
	if len(desc) == 0 {
		return
	}
	switch tracker.(type) {
	case *mod.Player, *xm.Player:
		fmt.Printf("Title:  %s\n", string(desc))
	default:
		printRADDescription(desc)
	}
	fmt.Println()
}

// printRADDescription decodes RAD's run-length-encoded description format.
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
