package queue

import (
	"encoding/json"
	"fmt"
	"log"

	"github.com/adjust/redismq"
	"github.com/nitrous-io/dedicated/hostbuilder/state"
)

type QueuePublisher struct {
	*redismq.Queue
}

var queuePublishersByName = map[string]*QueuePublisher{}

type QueueConsumer struct {
	*redismq.Consumer
}

type Message struct {
	Action       string `json: "action"`
	Event        string `json: "event"`
	ResourceName string `json: "resource_name"`
	ResourceID   string `json: "resource_id"`
	Payload      string `json: "payload"`
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

type HostQueuePublisher struct {
	HostID int64  `json: "id"`
	Slug   string `json: "slug"`
	UserID int64  `json: "user_id"`

	Provider       string      `json:"provider"`
	ID             string      `json:"id"`
	Region         string      `json:"region_id"`
	Name           string      `json:"name"`
	IPAddress      string      `json:"ip_address"`
	FreighterdPort uint16      `json:"freighterd_port"`
	State          state.State `json:"state"`

	queue *QueuePublisher
}

func init() {
	queueRoutes = map[string]queueHandler{}
}

func NewQueue(redisUrl string, redisPort string, redisDatabase int64, name string) *QueuePublisher {
	qp, ok := queuePublishersByName[name]
	if !ok {
		qp = &QueuePublisher{Queue: redismq.CreateQueue(redisUrl, redisPort, "", redisDatabase, name)}
		queuePublishersByName[name] = qp
	}
	return qp
}

// This is only for testing to clean up all database from queue.
// It should not be use in production
func (qp *QueuePublisher) Close() {
	name := qp.Name
	qp.Delete()
	delete(queuePublishersByName, name)
}

func (qp *QueuePublisher) NewHostQueuePublisher(hid int64, slug string, userID int64) *HostQueuePublisher {
	hqp := &HostQueuePublisher{
		HostID: hid,
		Slug:   slug,
		UserID: userID,
		queue:  qp,
	}
	return hqp
}

func (qp *QueuePublisher) Publish(action string, event string, resourceName string, resourceID string, payload string) error {
	message := &Message{
		Action:       action,
		Event:        event,
		ResourceName: resourceName,
		ResourceID:   resourceID,
		Payload:      payload,
	}
	b, err := json.Marshal(message)
	if err != nil {
		return err
	}
	str := string(b)
	err = qp.Put(str)
	if err != nil {
		return err
	}
	return nil
}

func (hqp *HostQueuePublisher) Publish(action string, event string, resourceName string, resourceID string) error {
	hostinfo, err := json.Marshal(hqp)
	if err != nil {
		return fmt.Errorf("Failed to marshal host queue information: %v", err)
	}

	message := &Message{
		Action:       action,
		Event:        event,
		ResourceName: resourceName,
		ResourceID:   resourceID,
		Payload:      string(hostinfo),
	}

	b, err := json.Marshal(message)
	if err != nil {
		return err
	}
	str := string(b)
	err = hqp.queue.Put(str)
	if err != nil {
		return err
	}
	return nil
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
			log.Printf("Failed to process (%m) message in (%s) queue: %v\n", m, queueName, err)
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
