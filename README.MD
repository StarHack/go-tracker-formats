# go-tracker-formats

Supports parsing the following formats:

- RAD (v1/v2)
- MOD

Planned support (not yet there):

- XM (FastTracker 2 style modules, via a separate formats/xm package)
- S3M
- IT

Sample files can be downloaded from:

https://www.3eality.com/Public/Modules/


## Formats

### RAD

Ports some concepts of Reality Adlib Tracker context:

- **OPAL** is the OPL3 emulator bundled with RAD V1/V2. It emulates Yamaha OPL3 FM-synthesis hardware so RAD tunes can be played on machines that do not have real OPL3 hardware. In the RAD docs, it is explicitly described as “an OPL3 emulator” included with RAD V1/V2.

- **RAD-v1** is the older first-generation format/tracker for Reality Adlib Tracker. The RAD V2 documentation says the V2 file format is “significantly different” and points readers to the RAD V1 docs for the old layout; the bundled examples also include a “Raster V2” tune described as a remake of a “RAD V1 Adlib classic.”

- **RAD-v2** is the newer version of Reality Adlib Tracker, released in May 2018 as Version 2.0a. It includes the tracker itself, sample tunes, source files such as player20.cpp, validate20.cpp, and opal.cpp, and uses a newer RAD 2.1 file format.

### MOD

- **MOD** support is implemented independently from the RAD players as a PCM/sample-based tracker path rather than an OPL/FM path.

- The current MOD implementation targets classic 4-channel 15-sample Soundtracker/NoiseTracker style modules as well as tagged 31-sample ProTracker-compatible variants such as `M.K.`, `M!K!`, `FLT4/8`, `xCHN`, and `xxCH`.

- **XM / FastTracker 2** should not be treated as “just another MOD variant” in the same package. It belongs in its own `formats/xm/` package because XM adds a substantially different file structure and playback model: compressed pattern events, instruments with note-to-sample maps, envelopes, fadeout, 8/16-bit delta-coded samples, ping-pong loops, volume-column commands, and linear-frequency playback.
- The existing test only compares row/tempo/speed state transitions — not actual audio samples. We need to compare real PCM output. Let me write a test that does that.