package ape

import (
	"bytes"
	"os"
	"os/exec"
	"testing"
)

func TestBlackbirdFrame(t *testing.T) {
	t.Parallel()

	path := "/Volumes/Storage/david/Downloads/b/Blackbird (2007)/Alter Bridge - Blackbird.ape"

	raw, err := os.ReadFile(path)
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

	cmd := exec.Command("ffmpeg", "-v", "error", "-i", path, "-t", "6.68734693877551", "-f", "s16le", "-")

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
