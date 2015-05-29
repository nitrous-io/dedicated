package queue

import (
	"encoding/json"
	"fmt"
	"time"
)

type Handler func(*Message) error

// Publisher represents a client that publishes a message (Publishing)
// into a queue.
type Publisher interface {
	// Publish publishes a message to the queue.
	Publish(msg *Message) error
	// PublishRetry publishes a message to the queue. If it fails to publish
	// the message, it will retry for the configurable number of times, sleep
	// for a configurable duration in between.
	PublishRetry(msg *Message, tries int, retryInterval time.Duration) error
	// Delete removes all messages from the queues. It will not be deleted if
	// there are any consumers on the queue
	Delete() error
	// ResetInput removes all messages from the input queue.
	ResetInput() error
	// GetInputLength returns the number of messages currently in the input
	// queue
	GetInputLength() int64
	// GetFailedLength returns the number of messages that has been marked as
	// Failed
	GetFailedLength() int64
}

// Delivery represents actions that can be acted on a message that is received
// from the queue
type Delivery interface {
	// Ack acknowledges the delivery of the current message. When Ack fails to
	// send the message, it will wait for a configurable interval before
	// retrying. Ack will not stop until it successfully sends the message to
	// prevent data inconsistency problem.
	Ack(t time.Duration) error
	// Fail push failed message to failed queue. Implementation of a failed
	// queue can differ. For example AMQP uses a dead-letter exchange, while
	// Redis use a list
	Fail()
}

// Consumer represents a client that consumes messages from a queue
type Consumer interface {
	Consume() (*Message, Delivery, error)
	SetHandler(h Handler)
	Start(stop chan struct{}) error
}

// Message satifies both Publishing and Delivering type, make it suitable to be
// sent/received across publishers and consumers
type Message struct {
	UUID         string `json: "uuid"`
	Action       string `json: "action"`
	Event        string `json: "event"`
	ResourceName string `json: "resource_name"`
	ResourceID   string `json: "resource_id"`
	Payload      []byte `json: "payload"`
}

// MarshalJSON encodes the message to JSON format
func (m *Message) Encode() ([]byte, error) {
	return json.Marshal(m)
}

// UnmarshalJSON decodes the JSON data into current message
func (m *Message) Decode(p []byte) error {
	return json.Unmarshal(p, m)
}

// String returns a string representation of current message, suitable for
// debugging purposes.
func (m *Message) String() string {
	return fmt.Sprintf("uuid=%s event=%s", m.UUID, m.Event)
}

var actions = []string{
	"create",
	"update",
	"delete",
}

type Action int

const (
	Create Action = iota
	Update
	Delete
)

func (a Action) String() string {
	return actions[a]
}

func (a Action) IsA(actionName string) bool {
	return a.String() == actionName
}
