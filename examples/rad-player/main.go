package main

import (
	"bufio"
	"encoding/binary"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/ebitengine/oto/v3"

	"github.com/StarHack/go-tracker-formats/formats"
	radv1 "github.com/StarHack/go-tracker-formats/formats/rad-v1"
	radv2 "github.com/StarHack/go-tracker-formats/formats/rad-v2"
	"github.com/StarHack/go-tracker-formats/opal"
)

type radPCMStream struct {
	player      formats.Tracker
	adlib       *opal.Opal
	maxSamples  int
	sentSamples int
	samPerTick  int
	samCnt      int
	done        bool
}

func (s *radPCMStream) Read(p []byte) (int, error) {
	if s.done {
		return 0, io.EOF
	}
	written := 0
	for written+4 <= len(p) {
		if s.maxSamples > 0 && s.sentSamples >= s.maxSamples {
			s.done = true
			break
		}
		var l, r int16
		s.adlib.Sample(&l, &r)
		binary.LittleEndian.PutUint16(p[written:written+2], uint16(l))
		binary.LittleEndian.PutUint16(p[written+2:written+4], uint16(r))
		written += 4
		s.sentSamples++

		s.samCnt++
		if s.samCnt >= s.samPerTick {
			s.samCnt = 0
			if s.player.Update() {
				s.done = true
				break
			}
		}
	}
	if written == 0 && s.done {
		return 0, io.EOF
	}
	return written, nil
}

func detectRADPlayer(tune []byte) (formats.Tracker, error) {
	if len(tune) < 17 {
		return nil, fmt.Errorf("file too short")
	}
	const hdr = "RAD by REALiTY!!"
	for i := 0; i < 16; i++ {
		if tune[i] != hdr[i] {
			return nil, fmt.Errorf("not a RAD file")
		}
	}
	switch tune[0x10] {
	case 0x10:
		if e := radv1.Validate(tune); e != "" {
			return nil, fmt.Errorf("rad v1 validate failed: %s", e)
		}
		return &radv1.Player{}, nil
	case 0x21:
		if e := radv2.Validate(tune); e != "" {
			return nil, fmt.Errorf("rad v2 validate failed: %s", e)
		}
		return &radv2.Player{}, nil
	default:
		return nil, fmt.Errorf("unsupported RAD version byte: 0x%02X", tune[0x10])
	}
}

func resolveExistingPath(candidates []string) string {
	for _, c := range candidates {
		if _, err := os.Stat(c); err == nil {
			return c
		}
	}
	return candidates[0]
}

func resolveDemoRADPaths() (string, string) {
	v1 := resolveExistingPath([]string{
		filepath.Join("sample-data", "Void - Mindflux v1.rad"),
		filepath.Join("..", "..", "sample-data", "Void - Mindflux v1.rad"),
	})
	v2 := resolveExistingPath([]string{
		filepath.Join("sample-data", "Void - Raster v2.rad"),
		filepath.Join("..", "..", "sample-data", "Void - Raster v2.rad"),
	})
	return v1, v2
}

func playRAD(ctx *oto.Context, path string, rate, seconds int, allowEnterSkip bool) error {
	tune, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read RAD: %w", err)
	}
	player, err := detectRADPlayer(tune)
	if err != nil {
		return fmt.Errorf("detect RAD: %w", err)
	}

	adlib := opal.New(rate)
	if e := player.Init(tune, adlib.Port); e != "" {
		return fmt.Errorf("init RAD player: %s", e)
	}
	hz := player.GetHertz()
	if hz <= 0 {
		return fmt.Errorf("invalid player hertz: %d", hz)
	}
	samPerTick := rate / hz
	if samPerTick < 1 {
		samPerTick = 1
	}
	maxSamples := 0
	if seconds > 0 {
		maxSamples = rate * seconds
	}
	stream := &radPCMStream{
		player:     player,
		adlib:      adlib,
		maxSamples: maxSamples,
		samPerTick: samPerTick,
	}
	op := ctx.NewPlayer(stream)
	defer op.Close()

	fmt.Printf("Playing %s at %d Hz...\n", path, rate)
	if allowEnterSkip {
		fmt.Println("Press Enter to skip to the next stage.")
	}
	op.Play()

	var enterC <-chan struct{}
	if allowEnterSkip {
		ch := make(chan struct{}, 1)
		enterC = ch
		go func() {
			r := bufio.NewReader(os.Stdin)
			_, _ = r.ReadString('\n')
			ch <- struct{}{}
		}()
	}

	for op.IsPlaying() {
		if allowEnterSkip {
			select {
			case <-enterC:
				fmt.Println("Skipped by user.")
				op.Pause()
				op.Close()
				return nil
			default:
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	fmt.Println("Playback finished.")
	return nil
}

func main() {
	radPath := flag.String("rad", "", "Path to RAD file (v1 or v2). If empty, demo mode plays v1 then v2.")
	rate := flag.Int("rate", 44100, "Playback sample rate")
	seconds := flag.Int("seconds", 0, "Playback duration in seconds (0 = until module repeats)")
	flag.Parse()

	ctx, ready, err := oto.NewContext(&oto.NewContextOptions{
		SampleRate:   *rate,
		ChannelCount: 2,
		Format:       oto.FormatSignedInt16LE,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "create oto context: %v\n", err)
		os.Exit(1)
	}
	<-ready

	if *radPath != "" {
		if err := playRAD(ctx, *radPath, *rate, *seconds, false); err != nil {
			fmt.Fprintf(os.Stderr, "%v\n", err)
			os.Exit(1)
		}
		fmt.Println("Done.")
		return
	}

	v1, v2 := resolveDemoRADPaths()
	fmt.Println("RAD demo mode:")
	if err := playRAD(ctx, v1, *rate, *seconds, true); err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}
	fmt.Println()
	if err := playRAD(ctx, v2, *rate, *seconds, true); err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}
	fmt.Println("Press Enter to exit.")
	_, _ = bufio.NewReader(os.Stdin).ReadString('\n')
	fmt.Println("Done.")
}
