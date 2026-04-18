// Package ftm reads FamiTracker module (.ftm) files: the binary block format used by
// FamiTracker / Dn-FamiTracker (see Dn-FamiTracker Source/DocumentFile.cpp and FamiTrackerDoc.cpp).
//
// LoadModule decodes all standard blocks into structured Go types. Validation follows the same
// bounds checks as the reference tracker for supported file versions (0x0200–0x0450).
//
// PCM playback (formats.PCMTracker) runs a cycle-accurate Ricoh 2A03 core (package nes2a03) from
// decoded pattern data. Expansion chips (VRC6, VRC7, FDS, N163, S5B, MMC5) are not emulated yet.
package ftm
