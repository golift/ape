package ape_test

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"

	"golift.io/ape"
)

func TestStreamEncodeMatchesEncode(t *testing.T) {
	t.Parallel()

	pcm := stereoPCM(250)
	stream := ape.Stream{SampleRate: 44100, Channels: 2, Bits: 16}
	opt := &ape.Options{Compression: ape.CompressionNormal, BlocksPerFrame: 40}

	one := encodeToBytes(t, pcm, stream, opt)
	chunked := encodeChunks(t, pcm, stream, opt, 7)

	if !bytes.Equal(one, chunked) {
		t.Fatal("chunked encoder output differs from Encode")
	}
}

func TestStreamDecodeMatchesDecode(t *testing.T) {
	t.Parallel()

	pcm := stereoPCM(180)
	stream := ape.Stream{SampleRate: 8000, Channels: 2, Bits: 16}
	opt := &ape.Options{Compression: ape.CompressionFast, BlocksPerFrame: 30}
	raw := encodeToBytes(t, pcm, stream, opt)

	got, _, err := ape.Decode(bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}

	dec, err := ape.NewDecoder(bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}

	if dec.Samples() != len(pcm)/pcmAlign(stream) {
		t.Fatalf("samples %d", dec.Samples())
	}

	var streamed []byte

	for {
		frame, nextErr := dec.Next()
		if errors.Is(nextErr, io.EOF) {
			break
		}

		if nextErr != nil {
			t.Fatal(nextErr)
		}

		streamed = append(streamed, frame...)
	}

	if !bytes.Equal(got, streamed) {
		t.Fatalf("streamed decode %d bytes, decode %d", len(streamed), len(got))
	}
}

func TestDecodeNonSeekable(t *testing.T) {
	t.Parallel()

	pcm := monoPCM(40)
	stream := ape.Stream{SampleRate: 8000, Channels: 1, Bits: 16}
	raw := encodeToBytes(t, pcm, stream, &ape.Options{BlocksPerFrame: 16})

	got, _, err := ape.Decode(bytes.NewBuffer(raw))
	if err != nil {
		t.Fatal(err)
	}

	if !bytes.Equal(pcm, got) {
		t.Fatal("non-seekable decode mismatch")
	}
}

func pcmAlign(stream ape.Stream) int { return stream.Channels * stream.Bits / 8 }

func encodeToBytes(t *testing.T, pcm []byte, stream ape.Stream, opt *ape.Options) []byte {
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

	return raw
}

func encodeChunks(t *testing.T, pcm []byte, stream ape.Stream, opt *ape.Options, chunk int) []byte {
	t.Helper()

	path := filepath.Join(t.TempDir(), "out.ape")

	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}

	enc, err := ape.NewEncoder(file, stream, len(pcm)/pcmAlign(stream), opt)
	if err != nil {
		t.Fatal(err)
	}

	for off := 0; off < len(pcm); off += chunk {
		end := min(off+chunk, len(pcm))

		err = enc.Write(pcm[off:end])
		if err != nil {
			t.Fatal(err)
		}
	}

	err = enc.Close()
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

	return raw
}
