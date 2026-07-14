package audio

import (
	"context"
	"io"
	"time"
)

type pacer struct {
	bytesPerSecond int
	next           time.Time
}

func newPacer(bytesPerSecond int) *pacer {
	return &pacer{bytesPerSecond: bytesPerSecond, next: time.Now()}
}

func (p *pacer) copy(ctx context.Context, dst io.Writer, src io.Reader) error {
	// Create a buffer to hold the data
	buf := make([]byte, pcmChunkSize)
	// Loop until the context is canceled
	for {
		// Check if the context is canceled
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		// Read a chunk of data
		n, err := src.Read(buf)
		// If there is data, write it to the destination
		if n > 0 {
			// Write the data to the buffer
			if _, writeErr := dst.Write(buf[:n]); writeErr != nil {
				// If there is an error, return it
				return writeErr
			}
			// Sleep for the duration of the chunk
			p.sleepFor(n)
		}
		if err != nil {
			if err == io.EOF {
				return nil
			}
			return err
		}
	}
}

func (p *pacer) sleepFor(n int) {
	// Calculate the duration of the chunk
	chunkDuration := time.Duration(float64(n) / float64(p.bytesPerSecond) * float64(time.Second))
	// Sleep until the next chunk
	p.next = p.next.Add(chunkDuration)
	// Sleep for the duration of the chunk
	if sleep := time.Until(p.next); sleep > 0 { // Sleep for the duration of the chunk
		time.Sleep(sleep)
	} else { // If the sleep is negative, reset the next chunk time
		p.next = time.Now()
	}
}
