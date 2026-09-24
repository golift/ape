package ape

import (
	"bytes"
	"encoding/binary"
	"os"
	"os/exec"
	"testing"
)

func TestParseOldRejects(t *testing.T) {
	t.Parallel()

	base := make([]byte, 32+8)
	copy(base, "MAC ")
	binary.LittleEndian.PutUint16(base[4:], 3970)
	binary.LittleEndian.PutUint16(base[6:], uint16(CompressionHigh))
	binary.LittleEndian.PutUint16(base[8:], flagCreateWAV)
	binary.LittleEndian.PutUint16(base[10:], 2)
	binary.LittleEndian.PutUint32(base[12:], 44100)
	binary.LittleEndian.PutUint32(base[24:], 1)
	binary.LittleEndian.PutUint32(base[28:], 100)
	binary.LittleEndian.PutUint32(base[32:], 0)

	cases := []struct {
		name string
		edit func([]byte)
	}{
		{name: "level", edit: func(b []byte) { binary.LittleEndian.PutUint16(b[6:], 1500) }},
		{name: "rate", edit: func(b []byte) { binary.LittleEndian.PutUint32(b[12:], 0) }},
		{name: "final", edit: func(b []byte) { binary.LittleEndian.PutUint32(b[28:], 1<<30) }},
	}

	for _, tc := range cases {
		raw := append([]byte(nil), base...)
		tc.edit(raw)

		_, err := parseOld(raw, 3970)
		if err == nil {
			t.Fatalf("%s accepted", tc.name)
		}
	}
}

func TestBlackbirdFrame(t *testing.T) {
	t.Parallel()

	path := os.Getenv("APE_TEST_FILE")
	if path == "" {
		t.Skip("APE_TEST_FILE is unset")
	}

	raw, err := os.ReadFile(path) //nolint:gosec // optional local fixture from APE_TEST_FILE
	if err != nil {
		t.Skip(err)
	}

	file, err := parseFile(raw)
	if err != nil {
		t.Fatal(err)
	}

	if file.version != 3970 || file.stream.Channels != 2 || file.stream.Bits != 16 {
		t.Fatalf("header version %d stream %+v", file.version, file.stream)
	}

	frame, skip := file.frame(0)

	got, err := decodeFrame(frame, skip, file.blocks(0), file.stream, file.level, file.version)
	if err != nil {
		t.Fatal(err)
	}

	cmd := exec.Command("ffmpeg", "-v", "error", "-i", path, "-t", "6.68734693877551", "-f", "s16le", "-") //nolint:gosec // path is the same optional fixture

	want, err := cmd.Output()
	if err != nil {
		t.Fatal(err)
	}

	if len(want) < len(got) || !bytes.Equal(got, want[:len(got)]) {
		limit := min(len(got), len(want))
		for i := range limit {
			if got[i] != want[i] {
				t.Fatalf("mismatch at %d got %x want %x", i, got[i:min(i+16, limit)], want[i:min(i+16, limit)])
			}
		}

		t.Fatalf("length %d vs %d", len(got), len(want))
	}
}
