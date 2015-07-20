package hostbuilder

import (
	"encoding/json"
	"fmt"
	"io"
)

// ProgressWriter writes stream data emitted by hostbuilder as newline-separated
// JSON messages.
type ProgressWriter struct {
	writer io.Writer
}

func NewProgressWriter(w io.Writer) *ProgressWriter {
	return &ProgressWriter{w}
}

func (w *ProgressWriter) Write(p []byte) (int, error) {
	err := w.WriteResponse(&Response{Stream: string(p)})
	return len(p), err
}

func (w *ProgressWriter) WriteError(err error) error {
	return w.WriteResponse(&Response{Error: err.Error()})
}

func (w *ProgressWriter) WriteResponse(r *Response) error {
	b, err := json.Marshal(r)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(w.writer, "%s\n", string(b))
	return err
}
