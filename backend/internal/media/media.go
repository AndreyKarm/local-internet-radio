package media

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"sync"
	"time"

	"github.com/dhowden/tag"
)

const maxFrameSize = 1441

var framePool = sync.Pool{
	New: func() any {
		return make([]byte, maxFrameSize)
	},
}

// ---------- Metadata ----------

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

	track := &Track{Title: fallbackTitle, Artist: "Unknown Artist"}

	if m, err := tag.ReadFrom(bytes.NewReader(data)); err == nil && m != nil {
		if title := m.Title(); title != "" {
			track.Title = title
		}
		if artist := m.Artist(); artist != "" {
			track.Artist = artist
		}
		track.Album = m.Album()
		if pic := m.Picture(); pic != nil {
			track.CoverData = pic.Data
			track.CoverMIME = pic.MIMEType
		}
	}

	return track, data, nil
}

// ---------- MP3 frame parsing ----------

var ErrNoSync = errors.New("mp3: frame sync not found")

// MPEG-1 Layer III only — that's all our upload-time transcode produces.
var bitrateTableV1L3 = map[int]int{
	1: 32, 2: 40, 3: 48, 4: 56, 5: 64, 6: 80, 7: 96,
	8: 112, 9: 128, 10: 160, 11: 192, 12: 224, 13: 256, 14: 320,
}

var sampleRateTableV1 = map[int]int{0: 44100, 1: 48000, 2: 32000}

type Frame struct {
	Offset     int
	Size       int
	SampleRate int
	Bitrate    int
	Samples    int // always 1152 for MPEG-1 Layer III
}

func (f Frame) Duration() time.Duration {
	if f.SampleRate == 0 {
		return 0
	}
	return time.Duration(float64(f.Samples) / float64(f.SampleRate) * float64(time.Second))
}

func NextFrame(buf []byte, offset int) (Frame, error) {
	for i := offset; i+4 <= len(buf); i++ {
		if buf[i] != 0xFF || (buf[i+1]&0xE0) != 0xE0 {
			continue
		}
		f, ok := parseHeader(buf[i : i+4])
		if !ok {
			continue
		}
		f.Offset = i
		if i+f.Size > len(buf) {
			continue
		}
		return f, nil
	}
	return Frame{}, ErrNoSync
}

func parseHeader(h []byte) (Frame, bool) {
	if (h[1]>>3)&0x03 != 0x03 || (h[1]>>1)&0x03 != 0x01 {
		return Frame{}, false // not MPEG-1 Layer III
	}

	bitrateIdx := int((h[2] >> 4) & 0x0F)
	sampleIdx := int((h[2] >> 2) & 0x03)
	padding := int((h[2] >> 1) & 0x01)

	bitrate, ok := bitrateTableV1L3[bitrateIdx]
	if !ok {
		return Frame{}, false
	}
	sampleRate, ok := sampleRateTableV1[sampleIdx]
	if !ok {
		return Frame{}, false
	}

	return Frame{
		Size:       (144*bitrate*1000)/sampleRate + padding,
		SampleRate: sampleRate,
		Bitrate:    bitrate,
		Samples:    1152,
	}, true
}

// SkipID3v2 returns the offset right after any ID3v2 tag. Embedded cover
// art can otherwise look like a false MPEG frame sync.
func SkipID3v2(data []byte) int {
	if len(data) < 10 || data[0] != 'I' || data[1] != 'D' || data[2] != '3' {
		return 0
	}
	size := int(data[6]&0x7F)<<21 | int(data[7]&0x7F)<<14 |
		int(data[8]&0x7F)<<7 | int(data[9]&0x7F)
	end := 10 + size
	if end > len(data) {
		end = len(data)
	}
	return end
}

func TotalDuration(data []byte) int {
	var total time.Duration
	offset := SkipID3v2(data)
	for {
		frame, err := NextFrame(data, offset)
		if err != nil {
			break
		}
		total += frame.Duration()
		offset = frame.Offset + frame.Size
	}
	return int(total.Seconds())
}

// ---------- Transcode ----------

const (
	SampleRate = 44100
	Channels   = 2
	Bitrate    = "128k"
)

// ToCanonicalMP3 transcodes an uploaded audio file to canonical CBR MP3
// (44.1kHz stereo, 128kbps) exactly once, at upload time.
func ToCanonicalMP3(ctx context.Context, src io.Reader, destPath string) error {
	cmd := exec.CommandContext(ctx, "ffmpeg",
		"-y", "-hide_banner", "-loglevel", "error",
		"-i", "pipe:0",
		"-vn",
		"-ar", fmt.Sprintf("%d", SampleRate),
		"-ac", fmt.Sprintf("%d", Channels),
		"-b:a", Bitrate,
		destPath,
	)
	cmd.Stdin = src

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("ffmpeg: %w: %s", err, stderr.String())
	}
	return nil
}

func ReadID3v2(r *bufio.Reader, fallbackTitle string) (*Track, int, error) {
	track := &Track{Title: fallbackTitle, Artist: "Unknown Artist"}

	hdr, err := r.Peek(10)
	if err != nil {
		return track, 0, nil // not even 10 bytes / no tag
	}
	if hdr[0] != 'I' || hdr[1] != 'D' || hdr[2] != '3' {
		return track, 0, nil
	}

	size := int(hdr[6]&0x7F)<<21 | int(hdr[7]&0x7F)<<14 |
		int(hdr[8]&0x7F)<<7 | int(hdr[9]&0x7F)
	tagLen := 10 + size

	tagBytes := make([]byte, tagLen)
	if _, err := io.ReadFull(r, tagBytes); err != nil {
		return nil, 0, err
	}

	if m, err := tag.ReadFrom(bytes.NewReader(tagBytes)); err == nil && m != nil {
		if title := m.Title(); title != "" {
			track.Title = title
		}
		if artist := m.Artist(); artist != "" {
			track.Artist = artist
		}
		track.Album = m.Album()
		if pic := m.Picture(); pic != nil {
			track.CoverData = pic.Data
			track.CoverMIME = pic.MIMEType
		}
	}
	return track, tagLen, nil
}

// ReadFrame reads one MPEG-1 Layer III frame (including its 4-byte header)
// from r, returning the parsed header and the raw frame bytes.
func ReadFrame(r *bufio.Reader) (Frame, []byte, error) {
	for {
		var hdr [4]byte
		if _, err := io.ReadFull(r, hdr[:1]); err != nil {
			return Frame{}, nil, err
		}
		if hdr[0] != 0xFF {
			continue
		}
		if _, err := io.ReadFull(r, hdr[1:]); err != nil {
			return Frame{}, nil, err
		}

		f, ok := parseHeader(hdr[:4])
		if !ok || f.Size < 20 || f.Size > maxFrameSize {
			continue
		}

		buf := framePool.Get().([]byte)
		if cap(buf) < f.Size {
			buf = make([]byte, f.Size)
		}
		buf = buf[:f.Size]
		copy(buf, hdr[:4])
		if _, err := io.ReadFull(r, buf[4:]); err != nil {
			return Frame{}, nil, err
		}
		return f, buf, nil
	}
}

// EstimateDuration computes playback seconds for our canonical 128 kbps CBR
// stream, avoiding the old full-file frame walk. 128 kbps = 16000 bytes/s.
func EstimateDuration(audioBytes int64) int {
	return int(audioBytes / 16000)
}

func ReleaseFrame(buf []byte) {
	framePool.Put(buf[:cap(buf)])
}
