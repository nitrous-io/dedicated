package queue

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"strings"
	"time"

	"github.com/adjust/redismq"
)

type QueuePublisher struct {
	*redismq.Queue
}

type QueueConsumer struct {
	*redismq.Consumer
}

type Message struct {
	UUID         string `json: "uuid"`
	Action       string `json: "action"`
	Event        string `json: "event"`
	ResourceName string `json: "resource_name"`
	ResourceID   string `json: "resource_id"`
	Payload      []byte `json: "payload"`
	p            *redismq.Package
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

type queueHandler func(*Message) error

var queueRoutes map[string]queueHandler

func init() {
	queueRoutes = map[string]queueHandler{}
}

func NewQueue(redisUrl string, redisPort string, redisDatabase int64, name string) *QueuePublisher {
	return &QueuePublisher{Queue: redismq.CreateQueue(redisUrl, redisPort, "", redisDatabase, name)}
}

// Publish pushes the message into the queue without retrying if that fails
func (qp *QueuePublisher) Publish(msg *Message) error {
	return qp.PublishRetry(msg, 1, 0)
}

// PublishRetry publishes the message at most `tries` times. When it fails to publish because
// redis is not available, it sleeps for `retryInterval` duration.
// When tries is -1, it keeps trying until it successes to publish to redis
func (qp *QueuePublisher) PublishRetry(msg *Message, tries int, retryInterval time.Duration) error {
	b, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	str := string(b)
	forever := tries == -1
	for forever || tries >= 0 {
		err := qp.Put(str)
		if err != nil {
			// if we failed to connect to redis or received EOF error then retry
			if _, ok := err.(*net.OpError); ok || err == io.EOF {
				log.Printf("uuid=%s Retrying event %s in (%s) queue: %v\n", msg.UUID, msg.Event, qp.Name, err)
				tries--
				time.Sleep(retryInterval)
				continue
			}
			// redis emits `open connections too fast` when we exceeds the error limit
			if strings.Contains(err.Error(), "open connections too fast") {
				log.Printf("uuid=%s Retrying event %s in (%s) queue: %v\n", msg.UUID, msg.Event, qp.Name, err)
				time.Sleep(1 * time.Second)
				continue
			}
			return err
		}
		return nil
	}
	return fmt.Errorf("timed out sending message %+v", msg)
}

func (qp *QueuePublisher) CreateConsumer() (*QueueConsumer, error) {
	c, err := qp.AddConsumer(qp.Name)
	if err != nil {
		return nil, err
	}

	return &QueueConsumer{Consumer: c}, nil
}

func (qc *QueueConsumer) Consume() (*Message, error) {
	p, err := qc.Get()
	if err != nil {
		if qc.HasUnacked() {
			log.Println("pushed the message to working queue even if it failes to get a message. Requeue again to process")
			time.Sleep(1 * time.Second)
			qc.RequeueWorking()
		}
		return nil, err
	}

	var m Message
	err = json.Unmarshal([]byte(p.Payload), &m)
	if err != nil {
		return nil, err
	}
	m.p = p

	return &m, nil
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

func (qc *QueueConsumer) Start(stop chan struct{}) error {
	queueName := qc.Name
	log.Printf("started queue consumer %s", queueName)

	msg := make(chan *Message)
	go func() {
		var last_error error
		for {
			m, err := qc.Consume()
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
