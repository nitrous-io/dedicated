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

	FileInfo struct {
		CID   string
		Name  string
		IsDir bool
	}

	AsyncHandler interface {
		HandleAsync(msg *Message) chan *Message
	}
)
