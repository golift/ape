package ape_test

import (
	"bytes"
	"encoding/binary"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"golift.io/ape"
)

func TestEncodeRoundTrip(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		channels int
		blocks   int
		pcm      []byte
	}{
		{name: "stereo", channels: 2, blocks: 64, pcm: stereoPCM(200)},
		{name: "mono", channels: 1, blocks: 64, pcm: monoPCM(180)},
		{name: "silence", channels: 2, blocks: 32, pcm: bytes.Repeat([]byte{0, 0, 0, 0}, 40)},
		{name: "pseudo", channels: 2, blocks: 32, pcm: pseudoPCM(90)},
		{name: "short-final", channels: 2, blocks: 50, pcm: stereoPCM(120)},
		{name: "roll", channels: 2, blocks: 300, pcm: stereoPCM(300)},
		{name: "negative", channels: 1, blocks: 16, pcm: negativePCM(40)},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			roundTrip(t, tc.pcm, ape.Stream{SampleRate: 44100, Channels: tc.channels, Bits: 16}, &ape.Options{
				Compression:    ape.CompressionFast,
				BlocksPerFrame: tc.blocks,
			})
		})
	}
}

func TestUnsupported(t *testing.T) {
	t.Parallel()

	pcm := stereoPCM(8)
	stream := ape.Stream{SampleRate: 44100, Channels: 2, Bits: 16}
	dst := (*os.File)(nil)

	err := ape.Encode(dst, pcm, stream, &ape.Options{Compression: 1500, BlocksPerFrame: 8})
	if err == nil {
		t.Fatal("level 1500 encoded")
	}

	err = ape.Encode(dst, pcm, ape.Stream{SampleRate: 44100, Channels: 33, Bits: 16}, nil)
	if err == nil {
		t.Fatal("33 channels encoded")
	}
}

func TestNormal(t *testing.T) {
	t.Parallel()

	opt := &ape.Options{Compression: ape.CompressionNormal, BlocksPerFrame: 600}
	roundTrip(t, stereoPCM(700), ape.Stream{SampleRate: 44100, Channels: 2, Bits: 16}, opt)
	roundTrip(t, monoPCM(80), ape.Stream{SampleRate: 44100, Channels: 1, Bits: 16}, &ape.Options{
		Compression:    ape.CompressionNormal,
		BlocksPerFrame: 40,
	})
	roundTrip(t, pcm8(50), ape.Stream{SampleRate: 22050, Channels: 1, Bits: 8}, &ape.Options{
		Compression:    ape.CompressionNormal,
		BlocksPerFrame: 16,
	})
	roundTrip(t, pcm24(40), ape.Stream{SampleRate: 48000, Channels: 2, Bits: 24}, &ape.Options{
		Compression:    ape.CompressionNormal,
		BlocksPerFrame: 16,
	})
	encodeOnly(t, pcm32(24), ape.Stream{SampleRate: 96000, Channels: 1, Bits: 32}, &ape.Options{
		Compression:    ape.CompressionNormal,
		BlocksPerFrame: 16,
	})
}

func TestHigherLevels(t *testing.T) {
	t.Parallel()

	pcm := stereoPCM(700)

	stream := ape.Stream{SampleRate: 44100, Channels: 2, Bits: 16}
	for _, level := range []ape.Compression{ape.CompressionHigh, ape.CompressionExtraHigh, ape.CompressionInsane} {
		roundTrip(t, pcm, stream, &ape.Options{Compression: level, BlocksPerFrame: 600})
	}

	roundTrip(t, pcm8(40), ape.Stream{SampleRate: 22050, Channels: 1, Bits: 8}, &ape.Options{
		Compression:    ape.CompressionHigh,
		BlocksPerFrame: 16,
	})
	roundTrip(t, pcm24(40), ape.Stream{SampleRate: 48000, Channels: 2, Bits: 24}, &ape.Options{
		Compression:    ape.CompressionInsane,
		BlocksPerFrame: 16,
	})
}

