package cops

/*
__attribute__((constructor)) init() {
	cops_start();
}
*/
import "C"

type (
	// Message will be serialized over the transport. It is used to represent both
	// the operation and its result.
	Message struct {
		Namespace string        `json:"namespace"`
		Event     string        `json:"event"`
		CID       int64         `json:"cid"`
		Args      []interface{} `json:"args"`
		Error     string        `json:"error"`
	}

	// Handler is the actual operation that operates on the container `id`.
	// TODO: deprecate this interface and upgrade implementation to AsyncHandler
	// e.g. cops/fs
	Handler interface {
		Handle(msg *Message) *Message
	}

	FileInfo struct {
		CID   string
		Name  string
		IsDir bool
	}

	// this will be used when handler supports this interface
	AsyncHandler interface {
		HandleAsync(msg *Message) chan *Message
	}

	NewHandler func(id string) (Handler, error)
)

var (
	Handlers = map[string]NewHandler{}
)

func RegisterHandler(namespace string, h NewHandler) {
	Handlers[namespace] = h
}
