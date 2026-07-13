package audio

import (
	"bytes"
	"context"
	"io"
	"os"
	"os/exec"
	"strconv"
	"time"

	"github.com/tcolgate/mp3"
)

func decodeMP3(ctx context.Context, data []byte) (io.Reader, func(), error) {
	cmd := exec.CommandContext(ctx, "ffmpeg",
		"-hide_banner", "-loglevel", "error",
		"-f", "mp3", "-i", "pipe:0",
		"-vn", "-f", "s16le", "-ar", strconv.Itoa(sampleRate), "-ac", strconv.Itoa(channels), "pipe:1",
	)
	cmd.Stdin = bytes.NewReader(data)
	cmd.Stderr = os.Stderr

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, nil, err
	}

	return stdout, func() { _ = cmd.Wait() }, nil
}

func probeDuration(data []byte) int {
	decoder := mp3.NewDecoder(bytes.NewReader(data))
	var frame mp3.Frame
	var skipped int
	var total time.Duration

	for {
		if err := decoder.Decode(&frame, &skipped); err != nil {
			break
		}
		total += frame.Duration()
	}

	return int(total.Seconds())
}
