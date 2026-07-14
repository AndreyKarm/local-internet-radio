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
	// Create a new ffmpeg process
	cmd := exec.CommandContext(ctx, "ffmpeg",
		"-hide_banner", "-loglevel", "error",
		"-f", "mp3", "-i", "pipe:0",
		"-vn", "-f", "s16le", "-ar", strconv.Itoa(sampleRate), "-ac", strconv.Itoa(channels), "pipe:1",
	)
	// Set the stdin of the command to the data
	cmd.Stdin = bytes.NewReader(data)
	cmd.Stderr = os.Stderr

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, nil, err
	}

	// Wait for the process to finish
	return stdout, func() { _ = cmd.Wait() }, nil
}

func probeDuration(data []byte) int {
	// Create a new decoder
	decoder := mp3.NewDecoder(bytes.NewReader(data))
	var frame mp3.Frame
	var skipped int
	var total time.Duration

	for {
		// Decode the next frame
		if err := decoder.Decode(&frame, &skipped); err != nil {
			break
		}
		// Add the duration of the frame to the total
		total += frame.Duration()
	}

	// Return the total duration in seconds
	return int(total.Seconds())
}
