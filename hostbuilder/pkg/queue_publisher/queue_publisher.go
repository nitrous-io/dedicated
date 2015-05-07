package queue_publisher

import (
	"encoding/json"

	"github.com/adjust/redismq"
)

type QueuePublisher struct {
	queue *redismq.Queue
}

type Message struct {
	Event        string `json: "event"`
	ResourceName string `json: "resource_name"`
	ResourceID   string `json: "resource_id"`
	Payload      string `json: "payload"`
}

func NewQueue(redisUrl string, redisPort string, redisDatabase int64, name string) *QueuePublisher {
	return &QueuePublisher{queue: redismq.CreateQueue(redisUrl, redisPort, "", redisDatabase, name)}
}

func (qp *QueuePublisher) Publish(event string, resourceName string, resourceID string, payload string) error {
	message := &Message{
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
	qp.queue.Put(str)
	return nil
}

func (qp *QueuePublisher) Length() int64 {
	return qp.queue.GetInputLength()
}

func (qp *QueuePublisher) Reset() error {
	err := qp.queue.ResetInput()
	return err
}
