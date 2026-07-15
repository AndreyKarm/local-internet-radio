package audio

import (
	"bytes"
	"context"
	"io"
	"log"
	"os"
	"os/exec"
	"strconv"
	"strings"
)

func decodeMP3(ctx context.Context, data []byte) (io.Reader, func(), error) {
	// Create a new ffmpeg process
	cmd := exec.CommandContext(ctx, "ffmpeg",
		"-hide_banner", "-loglevel", "error",
		"-i", "pipe:0",
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
	cmd := exec.Command("ffprobe",
		"-v", "error",
		"-show_entries", "format=duration",
		"-of", "default=noprint_wrappers=1:nokey=1",
		"-",
	)

	cmd.Stdin = bytes.NewReader(data)
	out, err := cmd.Output()
	if err != nil {
		return 0
	}

	// Parse the output (e.g., "234.567") into a float, then cast to int seconds
	durationFloat, err := strconv.ParseFloat(strings.TrimSpace(string(out)), 64)
	if err != nil {
		return 0
	}

	log.Printf("duration: %f", durationFloat)

	return int(durationFloat)
}