func TestDecode(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		pcm    []byte
		stream ape.Stream
		level  ape.Compression
	}{
		{name: "fast", pcm: stereoPCM(90), stream: ape.Stream{SampleRate: 44100, Channels: 2, Bits: 16}},
		{name: "mono", pcm: monoPCM(40), stream: ape.Stream{SampleRate: 44100, Channels: 1, Bits: 16}},
		{name: "silence", pcm: bytes.Repeat([]byte{0, 0, 0, 0}, 20), stream: ape.Stream{SampleRate: 44100, Channels: 2, Bits: 16}},
		{name: "pseudo", pcm: pseudoPCM(30), stream: ape.Stream{SampleRate: 44100, Channels: 2, Bits: 16}},
		{name: "normal", pcm: stereoPCM(80), stream: ape.Stream{SampleRate: 44100, Channels: 2, Bits: 16}, level: ape.CompressionNormal},
		{name: "high", pcm: stereoPCM(40), stream: ape.Stream{SampleRate: 44100, Channels: 2, Bits: 16}, level: ape.CompressionHigh},
		{name: "8", pcm: pcm8(30), stream: ape.Stream{SampleRate: 22050, Channels: 1, Bits: 8}},
		{name: "24", pcm: pcm24(24), stream: ape.Stream{SampleRate: 48000, Channels: 2, Bits: 24}},
		{name: "32", pcm: pcm32(16), stream: ape.Stream{SampleRate: 96000, Channels: 1, Bits: 32}},
		{name: "extra", pcm: stereoPCM(40), stream: ape.Stream{SampleRate: 44100, Channels: 2, Bits: 16}, level: ape.CompressionExtraHigh},
		{name: "insane", pcm: stereoPCM(40), stream: ape.Stream{SampleRate: 44100, Channels: 2, Bits: 16}, level: ape.CompressionInsane},
		{name: "float", pcm: pcmFloat(16), stream: ape.Stream{SampleRate: 44100, Channels: 2, Bits: 32, Float: true}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			path := filepath.Join(t.TempDir(), "out.ape")

			file, err := os.Create(path)
			if err != nil {
				t.Fatal(err)
			}

			err = ape.Encode(file, tc.pcm, tc.stream, &ape.Options{
				Compression:    tc.level,
				BlocksPerFrame: 32,
			})
			if err != nil {
				t.Fatal(err)
			}

			err = file.Close()
			if err != nil {
				t.Fatal(err)
			}

			raw, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}

			got, stream, err := ape.Decode(bytes.NewReader(raw))
			if err != nil {
				t.Fatal(err)
			}

			if stream.Channels != tc.stream.Channels || stream.Bits != tc.stream.Bits {
				t.Fatalf("stream %+v", stream)
			}

			if !bytes.Equal(got, tc.pcm) {
				for i := range min(len(got), len(tc.pcm)) {
					if got[i] != tc.pcm[i] {
						t.Fatalf("pcm mismatch at %d got %x want %x", i, got[i:], tc.pcm[i:])
					}
				}

				t.Fatalf("pcm length %d vs %d", len(got), len(tc.pcm))
			}
		})
	}
}

func TestLink(t *testing.T) {
	t.Parallel()

	pcm := stereoPCM(80)
	dir := t.TempDir()
	image := filepath.Join(dir, "image.ape")

	file, err := os.Create(image)
	if err != nil {
		t.Fatal(err)
	}

	err = ape.Encode(file, pcm, ape.Stream{SampleRate: 44100, Channels: 2, Bits: 16}, &ape.Options{BlocksPerFrame: 32})
	if err != nil {
		t.Fatal(err)
	}

	err = file.Close()
	if err != nil {
		t.Fatal(err)
	}

	link := filepath.Join(dir, "track.apl")
	body := "[Monkey's Audio Image Link File]\r\nImage File=image.ape\r\nStart Block=10\r\nFinish Block=40\r\n"

	err = os.WriteFile(link, []byte(body), 0o600)
	if err != nil {
		t.Fatal(err)
	}

	got, stream, err := ape.DecodeFile(link)
	if err != nil {
		t.Fatal(err)
	}

	if stream.Channels != 2 || !bytes.Equal(got, pcm[10*4:40*4]) {
		t.Fatalf("link pcm %d bytes stream %+v", len(got), stream)
	}
}

func TestChannels(t *testing.T) {
	t.Parallel()

	// ffmpeg's APE decoder only accepts mono and stereo, so these check the
	// channel count stored in the header.
	for _, channels := range []int{3, 4, 5, 6, 7, 8, 32} {
		encodeOnly(t, multiPCM(channels, 12), ape.Stream{
			SampleRate: 44100,
			Channels:   channels,
			Bits:       16,
		}, shortFrame())
	}

	encodeOnly(t, multiPCM24(4, 12), ape.Stream{SampleRate: 48000, Channels: 4, Bits: 24}, shortFrame())
}

func TestBitDepths(t *testing.T) {
	t.Parallel()

	roundTrip(t, pcm8(40), ape.Stream{SampleRate: 44100, Channels: 1, Bits: 8}, shortFrame())
	roundTrip(t, pcm24(30), ape.Stream{SampleRate: 48000, Channels: 2, Bits: 24}, shortFrame())
	encodeOnly(t, pcm32(20), ape.Stream{SampleRate: 96000, Channels: 1, Bits: 32}, shortFrame())
	encodeOnly(t, pcmFloat(16), ape.Stream{SampleRate: 44100, Channels: 2, Bits: 32, Float: true}, shortFrame())
}

func encodeOnly(t *testing.T, pcm []byte, stream ape.Stream, opt *ape.Options) {
	t.Helper()

	path := filepath.Join(t.TempDir(), "out.ape")

	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}

	err = ape.Encode(file, pcm, stream, opt)
	if err != nil {
		t.Fatal(err)
	}

	err = file.Close()
	if err != nil {
		t.Fatal(err)
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	got := binary.LittleEndian.Uint16(raw[70:])
	if int(got) != stream.Channels {
		t.Fatalf("channels %d, want %d", got, stream.Channels)
	}
}

