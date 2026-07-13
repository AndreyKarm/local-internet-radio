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
	buf := make([]byte, pcmChunkSize)
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		n, err := src.Read(buf)
		if n > 0 {
			if _, writeErr := dst.Write(buf[:n]); writeErr != nil {
				return writeErr
			}
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
	chunkDuration := time.Duration(float64(n) / float64(p.bytesPerSecond) * float64(time.Second))
	p.next = p.next.Add(chunkDuration)
	if sleep := time.Until(p.next); sleep > 0 {
		time.Sleep(sleep)
	} else {
		p.next = time.Now()
	}
}
