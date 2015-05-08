package queue

import (
	"encoding/json"
	"fmt"
	"log"

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

type queueHandler func(*QueueConsumer, *Message) error

var queueRoutes map[string]queueHandler

// AddHandler adds the handler to queue's name
func AddHandler(name string, h queueHandler) {
	queueRoutes[name] = h
}

func init() {
	queueRoutes = map[string]queueHandler{}
}

func NewQueue(redisUrl string, redisPort string, redisDatabase int64, name string) *QueuePublisher {
	return &QueuePublisher{Queue: redismq.CreateQueue(redisUrl, redisPort, "", redisDatabase, name)}
}

func (qp *QueuePublisher) Publish(msg *Message) error {
	b, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	str := string(b)
	return qp.Put(str)
}

func (qp *QueuePublisher) CreateConsumer() (*QueueConsumer, error) {
	c, err := qp.AddConsumer(qp.Name)
	if err != nil {
		return nil, err
	}

	return &QueueConsumer{c}, nil
}

func (qc *QueueConsumer) Consume() (*Message, error) {
	p, err := qc.Get()
	if err != nil {
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

func (m *Message) Ack() {
	m.p.Ack()
}

func (qc *QueueConsumer) Start() error {
	queueName := qc.Name
	for {
		m, err := qc.Consume()
		if err != nil {
			log.Printf("Failed to consume a message from (%s) queue: %v\n", queueName, err)
			continue
		}

		err = queueRoutes[queueName](qc, m)
		if err != nil {
			log.Printf("Failed to process (%+v) message in (%s) queue: %v\n", m, queueName, err)
			// FIXME: We need to decide whether let it fail or put this to failed queue and consume next message
			// If we ignore it, it could affect next message. For example, first message is `create` and next one is `update`
			// If it fails to consume first message, second message will also fail since item is not created yet.
			m.Ack()
			continue
		}

		m.Ack()
	}
	return nil
}

func StartConsumer(redisHost string, redisPort string, redisDB int64, queueName string) error {
	qc, err := CreateConsumer(redisHost, redisPort, redisDB, queueName)
	if err != nil {
		log.Fatalf("Failed to start queue consumer: %v\n", err)
	}
	return qc.Start()
}

func CreateConsumer(redisHost string, redisPort string, redisDB int64, queueName string) (*QueueConsumer, error) {
	queue := redismq.CreateQueue(redisHost, redisPort, "", int64(redisDB), queueName)
	c, err := queue.AddConsumer(queueName)
	if err != nil {
		return nil, fmt.Errorf("Failed to add a consumer: %v\n", err)
	}

	err = c.RequeueWorking() // Requeue all messages that does not send ACK
	if err != nil {
		return nil, fmt.Errorf("Failed to requeue not finished job: %v\n", err)
	}

	return &QueueConsumer{c}, nil
}
