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

func (track *Track) StreamTitle() string {
	if track.Artist != "" && track.Artist != "Unknown Artist" {
		return fmt.Sprintf("%s - %s", track.Artist, track.Title)
	}
	return track.Title
}

func Parse(r io.Reader, fallbackTitle string) (*Track, []byte, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, nil, err
	}

	track := &Track{Title: fallbackTitle, Artist: "Unknown Artist"}

	if metadata, err := tag.ReadFrom(bytes.NewReader(data)); err == nil && metadata != nil {
		if title := metadata.Title(); title != "" {
			track.Title = title
		}
		if artist := metadata.Artist(); artist != "" {
			track.Artist = artist
		}
		track.Album = metadata.Album()
		if pic := metadata.Picture(); pic != nil {
			track.CoverData = pic.Data
			track.CoverMIME = pic.MIMEType
		}
	}

	return track, data, nil
}
