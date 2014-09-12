// server <--> client
// --> d|action@foobar $ # output from server
// <-- d|whoami          # input from client
// --> d|action          # output from server
// <-- r|80 25           # resize from client
// <-- c|good bye        # close msg
//
// NOT IMPLEMENTED
// <-- h                  # heartbeat from client
package protoshell

import (
	"bytes"
	"io"
)

var (
	dataMsg      = []byte("d|")
	resizeMsg    = []byte("r|")
	closeMsg     = []byte("c|")
	heartbeatMsg = []byte("h")
)

// LocalInput handles client input with p|client input
type LocalInput struct {
	reader io.Reader
}

func NewLocalInput(r io.Reader) io.Reader {
	return &LocalInput{reader: r}
}

func (s *LocalInput) Read(p []byte) (int, error) {
	prefixLen := len(dataMsg)
	buf := make([]byte, len(p)-prefixLen)
	n, err := s.reader.Read(buf)
	if err != nil {
		return 0, err
	}
	copy(p, dataMsg)
	copy(p[prefixLen:], buf[0:n])
	return prefixLen + n, nil
}

// LocalOutput handles client write which outputs p|server_output
type LocalOutput struct {
	writer io.Writer
}

func NewLocalOutput(w io.Writer) io.Writer {
	return &LocalOutput{writer: w}
}

func (s *LocalOutput) Write(p []byte) (int, error) {
	l := len(p)
	// ignore heartbeat message
	if l == 1 && bytes.Equal(p[0:1], heartbeatMsg) {
		return l, nil
	}
	if l >= 2 {
		// detect server output, copy everything back to p without the prefix
		if bytes.Equal(p[0:2], dataMsg) {
			n, err := s.writer.Write(p[2:])
			return n + 2, err
		}
		// ignore resize
		if bytes.Equal(p[0:2], resizeMsg) {
			return l, nil
		}
		// ignore close
		if bytes.Equal(p[0:2], closeMsg) {
			return l, nil
		}
	}
	// otherwise, append the message from server as the message can be
	// fragmented so we cant ignore it
	return s.writer.Write(p)
}
