package cops

import (
	"encoding/json"
	"fmt"
	"io"
)

// Message will be serialized over the transport. It is used to represent both
// the operation and its result.
type Message struct {
	Namespace string        `json:"namespace"`
	Event     string        `json:"event"`
	CID       int64         `json:"cid"`
	Args      []interface{} `json:"args"`
	Error     error         `json:"error"`
}

// Handler is the actual operation that operates on the container `id`.
// TODO: deprecate this interface and upgrade implementation to AsyncHandler
// e.g. cops/fs
type Handler interface {
	Handle(msg *Message) *Message
}

// this will be used when handler supports this interface
type AsyncHandler interface {
	HandleAsync(msg *Message) chan *Message
}

type NewHandler func(id string) (Handler, error)

var Handlers = map[string]NewHandler{}

// Start listening to messages meant for container with `id` and deliver them to
// correct handlers.
func Start(id string, r io.Reader, w io.Writer) chan error {
	decoder := json.NewDecoder(r)
	encoder := json.NewEncoder(w)
	errCh := make(chan error, 1)

	// collection of handlers for this container.
	handlers := map[string]Handler{}

	go func() {
		for {
			msg := &Message{}
			if err := decoder.Decode(msg); err != nil {
				errCh <- fmt.Errorf("failed to decode msg %s", err)
				return
			}

			// get the handler for this namespace, init and cache it if
			// necessary
			h, ok := handlers[msg.Namespace]
			if !ok {
				var err error
				newHandler, ok := Handlers[msg.Namespace]
				if !ok {
					errCh <- fmt.Errorf("invalid operation %s", msg.Namespace)
					return
				}

				h, err = newHandler(id)
				if err != nil {
					errCh <- fmt.Errorf("failed to handle %s for container %s, %s", msg.Namespace, id, err)
					return
				}
				handlers[msg.Namespace] = h
			}

			// handle the operation in a separate goroutine to not block the
			// message pipeline
			go func() {
				uh, ok := h.(AsyncHandler)
				if ok {
					ch := uh.HandleAsync(msg)
					for reply := range ch {
						if err := encoder.Encode(reply); err != nil {
							errCh <- fmt.Errorf("failed to encode %v, %s", reply, err)
							return
						}
					}
				} else {
					reply := h.Handle(msg)
					if err := encoder.Encode(reply); err != nil {
						errCh <- fmt.Errorf("failed to encode %v, %s", reply, err)
						return
					}
				}
			}()
		}
	}()

	return errCh
}

func RegisterHandler(namespace string, h NewHandler) {
	Handlers[namespace] = h
}
