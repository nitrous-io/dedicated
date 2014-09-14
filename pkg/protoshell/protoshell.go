package protoshell

import (
	"crypto/tls"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"syscall"

	"github.com/gorilla/websocket"
	"github.com/nitrous-io/nitrous-common/term"
	"github.com/nitrous-io/nitrous-common/wsconn"
)

const (
	defaultWidth  = 80
	defaultHeight = 25
)

type TokenHandler func() (string, error)

type ProtoShell struct {
	ID              string `json:"id"`
	host            string
	containerID     string
	command         string
	isTerminalOut   bool // this checks if the current shell ouput is a interactive terminal
	isTerminalIn    bool // this checks if the current shell input is a interactive terminal
	tokenHandler    TokenHandler
	tlsClientConfig *tls.Config
	conn            *wsconn.Conn
	netConn         net.Conn

	in  io.Reader
	out io.Writer
}

// New creates a shell that uses WebSockets.
// FIXME Sprinkle TLS magic (i.e. https and wss://).
func New(host, containerID, command string, tlsConfig *tls.Config, tokenHandler TokenHandler) *ProtoShell {
	return &ProtoShell{
		host:            host,
		containerID:     containerID,
		command:         command,
		tokenHandler:    tokenHandler,
		tlsClientConfig: tlsConfig,
	}
}

// connect stdin and stdout respectively to their remote parts
func (s *ProtoShell) Attach(infd uintptr, outfd uintptr, stdin io.ReadCloser, stdout io.Writer) error {
	s.isTerminalIn = term.IsTerminal(infd)
	s.isTerminalOut = term.IsTerminal(outfd)

	if s.isTerminalIn {
		// restore the terminal back to old state when we finish
		var oldState *term.State
		oldState, err := term.SetRawTerminal(infd)
		if err != nil {
			return err
		}
		defer term.RestoreTerminal(infd, oldState)
	}

	// construct token header
	token, err := s.tokenHandler()
	if err != nil {
		return err
	}

	// construct params header that includes the wanted command
	params := url.Values{}
	params.Set("command", s.command)
	params.Set("token", token)
	params.Set("binary", "true")
	wsEndpointURI := fmt.Sprintf("wss://%s/containers/%s/shell?%s", s.host, s.containerID, params.Encode())

	dialer := websocket.Dialer{
		NetDial: func(network, address string) (net.Conn, error) {
			c, err := net.Dial(network, address)
			if err != nil {
				return nil, err
			}
			// associate the underlying network connection with the current
			// shell
			s.netConn = c
			return c, nil
		},
	}
	if s.tlsClientConfig != nil {
		dialer.TLSClientConfig = s.tlsClientConfig
	}
	ws, _, err := dialer.Dial(wsEndpointURI, http.Header{})
	if err != nil {
		return fmt.Errorf("Failed to attach to shell for container %s: %v", s.containerID, err)
	}

	s.conn = wsconn.NewConnection(ws)
	s.conn.MessageType = websocket.BinaryMessage
	defer func() {
		if s.netConn != nil {
			s.netConn.Close()
		}
		if s.conn != nil {
			s.conn.Close()
		}
	}()

	// wrap input/output stream with our protocol
	s.in = NewLocalInput(stdin)
	s.out = NewLocalOutput(stdout)

	finishStdout := promise(func() error {
		defer func() {
			stdin.Close()
		}()
		_, err := io.Copy(s.out, s.conn)
		if err == io.EOF || err == io.ErrUnexpectedEOF {
			return nil
		}
		return err
	})

	finishStdin := promise(func() error {
		io.Copy(s.conn, s.in)
		if tcpc, ok := s.netConn.(*net.TCPConn); ok {
			if err := tcpc.CloseWrite(); err != nil {
				log.Printf("Couldn't send EOF: %s", err)
			}
		} else {
			log.Printf("Unexpected network connection: %s", s.netConn)
		}
		return nil
	})

	// monitor tty size but doesnt fail when it errors because the remote end
	// may have been terminated.
	//
	// We only monitor size when the current shell lives in a terminal
	if s.isTerminalOut {
		s.monitorSize(outfd)
	}

	if err := <-finishStdout; err != nil {
		return err
	}

	// we only want to wait for stdin when the terminal is not in interactive
	// mode
	if !s.isTerminalIn {
		return <-finishStdin
	}
	return nil
}

// Run executes a remote command inside a container in a specific host
func (s *ProtoShell) Run(infd uintptr, outfd uintptr, stdin io.ReadCloser, stdout io.Writer) error {
	if err := s.Attach(infd, outfd, stdin, stdout); err != nil {
		return fmt.Errorf("Shell detached %v", err)
	}
	return nil
}

// monitorSize resizes the local and remote shell so they have the same size
//
// There are cases where the remote-end had beeen terminated when we resizes it
// (because the process finished / terminates early because it was run wrongly),
// so we will just ignore errors here because it's probably the only source
// of error for resizing.
func (s *ProtoShell) monitorSize(fd uintptr) {
	s.resize(fd)
	sigchan := make(chan os.Signal, 1)
	signal.Notify(sigchan, syscall.SIGWINCH)
	go func() {
		for _ = range sigchan {
			s.resize(fd)
		}
	}()
}

// given the current terminal file descriptor, resize the remote terminal to
// match the local terminal size
func (s *ProtoShell) resize(fd uintptr) {
	w, h := getSize(fd)
	msg := fmt.Sprintf("w|%d %d", w, h)
	s.conn.Write([]byte(msg))
}

// get current size of the client terminal
func getSize(fd uintptr) (int, int) {
	ws, err := term.GetWinsize(fd)
	if err != nil {
		return defaultWidth, defaultHeight
	}
	return int(ws.Width), int(ws.Height)
}

// basic promise implementation: it wraps calls a function in a goroutine,
// and returns a channel which will later return the function's return value.
func promise(f func() error) chan error {
	ch := make(chan error, 1)
	go func() {
		ch <- f()
	}()
	return ch
}
