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

	pcm, stream, err := DecodeFile(image)
	if err != nil {
		return nil, Stream{}, err
	}

	align := stream.blockAlign()
	if start < 0 || start*align > len(pcm) {
		return nil, Stream{}, errLinkStart
	}

	from := start * align

	to := len(pcm)
	if finish >= 0 {
		if finish < start || finish*align > len(pcm) {
			return nil, Stream{}, errLinkEnd
		}

		to = finish * align
	}

	return pcm[from:to], stream, nil
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
