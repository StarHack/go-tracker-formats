// main.go - rad2wav: convert RAD tune files to WAV audio.
// Pure Go, no CGO.
//
// Usage:
//
//	rad2wav input.rad [-o output.wav] [-rate 44100]
package main

import (
	"encoding/binary"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"rad2wav/formats"
	radv1 "rad2wav/formats/rad-v1"
	radv2 "rad2wav/formats/rad-v2"
	"rad2wav/opal"
)

const defaultSampleRate = 44100

func main() {
	outFlag := flag.String("o", "", "output WAV file (default: input basename + .wav)")
	rateFlag := flag.Int("rate", defaultSampleRate, "output sample rate in Hz")
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: rad2wav [options] input.rad\n\nOptions:\n")
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

	player, errMsg := detect(tune)
	if errMsg != "" {
		fmt.Fprintf(os.Stderr, "Invalid RAD file: %s\n", errMsg)
		os.Exit(1)
	}

	printDescription(player.GetDescription())

	fmt.Printf("Rendering to %s at %d Hz...\n", outFile, sampleRate)
	samples := renderToSamples(tune, sampleRate, player)
	fmt.Printf("Done: %d samples (%.2f seconds)\n", len(samples)/2, float64(len(samples)/2)/float64(sampleRate))

	if err := writeWAV(outFile, samples, sampleRate); err != nil {
		fmt.Fprintf(os.Stderr, "Error writing WAV: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Written: %s\n", outFile)
}

// detect validates the tune and returns the appropriate Tracker, or an error string.
func detect(tune []byte) (formats.Tracker, string) {
	if len(tune) < 0x11 {
		return nil, "Not a RAD tune file."
	}
	switch tune[0x10] {
	case 0x10:
		if errMsg := radv1.Validate(tune); errMsg != "" {
			return nil, errMsg
		}
		return &radv1.Player{}, ""
	case 0x21:
		if errMsg := radv2.Validate(tune); errMsg != "" {
			return nil, errMsg
		}
		return &radv2.Player{}, ""
	default:
		return nil, "Not a version 1.0 or 2.1 RAD tune."
	}
}

// renderToSamples plays the tune and returns interleaved stereo int16 PCM.
func renderToSamples(tune []byte, sampleRate int, player formats.Tracker) []int16 {
	adlib := opal.New(sampleRate)

	if errMsg := player.Init(tune, adlib.Port); errMsg != "" {
		fmt.Fprintf(os.Stderr, "Error: player init failed: %s\n", errMsg)
		os.Exit(1)
	}

	hertz := player.GetHertz()
	if hertz <= 0 {
		fmt.Fprintf(os.Stderr, "Error: failed to initialise player (bad tune?)\n")
		os.Exit(1)
	}

	samplesPerTick := sampleRate / hertz
	sampleCounter := 0
	var out []int16

	for {
		var l, r int16
		adlib.Sample(&l, &r)
		out = append(out, l, r)
		sampleCounter++
		if sampleCounter >= samplesPerTick {
			sampleCounter = 0
			if player.Update() {
				break
			}
		}
	}
	return out
}

// writeWAV writes interleaved stereo int16 samples to a PCM WAV file.
func writeWAV(path string, samples []int16, sampleRate int) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	numChannels := uint16(2)
	bitsPerSample := uint16(16)
	byteRate := uint32(sampleRate) * uint32(numChannels) * uint32(bitsPerSample/8)
	blockAlign := numChannels * bitsPerSample / 8
	dataSize := uint32(len(samples)) * uint32(bitsPerSample/8)
	riffSize := 36 + dataSize
	le := binary.LittleEndian
	write := func(v interface{}) { binary.Write(f, le, v) }

	f.WriteString("RIFF")
	write(riffSize)
	f.WriteString("WAVE")
	f.WriteString("fmt ")
	write(uint32(16))
	write(uint16(1))
	write(numChannels)
	write(uint32(sampleRate))
	write(byteRate)
	write(blockAlign)
	write(bitsPerSample)
	f.WriteString("data")
	write(dataSize)
	write(samples)
	return nil
}

// printDescription prints the embedded tune description.
func printDescription(desc []byte) {
	if len(desc) == 0 {
		return
	}
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
	fmt.Println()
}
