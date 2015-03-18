/*
server <--> client

The message is json-encoded with type

--> d|{type: "fs.list", cid: callbackID, args: ["path"]} // list a directory
<-- d|{type: "fs.list", cid: callbackID, args: ["dir1", "dir2]} // reply a message with some response
<-- c|we will be back // close message
--> h // heartbeat message

NOTE: this is pretty similar to the shell protocol. Maybe we could refactor
something here.
*/
package msgproto

import (
	"bytes"
	"io"
	"log"
)

const (
	DataMessage  = 0
	CloseMessage = 1
)

var (
	dataMsg      = []byte("d|")
	closeMsg     = []byte("c|")
	heartbeatMsg = []byte("h")
)

type messageReader interface {
	ReadMessage() (int, []byte, error)
}

// returns type (DataMessage or CloseMessage)
func ReadAndDecodeMessage(ws messageReader) (int, []byte, error) {
	for {
		_, d, err := ws.ReadMessage()
		if err != nil {
			return 0, nil, err
		}

		n := len(d)
		if n == 1 && d[0] == 'h' {
			// ignore heartbeat message
			continue
		}
		if n > 2 {
			if bytes.Equal(d[:2], dataMsg) {
				return DataMessage, d[2:], nil
			}
			if bytes.Equal(d[:2], closeMsg) {
				return CloseMessage, d[2:], nil
			}
		}
		log.Printf("ReadAndDecodeMessage(): invalid message: %d bytes, '%s'", len(d), string(d))
	}
}

func WriteDataMessage(p []byte, w io.Writer) (int, error) {
	l := len(p)
	buf := make([]byte, l+len(dataMsg))
	copy(buf[0:2], dataMsg)
	copy(buf[2:], p)
	return w.Write(buf)
}
