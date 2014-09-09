// this handles our websocket protocol
//
// d|action@foobar		# output from server
// p|whoami				# input from client
// d|action				# output from server
// w|80 25				# resize from client
// c|server went away	# close msg
//
// NOT IMPLEMENTED
// h					# heartbeat from client
package protoshell

import (
	"bytes"
	"fmt"
	"io"
)

var (
	serverOutputMsg = []byte("d|")
	clientInputMsg  = []byte("p|")
	resizeMsg       = []byte("w|")
	closeMsg        = []byte("c|")
	heartbeatMsg    = []byte("h")
)

// LocalInput handles client input with p|client input
type LocalInput struct {
	reader io.Reader
}

func NewLocalInput(r io.Reader) io.Reader {
	return &LocalInput{reader: r}
}

func (s *LocalInput) Read(p []byte) (int, error) {
	prefixLen := len(clientInputMsg)
	buf := make([]byte, len(p)-prefixLen)
	n, err := s.reader.Read(buf)
	if err != nil {
		return 0, err
	}
	copy(p, clientInputMsg)
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
		if bytes.Equal(p[0:2], serverOutputMsg) {
			_, err := s.writer.Write(p[2:])
			return l, err
		}
	}
	return 0, fmt.Errorf("invalid server output %s", string(p))
}
