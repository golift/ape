package ape

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

const linkHeader = "[Monkey's Audio Image Link File]"

// DecodeFile reads an APE file, or an APL link that names one.
// An APL link returns only the sample range it points at.
func DecodeFile(path string) ([]byte, Stream, error) {
	return decodeFile(path, map[string]struct{}{})
}

func decodeFile(path string, seen map[string]struct{}) ([]byte, Stream, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, Stream{}, fmt.Errorf("ape: reading file: %w", err)
	}

	abs = filepath.Clean(abs)
	if _, ok := seen[abs]; ok {
		return nil, Stream{}, errLinkCycle
	}

	seen[abs] = struct{}{}

	raw, err := os.ReadFile(path) //nolint:gosec // the path is the file the caller asked to open
	if err != nil {
		return nil, Stream{}, fmt.Errorf("ape: reading file: %w", err)
	}

	image, start, finish, ok := parseLink(raw)
	if !ok {
		return Decode(bytes.NewReader(raw))
	}

	if !filepath.IsAbs(image) {
		image = filepath.Join(filepath.Dir(path), image)
	}

	pcm, stream, err := decodeFile(image, seen)
	if err != nil {
		return nil, Stream{}, err
	}

	pcm, err = sliceLink(pcm, stream.blockAlign(), start, finish)
	if err != nil {
		return nil, Stream{}, err
	}

	return pcm, stream, nil
}

func sliceLink(pcm []byte, align, start, finish int) ([]byte, error) {
	from, ok := linkOffset(start, align, len(pcm))
	if !ok {
		return nil, errLinkStart
	}

	to := len(pcm)
	if finish >= 0 {
		to, ok = linkOffset(finish, align, len(pcm))
		if !ok || finish < start {
			return nil, errLinkEnd
		}
	}

	return pcm[from:to], nil
}

func linkOffset(blocks, align, size int) (int, bool) {
	if blocks < 0 || align <= 0 || blocks > size/align {
		return 0, false
	}

	return blocks * align, true
}

//nolint:nonamedreturns // named returns help here
func parseLink(raw []byte) (image string, start, finish int, ok bool) {
	text := string(raw)
	if !strings.Contains(text, linkHeader) {
		return "", 0, 0, false
	}

	image = linkValue(text, "Image File=")
	startText := linkValue(text, "Start Block=")

	finishText := linkValue(text, "Finish Block=")
	if image == "" || startText == "" || finishText == "" {
		return "", 0, 0, false
	}

	start, err := strconv.Atoi(startText)
	if err != nil {
		return "", 0, 0, false
	}

	finish, err = strconv.Atoi(finishText)
	if err != nil {
		return "", 0, 0, false
	}

	return image, start, finish, true
}

func linkValue(text, key string) string {
	_, after, ok := strings.Cut(text, key)
	if !ok {
		return ""
	}

	rest := after

	rest = strings.TrimRight(rest, "\r")
	if nl := strings.IndexAny(rest, "\r\n"); nl >= 0 {
		rest = rest[:nl]
	}

	return strings.TrimSpace(rest)
}
