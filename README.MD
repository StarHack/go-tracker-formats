# go-tracker-formats

Supports working with the following listed formats. For implementation guidelines on how to work with each format, check out `examples/` directory.

- RAD (v1/v2)
- MOD
- XM (FastTracker 2 style modules)
- S3M (partially)
- IT (Impulse Tracker module, partially works)

Planned support (not yet there):

- FTM (FamiTracker music / NES chiptune scene)

## Formats

Multiple formats were implemented independently and the idea is you may use parts of this library for your particular use case.

### RAD

Ports some concepts of Reality Adlib Tracker format into pure Go code. They do provide modules available for download on https://www.3eality.com/Public/Modules/

- **OPAL** is the OPL3 emulator bundled with RAD V1/V2. It emulates Yamaha OPL3 FM-synthesis hardware so RAD tunes can be played on machines that do not have real OPL3 hardware. In the RAD docs, it is explicitly described as “an OPL3 emulator” included with RAD V1/V2.

- **RAD-v1** is the older first-generation format/tracker for Reality Adlib Tracker. The RAD V2 documentation says the V2 file format is “significantly different” and points readers to the RAD V1 docs for the old layout; the bundled examples also include a “Raster V2” tune described as a remake of a “RAD V1 Adlib classic.”

- **RAD-v2** is the newer version of Reality Adlib Tracker, released in May 2018 as Version 2.0a. It includes the tracker itself, sample tunes, source files such as player20.cpp, validate20.cpp, and opal.cpp, and uses a newer RAD 2.1 file format.

### MOD

- **MOD** support is implemented as a PCM/sample-based tracker path rather than an OPL/FM path.

- The current MOD implementation targets classic 4-channel 15-sample Soundtracker/NoiseTracker style modules as well as tagged 31-sample ProTracker-compatible variants such as `M.K.`, `M!K!`, `FLT4/8`, `xCHN`, and `xxCH`.

### XM

- **XM / FastTracker 2** implemented in its own `formats/xm/` package because XM adds a substantially different file structure and playback model: compressed pattern events, instruments with note-to-sample maps, envelopes, fadeout, 8/16-bit delta-coded samples, ping-pong loops, volume-column commands, and linear-frequency playback.

### S3M

- Only supports some core effects so far

### IT

- **IT / Impulse Tracker** lives in `formats/it/`. The loader covers the on-disk header, instrument/sample headers, pattern unpacking, and IT 2.14 sample compression. The player is still minimal at this stage (core row timing, note/sample/instrument routing, global volume, and a few row commands such as speed and tempo).

## Testcases

### XM

```
go test -run TestXM_ListSamples -v ./test

go test -run TestXM_ExtractSamples -v ./test -args \
  -xm_extract_samples "Instrument 3/drum - orchestra,Instrument 41/drum - core" \
  -xm_extract_out_dir "../sample-data/extracted"

```

