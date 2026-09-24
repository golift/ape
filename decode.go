package ape

import (
	"encoding/binary"
	"fmt"
	"io"
)

// Decode reads a version 3990 APE file and returns interleaved little-endian PCM.
func Decode(src io.Reader) ([]byte, Stream, error) {
	raw, err := io.ReadAll(src)
	if err != nil {
		return nil, Stream{}, fmt.Errorf("ape: reading file: %w", err)
	}

	file, err := parseFile(raw)
	if err != nil {
		return nil, Stream{}, err
	}

	pcm := make([]byte, 0, file.samples*file.stream.blockAlign())
	for idx := range file.frames {
		frame, skip := file.frame(idx)

		chunk, err := decodeFrame(frame, skip, file.blocks(idx), file.stream, file.level, file.version)
		if err != nil {
			return nil, Stream{}, err
		}

		pcm = append(pcm, chunk...)
	}

	if file.stream.Float {
		pcm = transformFloat(pcm)
	}

	return pcm, file.stream, nil
}

type apeFile struct {
	stream  Stream
	level   Compression
	version int
	blocks  func(int) int
	frames  int
	samples int
	audio   []byte
	seek    []uint32
	end     int
}

// frame returns the frame bytes aligned to the first frame's word boundary.
// skip is the bit index of the frame's first bit inside that window.
func (f *apeFile) frame(idx int) ([]byte, uint32) {
	start := int(f.seek[idx])
	remainder := (start - int(f.seek[0])) & (wordSize - 1)
	aligned := start - remainder

	stop := f.end
	if idx+1 < len(f.seek) {
		stop = int(f.seek[idx+1])
	}

	stop += wordSize
	if stop > len(f.audio) {
		stop = len(f.audio)
	}

	if aligned < 0 || aligned > stop {
		return nil, 0
	}

	return f.audio[aligned:stop], uint32(remainder * 8)
}

//nolint:cyclop,funlen // container versions share one parser
func parseFile(raw []byte) (apeFile, error) {
	if len(raw) < descriptorSize+headerSize || (string(raw[:3]) != "MAC" && string(raw[:4]) != "MACF") {
		return apeFile{}, ErrUnsupported
	}

	version := int(binary.LittleEndian.Uint16(raw[4:]))
	if version < version3980 {
		return parseOld(raw, version)
	}

	descBytes := int(binary.LittleEndian.Uint32(raw[8:]))
	headBytes := int(binary.LittleEndian.Uint32(raw[12:]))
	seekBytesN := int(binary.LittleEndian.Uint32(raw[16:]))
	headerData := int(binary.LittleEndian.Uint32(raw[20:]))
	frameBytes := int(binary.LittleEndian.Uint32(raw[24:])) + int(binary.LittleEndian.Uint32(raw[28:]))<<32

	if descBytes < descriptorSize || headBytes < headerSize || len(raw) < descBytes+headBytes+seekBytesN {
		return apeFile{}, errShortDescriptor
	}

	head := raw[descBytes : descBytes+headBytes]
	level := Compression(binary.LittleEndian.Uint16(head[0:]))
	flags := binary.LittleEndian.Uint16(head[2:])
	frameBlocks := int(binary.LittleEndian.Uint32(head[4:]))
	finalBlocks := int(binary.LittleEndian.Uint32(head[8:]))
	frames := int(binary.LittleEndian.Uint32(head[12:]))
	stream := Stream{
		Bits:       int(binary.LittleEndian.Uint16(head[16:])),
		Channels:   int(binary.LittleEndian.Uint16(head[18:])),
		SampleRate: int(binary.LittleEndian.Uint32(head[20:])),
		Float:      raw[3] == 'F' || flags&flagFloat != 0,
	}

	if frames <= 0 || seekBytesN < frames*wordSize || frameBlocks <= 0 {
		return apeFile{}, errEmptyAudio
	}

	seekAt := descBytes + headBytes

	seek := make([]uint32, frames)
	for idx := range frames {
		seek[idx] = binary.LittleEndian.Uint32(raw[seekAt+idx*wordSize:])
	}

	audioStart := seekAt + seekBytesN

	audioEnd := min(audioStart+headerData+frameBytes, len(raw))

	samples := (frames-1)*frameBlocks + finalBlocks

	return apeFile{
		stream:  stream,
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

//nolint:cyclop,funlen // silence, pseudo-stereo, stereo, and multi-channel are separate paths
func decodeFrame(frame []byte, skip uint32, blocks int, stream Stream, level Compression, version int) ([]byte, error) {
	if len(frame) < 8 || blocks < 0 {
		return nil, errShortFrame
	}

	reader := newRangeReader(frame, skip)
	reader.version = version
	crc := reader.readBits(32)

	special := uint32(0)
	if crc&(1<<crcHighBit) != 0 {
		special = reader.readBits(32)
	}

	reader.startCoder()

	out := make([]byte, blocks*stream.blockAlign())
	preds := make([]*decoder, stream.Channels)
	sums := make([]coderState, stream.Channels)

	for idx := range preds {
		preds[idx] = newDecoder(stream.Bits, level, version)
		sums[idx] = newCoderState()
	}

	var lastX int32

	for block := range blocks {
		ch := make([]int32, stream.Channels)
		switch {
		case stream.Channels == 1 && special&specialMonoSilence != 0:
		case stream.Channels == 2 && special&specialLeftSilence != 0 && special&specialRightSilence != 0:
		case stream.Channels == 2 && version < version3950 && special&specialPseudoStereo == 0:
			x := reader.decodeValue(&sums[0])
			y := reader.decodeValue(&sums[1])
			ch[0] = preds[0].decompress(x, 0)
			ch[1] = preds[1].decompress(y, 0)
		case stream.Channels == 2 && special&specialPseudoStereo != 0:
			ch[0] = preds[0].decompress(reader.decodeValue(&sums[0]), 0)
			lastX = ch[0]
		case stream.Channels == 2:
			y := reader.decodeValue(&sums[1])
			x := reader.decodeValue(&sums[0])
			ch[1] = preds[1].decompress(y, int64(lastX))
			ch[0] = preds[0].decompress(x, int64(ch[1]))
			lastX = ch[0]
		default:
			for idx := range ch {
				ch[idx] = preds[idx].decompress(reader.decodeValue(&sums[idx]), 0)
			}
		}

		writeBlock(out[block*stream.blockAlign():], ch, stream.Bits)
	}

	if reader.short {
		return nil, errShortFrame
	}

	return out, nil
}