func TestHeaderAndTag(t *testing.T) {
	t.Parallel()

	header := []byte("RIFF....WAVEfmt ")
	footer := []byte("data....")
	opt := shortFrame()
	opt.Header = header
	opt.Footer = footer
	opt.Format = ape.FormatWAV
	opt.Tags = map[string]string{"Artist": "Go Lift", "Title": "Parity"}

	dir := t.TempDir()
	path := filepath.Join(dir, "out.ape")

	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}

	pcm := stereoPCM(40)

	err = ape.Encode(file, pcm, ape.Stream{SampleRate: 44100, Channels: 2, Bits: 16}, opt)
	if err != nil {
		t.Fatal(err)
	}

	err = file.Close()
	if err != nil {
		t.Fatal(err)
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	if !bytes.Contains(raw, header) || !bytes.Contains(raw, footer) || !bytes.Contains(raw, []byte("APETAGEX")) {
		t.Fatal("header, footer, or tag missing")
	}

	roundTrip(t, pcm, ape.Stream{SampleRate: 44100, Channels: 2, Bits: 16}, opt)
}

func shortFrame() *ape.Options {
	return &ape.Options{Compression: ape.CompressionFast, BlocksPerFrame: 16}
}

func roundTrip(t *testing.T, pcm []byte, stream ape.Stream, opt *ape.Options) {
	t.Helper()

	dir := t.TempDir()
	path := filepath.Join(dir, "out.ape")

	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}

	err = ape.Encode(file, pcm, stream, opt)
	if err != nil {
		t.Fatal(err)
	}

	err = file.Close()
	if err != nil {
		t.Fatal(err)
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	magic := "MAC "
	sampleFormat := "s16le"

	switch {
	case stream.Float:
		magic = "MACF"
		sampleFormat = "f32le"
	case stream.Bits == 8:
		sampleFormat = "u8"
	case stream.Bits == 24:
		sampleFormat = "s24le"
	case stream.Bits == 32:
		sampleFormat = "s32le"
	}

	if string(raw[:4]) != magic {
		t.Fatalf("magic %q", raw[:4])
	}

	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("ffmpeg not in PATH")
	}

	pcmPath := filepath.Join(dir, "out.pcm")
	//nolint:gosec // ffmpeg is the decode oracle
	cmd := exec.CommandContext(t.Context(), ffmpeg, "-y", "-i", path, "-f", sampleFormat, pcmPath)

	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("ffmpeg: %v\n%s", err, out)
	}

	got, err := os.ReadFile(pcmPath)
	if err != nil {
		t.Fatal(err)
	}

	if !bytes.Equal(got, pcm) {
		t.Fatalf("pcm mismatch: got %d bytes, want %d", len(got), len(pcm))
	}
}

func stereoPCM(n int) []byte {
	out := make([]byte, n*4)
	for i := range n {
		put16(out[i*4:], int16(i*17-40))
		put16(out[i*4+2:], int16(80-i*3))
	}

	return out
}

func monoPCM(n int) []byte {
	out := make([]byte, n*2)
	for i := range n {
		put16(out[i*2:], int16(i*13-20))
	}

	return out
}

func pseudoPCM(n int) []byte {
	out := make([]byte, n*4)
	for i := range n {
		sample := int16(i*5 - 10)
		put16(out[i*4:], sample)
		put16(out[i*4+2:], sample)
	}

	return out
}

func negativePCM(n int) []byte {
	out := make([]byte, n*2)
	for i := range n {
		put16(out[i*2:], int16(-1000-i))
	}

	return out
}

func put16(dst []byte, v int16) {
	dst[0] = byte(v)
	dst[1] = byte(v >> 8)
}

func multiPCM(channels, blocks int) []byte {
	out := make([]byte, channels*blocks*2)
	for i := range channels * blocks {
		put16(out[i*2:], int16(i*13-80+channels))
	}

	return out
}

func multiPCM24(channels, blocks int) []byte {
	out := make([]byte, channels*blocks*3)
	for i := range channels * blocks {
		put24(out[i*3:], i*90-200)
	}

	return out
}

func pcm8(n int) []byte {
	out := make([]byte, n)
	for i := range n {
		out[i] = byte(i*3 + 20)
	}

	return out
}

func pcm24(n int) []byte {
	out := make([]byte, n*6)
	for i := range n {
		put24(out[i*6:], i*1000-500)
		put24(out[i*6+3:], 8000-i*50)
	}

	return out
}

func pcm32(n int) []byte {
	out := make([]byte, n*4)
	for i := range n {
		put32(out[i*4:], int32(i*100000-40000))
	}

	return out
}

func pcmFloat(n int) []byte {
	out := make([]byte, n*8)
	for i := range n {
		putFloat(out[i*8:], float32(i)*0.01-0.2)
		putFloat(out[i*8+4:], 0.5-float32(i)*0.02)
	}

	return out
}

func put24(dst []byte, v int) {
	dst[0] = byte(v)
	dst[1] = byte(v >> 8)
	dst[2] = byte(v >> 16)
}

func put32(dst []byte, v int32) {
	dst[0] = byte(v)
	dst[1] = byte(v >> 8)
	dst[2] = byte(v >> 16)
	dst[3] = byte(v >> 24)
}

func putFloat(dst []byte, v float32) {
	put32(dst, int32(math.Float32bits(v)))
}
