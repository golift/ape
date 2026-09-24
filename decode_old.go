package ape

import "encoding/binary"

// parseOld reads a 3.93–3.97 header. The range coder is the pre-3990 one.
// Versions before 3.93 use an older predictor and are rejected.
//
//nolint:cyclop,funlen // the legacy header has several optional fields
func parseOld(raw []byte, version int) (apeFile, error) {
	if version < version3930 || len(raw) < oldHeaderSize {
		return apeFile{}, ErrUnsupported
	}

	var (
		level       = Compression(binary.LittleEndian.Uint16(raw[6:]))
		flags       = binary.LittleEndian.Uint16(raw[8:])
		channels    = int(binary.LittleEndian.Uint16(raw[10:]))
		rate        = int(binary.LittleEndian.Uint32(raw[12:]))
		headerBytes = int(binary.LittleEndian.Uint32(raw[16:]))
		terminating = int(binary.LittleEndian.Uint32(raw[20:]))
		frames      = int(binary.LittleEndian.Uint32(raw[24:]))
		finalBlocks = int(binary.LittleEndian.Uint32(raw[28:]))
	)

	if frames <= 0 || channels < 1 || channels > maxChannels {
		return apeFile{}, errEmptyAudio
	}

	extra := 0
	if flags&flagHasPeak != 0 {
		extra += wordSize
	}

	seekCount := frames

	if flags&flagHasSeek != 0 {
		at := oldHeaderSize + extra
		if at+wordSize > len(raw) {
			return apeFile{}, errShortSeekCount
		}

		seekCount = int(binary.LittleEndian.Uint32(raw[at:]))
		extra += wordSize
	}

	wav := 0
	if flags&flagCreateWAV == 0 {
		wav = headerBytes
	}

	seekAt := oldHeaderSize + extra + wav
	if seekCount < frames || seekAt < 0 || seekAt+seekCount*wordSize > len(raw) {
		return apeFile{}, errShortSeekTable
	}

	seek := make([]uint32, frames)
	for idx := range frames {
		seek[idx] = binary.LittleEndian.Uint32(raw[seekAt+idx*wordSize:])
	}

	audioEnd := len(raw) - terminating
	if audioEnd < seekAt || audioEnd > len(raw) {
		audioEnd = len(raw)
	}

	if version < version3950 && level == CompressionInsane {
		return apeFile{}, ErrUnsupported
	}

	frameBlocks := blocksFast
	if version >= version3950 {
		frameBlocks = blocksExtra
	}

	bits := 16

	switch {
	case flags&flag8Bit != 0:
		bits = 8
	case flags&flag24Bit != 0:
		bits = 24
	}

	samples := (frames-1)*frameBlocks + finalBlocks

	return apeFile{
		stream: Stream{
			Bits:       bits,
			Channels:   channels,
			SampleRate: rate,
		},
		level:   level,
		version: version,
		frames:  frames,
		samples: samples,
		audio:   raw,
		seek:    seek,
		end:     audioEnd,
		blocks: func(idx int) int {
			if idx == frames-1 {
				return finalBlocks
			}

			return frameBlocks
		},
	}, nil
}
