package main

import (
	"encoding/binary"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/ebitengine/oto/v3"

	"github.com/StarHack/go-tracker-formats/formats/s3m"
)

type s3mPCMStream struct {
	player     *s3m.Player
	maxSamples int
	sent       int
	done       bool
}

func (s *s3mPCMStream) Read(p []byte) (int, error) {
	if s.done {
		return 0, io.EOF
	}
	written := 0
	for written+4 <= len(p) {
		if s.maxSamples > 0 && s.sent >= s.maxSamples {
			s.done = true
			break
		}
		var l, r int16
		repeat := s.player.Sample(&l, &r)
		binary.LittleEndian.PutUint16(p[written:written+2], uint16(l))
		binary.LittleEndian.PutUint16(p[written+2:written+4], uint16(r))
		written += 4
		s.sent++
		if repeat {
			s.done = true
			break
		}
	}
	if written == 0 && s.done {
		return 0, io.EOF
	}
	return written, nil
}

func resolveDefaultS3MPath() string {
	candidates := []string{
		filepath.Join("sample-data", "s3m_--_moving_that_rhythm.s3m"),
		filepath.Join("..", "..", "sample-data", "s3m_--_moving_that_rhythm.s3m"),
	}
	for _, c := range candidates {
		if _, err := os.Stat(c); err == nil {
			return c
		}
	}
	return candidates[0]
}

func main() {
	s3mPath := flag.String("s3m", "", "Path to S3M (Scream Tracker 3) module")
	rate := flag.Int("rate", 44100, "Playback sample rate")
	seconds := flag.Int("seconds", 0, "Playback duration in seconds (0 = until module repeats)")
	flag.Parse()

	path := *s3mPath
	if path == "" {
		path = resolveDefaultS3MPath()
	}

	moduleData, err := os.ReadFile(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "read s3m: %v\n", err)
		os.Exit(1)
	}

	pl := &s3m.Player{}
	if e := pl.Init(moduleData, *rate); e != "" {
		fmt.Fprintf(os.Stderr, "init s3m player: %s\n", e)
		os.Exit(1)
	}

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

	maxSamples := 0
	if *seconds > 0 {
		maxSamples = *rate * *seconds
	}
	stream := &s3mPCMStream{player: pl, maxSamples: maxSamples}
	op := ctx.NewPlayer(stream)
	defer op.Close()

	fmt.Printf("Playing %s at %d Hz...\n", path, *rate)
	op.Play()
	for op.IsPlaying() {
		time.Sleep(20 * time.Millisecond)
	}
	fmt.Println("Done.")
}
