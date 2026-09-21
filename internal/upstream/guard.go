package upstream

import (
	"bytes"
	"encoding/json"
	"io"
	"sync"
)

// v7 can synthesize STOP on a clean Antigravity EOF after discarding a native error frame.
// Validate wire errors and actual finish reasons before SDK translation. Keep this guard until
// the executor guarantees those semantics itself; translated terminal events are not proof of success.
func guardGeminiStream(source io.ReadCloser) io.ReadCloser {
	reader, writer := io.Pipe()
	body := &guardedBody{PipeReader: reader, source: source, done: make(chan struct{})}
	go func() {
		defer close(body.done)
		defer source.Close()
		err := readGeminiStream(source, func(raw []byte) error {
			var compact bytes.Buffer
			if json.Compact(&compact, raw) != nil {
				return ErrResponse
			}
			if _, err := writer.Write(append(append([]byte("data: "), compact.Bytes()...), []byte("\n\n")...)); err != nil {
				return err
			}
			return nil
		}, true)
		writer.CloseWithError(err)
	}()
	return body
}

type guardedBody struct {
	*io.PipeReader
	source io.ReadCloser
	done   chan struct{}
	once   sync.Once
}

func (b *guardedBody) Close() error {
	b.once.Do(func() { b.PipeReader.Close(); b.source.Close(); <-b.done })
	return nil
}
