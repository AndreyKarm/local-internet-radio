package broadcaster

import (
	"bytes"
	"net/http"
	"strings"
)

const icyMetaInt = 16000

type icyWriter struct {
	w           http.ResponseWriter
	getTitle    func() string
	bytesToMeta int
	lastTitle   string
	buf         bytes.Buffer
}

type icyInjector interface {
	Write(p []byte) error
}

type plainWriter struct{ w http.ResponseWriter }

func newICYWriter(w http.ResponseWriter, getTitle func() string) *icyWriter {
	// Create a new buffer for the metadata blocks and set the initial bytes to metadata
	return &icyWriter{w: w, getTitle: getTitle, bytesToMeta: icyMetaInt}
}

func (iw *icyWriter) writeMetaBlock(buf *bytes.Buffer) {
	// Get the title
	title := iw.getTitle()
	// If the title is the same as the last title, write a zero byte
	if title == iw.lastTitle {
		buf.WriteByte(0x00)
		return
	}
	// Write the title
	iw.lastTitle = title

	// Write the block length
	tag := "StreamTitle='" + strings.ReplaceAll(title, "'", "") + "';"
	blockLen := (len(tag) + 15) / 16 // Round up to the nearest 16 bytes
	// Write the block length
	buf.WriteByte(byte(blockLen))
	// Write the tag
	padded := make([]byte, blockLen*16)
	// Pad the tag with zero bytes
	copy(padded, tag)

	// Write the padding
	buf.Write(padded)
}

func (iw *icyWriter) Write(b []byte) error {
	iw.buf.Reset()
	// If there is no data, return
	for len(b) > 0 {
		// If there is no more data to write, write the metadata block
		if iw.bytesToMeta > len(b) {
			iw.buf.Write(b)
			iw.bytesToMeta -= len(b)
			b = nil
			break
		}

		// Write the data to the buffer
		if iw.bytesToMeta > 0 {
			iw.buf.Write(b[:iw.bytesToMeta])
			b = b[iw.bytesToMeta:]
		}

		// Write the metadata block
		iw.writeMetaBlock(&iw.buf)
		iw.bytesToMeta = icyMetaInt
	}

	_, err := iw.w.Write(iw.buf.Bytes())
	return err
}
