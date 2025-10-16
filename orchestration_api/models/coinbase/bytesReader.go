package coinbase

import (
	"io"
)

// helper to avoid importing bytes directly elsewhere
func BytesReader(b []byte) *bytesReaderWrapper { return &bytesReaderWrapper{b: b} }

type bytesReaderWrapper struct{ b []byte }

func (w *bytesReaderWrapper) Read(p []byte) (int, error) {
	if len(w.b) == 0 {
		return 0, io.EOF
	}
	n := copy(p, w.b)
	w.b = w.b[n:]
	return n, nil
}

func (w *bytesReaderWrapper) Close() error { return nil }
