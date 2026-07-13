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
	return &icyWriter{w: w, getTitle: getTitle, bytesToMeta: icyMetaInt}
}

func (iw *icyWriter) writeMetaBlock(buf *bytes.Buffer) {
	title := iw.getTitle()
	if title == iw.lastTitle {
		buf.WriteByte(0x00)
		return
	}
	iw.lastTitle = title

	tag := "StreamTitle='" + strings.ReplaceAll(title, "'", "") + "';"
	blockLen := (len(tag) + 15) / 16
	buf.WriteByte(byte(blockLen))
	padded := make([]byte, blockLen*16)
	copy(padded, tag)

	buf.Write(padded)
}

func (iw *icyWriter) Write(b []byte) error {
	iw.buf.Reset()
	for len(b) > 0 {
		if iw.bytesToMeta > len(b) {
			iw.buf.Write(b)
			iw.bytesToMeta -= len(b)
			b = nil
			break
		}

		if iw.bytesToMeta > 0 {
			iw.buf.Write(b[:iw.bytesToMeta])
			b = b[iw.bytesToMeta:]
		}

		iw.writeMetaBlock(&iw.buf)
		iw.bytesToMeta = icyMetaInt
	}

	_, err := iw.w.Write(iw.buf.Bytes())
	return err
}
