package metadata

import (
	"bytes"
	"fmt"
	"io"

	"github.com/dhowden/tag"
)

type Track struct {
	Title     string
	Artist    string
	Album     string
	CoverData []byte
	CoverMIME string
}

func (t *Track) StreamTitle() string {
	if t.Artist != "" && t.Artist != "Unknown Artist" {
		return fmt.Sprintf("%s - %s", t.Artist, t.Title)
	}
	return t.Title
}

func Parse(r io.Reader, fallbackTitle string) (*Track, []byte, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, nil, err
	}

	t := &Track{Title: fallbackTitle, Artist: "Unknown Artist"}

	if m, err := tag.ReadFrom(bytes.NewReader(data)); err == nil && m != nil {
		if title := m.Title(); title != "" {
			t.Title = title
		}
		if artist := m.Artist(); artist != "" {
			t.Artist = artist
		}
		t.Album = m.Album()
		if pic := m.Picture(); pic != nil {
			t.CoverData = pic.Data
			t.CoverMIME = pic.MIMEType
		}
	}

	return t, data, nil
}
