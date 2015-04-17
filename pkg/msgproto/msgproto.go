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
	DataMessage = iota
	CloseMessage
)

var (
	dataMsgPrefix       = []byte("d|")
	closeMsgPrefix      = []byte("c|")
	heartbeatMsg   byte = 'h'
)

type MessageReader interface {
	ReadMessage() (int, []byte, error)
}

// returns type (DataMessage or CloseMessage)
func ReadAndDecodeMessage(ws MessageReader) (int, []byte, error) {
	for {
		_, d, err := ws.ReadMessage()
		if err != nil {
			return 0, nil, err
		}

		n := len(d)
		if n == 1 && d[0] == heartbeatMsg {
			// ignore heartbeat message
			continue
		}
		if n > 2 {
			if bytes.Equal(d[:2], dataMsgPrefix) {
				return DataMessage, d[2:], nil
			}
			if bytes.Equal(d[:2], closeMsgPrefix) {
				return CloseMessage, d[2:], nil
			}
		}
		log.Printf("ReadAndDecodeMessage(): invalid message: %d bytes, '%s'", len(d), string(d))
	}
}

func WriteDataMessage(p []byte, w io.Writer) (int, error) {
	l := len(p)
	buf := make([]byte, l+len(dataMsgPrefix))
	copy(buf[0:2], dataMsgPrefix)
	copy(buf[2:], p)
	return w.Write(buf)
}
