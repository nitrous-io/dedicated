package queue

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"strings"
	"time"

	"github.com/adjust/redismq"
)

// Publisher represents a client that publishes a message (Publishing)
// into a queue.
type Publisher interface {
	// Publish publishes a message to the queue.
	Publish(msg Publishing) error
	// PublishRetry publishes a message to the queue. If it fails to publish
	// the message, it will retry for the configurable number of times, sleep
	// for a configurable duration in between.
	PublishRetry(msg Publishing, tries int, retryInterval time.Duration) error
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

// Publishing represents a message that can be sent to a queue.
type Publishing interface {
	// Encode encodes the message into valid JSON.
	Encode() ([]byte, error)
}

// Delivery represents a message that is received from the queue
type Delivery interface {
	// Decode decodes the message from valid JSON.
	Decode([]byte) error
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
	Consume() (Delivery, error)
}

type QueuePublisher struct {
	Queue *redismq.Queue
}

type QueueConsumer struct {
	Consumer *redismq.Consumer
}

var (
	ErrUnexpectedPublishing = errors.New("unexpected publishing format")
	ErrUnexpectedDelivery   = errors.New("unexpected delivery format")
)

// Message satifies both Publishing and Delivering type, make it suitable to be
// sent/received across publishers and consumers
type Message struct {
	UUID         string `json: "uuid"`
	Action       string `json: "action"`
	Event        string `json: "event"`
	ResourceName string `json: "resource_name"`
	ResourceID   string `json: "resource_id"`
	Payload      []byte `json: "payload"`
	p            *redismq.Package
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

// Ack send ack packet to consume next message
// it sleeps for `retryInterval` duration when it fails to send it
// it will not stop retrying because it could occur data inconsistency problem
func (m *Message) Ack(retryInterval time.Duration) error {
	for {
		err := m.p.Ack()
		if err != nil {
			// if we failed to connect to redis or received EOF error then retry
			if _, ok := err.(*net.OpError); ok || err == io.EOF {
				log.Printf("uuid=%s Retrying ack for message(%s) : %v\n", m.UUID, m.Event, err)
				time.Sleep(retryInterval)
				continue
			}

			// redis emits `open connections too fast` when we exceeds the error limit
			if strings.Contains(err.Error(), "open connections too fast") {
				log.Printf("uuid=%s Retrying ack for message(%s) : %v\n", m.UUID, m.Event, err)
				time.Sleep(1 * time.Second)
				continue
			}
			return err
		}
		return nil
	}
	return fmt.Errorf("timed out sending message %+v", m)
}

// Fail push failed message to failed queue
// It will not retry since we can submit a message manually
func (m *Message) Fail() {
	err := m.p.Fail()
	if err != nil {
		log.Printf("[ERROR] uuid=%s Failed to push a message(%s) to failed queue: %v\n", m.UUID, m.Payload, err)
	}
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

type queueHandler func(Delivery) error

var queueRoutes map[string]queueHandler

func init() {
	queueRoutes = map[string]queueHandler{}
}

func NewQueue(redisUrl string, redisPort string, redisDatabase int64, name string) Publisher {
	return &QueuePublisher{Queue: redismq.CreateQueue(redisUrl, redisPort, "", redisDatabase, name)}
}

// Publish pushes the message into the queue without retrying if that fails
func (qp *QueuePublisher) Publish(msg Publishing) error {
	return qp.PublishRetry(msg, 1, 0)
}

// PublishRetry publishes the message at most `tries` times. When it fails to publish because
// redis is not available, it sleeps for `retryInterval` duration.
// When tries is -1, it keeps trying until it successes to publish to redis
func (qp *QueuePublisher) PublishRetry(msg Publishing, tries int, retryInterval time.Duration) error {
	m, ok := msg.(*Message)
	if !ok {
		return ErrUnexpectedPublishing
	}
	b, err := msg.Encode()
	if err != nil {
		return err
	}
	str := string(b)
	forever := tries == -1
	for forever || tries >= 0 {
		err := qp.Queue.Put(str)
		if err != nil {
			// if we failed to connect to redis or received EOF error then retry
			if _, ok := err.(*net.OpError); ok || err == io.EOF {
				log.Printf("uuid=%s Retrying event %s in (%s) queue: %v\n", m.UUID, m.Event, qp.Queue.Name, err)
				tries--
				time.Sleep(retryInterval)
				continue
			}
			// redis emits `open connections too fast` when we exceeds the error limit
			if strings.Contains(err.Error(), "open connections too fast") {
				log.Printf("uuid=%s Retrying event %s in (%s) queue: %v\n", m.UUID, m.Event, qp.Queue.Name, err)
				time.Sleep(1 * time.Second)
				continue
			}
			return err
		}
		return nil
	}
	return fmt.Errorf("timed out sending message %+v", msg)
}

// Delete clears all input and failed queues as well as all consumers. It will
// not proceed as long as consumers are running
func (qp *QueuePublisher) Delete() error {
	return qp.Queue.Delete()
}

// ResetInput removes all messages from the input queue
func (qp *QueuePublisher) ResetInput() error {
	return qp.Queue.ResetInput()
}

// GetInputLength returns the number of pending messages in the input queue
func (qp *QueuePublisher) GetInputLength() int64 {
	return qp.Queue.GetInputLength()
}

// GetInputLength returns the number of messages in the failed queue
func (qp *QueuePublisher) GetFailedLength() int64 {
	return qp.Queue.GetFailedLength()
}

func (qc *QueueConsumer) Consume() (Delivery, error) {
	p, err := qc.Consumer.Get()
	if err != nil {
		if qc.Consumer.HasUnacked() {
			log.Println("pushed the message to working queue even if it failes to get a message. Requeue again to process")
			time.Sleep(1 * time.Second)
			qc.Consumer.RequeueWorking()
		}
		return nil, err
	}

	m := &Message{}
	if err := m.Decode([]byte(p.Payload)); err != nil {
		return nil, err
	}
	m.p = p
	return m, nil
}

func (qc *QueueConsumer) Start(stop chan struct{}) error {
	queueName := qc.Consumer.Name
	log.Printf("started queue consumer %s", queueName)

	msg := make(chan *Message)
	go func() {
		var last_error error
		for {
			delivery, err := qc.Consume()
			if err != nil {
				log.Printf("[ERROR] Failed to consume a message from (%s) queue: %+v\n", queueName, err)
				// redis emits `open connections too fast` when we exceeds the error limit
				if err == io.EOF || strings.Contains(err.Error(), "open connections too fast") {
					time.Sleep(1 * time.Second)
				} else {
					if last_error == err {
						time.Sleep(2 * time.Second)
					} else {
						time.Sleep(50 * time.Millisecond)
					}
				}

				last_error = err
				continue
			}
			m, ok := delivery.(*Message)
			if !ok {
				log.Printf("[ERROR] Unknown delivery %#v", delivery)
				continue
			}
			last_error = nil
			msg <- m
			<-msg
		}
	}()

	for {
		select {
		case c := <-stop:
			log.Printf("stopped queue consumer %s", queueName)
			stop <- c
			return nil
		case m := <-msg:
			err := queueRoutes[queueName](m)
			if err != nil {
				log.Printf("[ERROR] uuid=%s Failed to process (%+v) message in (%s) queue: %v\n", m.UUID, m, queueName, err)
				m.Fail()
			} else {
				err := m.Ack(1 * time.Second)
				if err != nil {
					log.Printf("[ERROR] uuid=%s Failed to send ACK for (%+v) message in (%s) queue: %v\n", m.UUID, m, queueName, err)
				}
			}
			msg <- m
		}
	}

	return nil
}

func StartConsumer(redisHost, redisPort string, redisDB int64, queueName string, handler queueHandler, stop chan struct{}) error {
	var qc *QueueConsumer
	var err error

	for {
		select {
		case c := <-stop:
			log.Printf("shutting down before initialize queue consumer: %s", queueName)
			stop <- c
			return nil
		default:
			qc, err = CreateConsumer(redisHost, redisPort, redisDB, queueName)
			if err == nil {
				queueRoutes[queueName] = handler
				return qc.Start(stop)
			}

			if !strings.Contains(err.Error(), "consumer with this name is already active") {
				log.Fatalf("[ERROR] Failed to start queue consumer: %v\n", err)
			}

			log.Printf("Waiting for previous queue comsumer to be down")
			time.Sleep(100 * time.Millisecond)
		}
	}
}

func CreateConsumer(redisHost, redisPort string, redisDB int64, queueName string) (*QueueConsumer, error) {
	queue := redismq.CreateQueue(redisHost, redisPort, "", int64(redisDB), queueName)
	c, err := queue.AddConsumer(queueName)
	if err != nil {
		return nil, fmt.Errorf("Failed to add a consumer: %v\n", err)
	}

	err = c.RequeueWorking() // Requeue all messages that does not send ACK
	if err != nil {
		return nil, fmt.Errorf("Failed to requeue not finished job: %v\n", err)
	}

	return &QueueConsumer{Consumer: c}, nil
}
