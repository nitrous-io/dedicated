package queue

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"strings"
	"time"

	"github.com/Sirupsen/logrus"
	"github.com/adjust/redismq"
)

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
	// CreateConsumer creates a consumer to read messages from the current
	// queue. This is likely Redis-only.
	CreateConsumer() (*QueueConsumer, error)
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
	Start(stop chan struct{}) error
}

type QueuePublisher struct {
	Queue *redismq.Queue
}

type QueueConsumer struct {
	Consumer       *redismq.Consumer
	handlerTimeout time.Duration
	handler        queueHandler
}

type Message struct {
	UUID         string `json: "uuid"`
	Action       string `json: "action"`
	Event        string `json: "event"`
	ResourceName string `json: "resource_name"`
	ResourceID   string `json: "resource_id"`
	Payload      []byte `json: "payload"`
}

type queueHandler func(*Message) error

func NewQueue(redisUrl string, redisPort string, redisDatabase int64, name string) Publisher {
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
		err := qp.Queue.Put(str)
		if err != nil {
			// if we failed to connect to redis or received EOF error then retry
			if _, ok := err.(*net.OpError); ok || err == io.EOF {
				logrus.Warnf("uuid=%s Retrying event %s in (%s) queue: %v", msg.UUID, msg.Event, qp.Queue.Name, err)
				tries--
				time.Sleep(retryInterval)
				continue
			}
			// redis emits `open connections too fast` when we exceeds the error limit
			if strings.Contains(err.Error(), "open connections too fast") {
				logrus.Warnf("uuid=%s Retrying event %s in (%s) queue: %v", msg.UUID, msg.Event, qp.Queue.Name, err)
				time.Sleep(1 * time.Second)
				continue
			}
			return err
		}
		return nil
	}
	return fmt.Errorf("timed out sending message %+v", msg)
}

// CreateConsumer creates a consumer to read message from the redis queue.
func (qp *QueuePublisher) CreateConsumer() (*QueueConsumer, error) {
	c, err := qp.Queue.AddConsumer(qp.Queue.Name)
	if err != nil {
		return nil, err
	}

	return &QueueConsumer{Consumer: c}, nil
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

func (qc *QueueConsumer) Consume() (*Message, Delivery, error) {
	p, err := qc.Consumer.Get()
	if err != nil {
		if qc.Consumer.HasUnacked() {
			logrus.Warn("pushed the message to working queue even if it failes to get a message. Requeue again to process")
			time.Sleep(1 * time.Second)
			qc.Consumer.RequeueWorking()
		}
		return nil, nil, err
	}

	var m Message
	err = json.Unmarshal([]byte(p.Payload), &m)
	if err != nil {
		return nil, nil, err
	}
	return &m, &redisDelivery{msg: &m, p: p}, nil
}

// RedisDelivery encapsulates the redis package and makes it behave as the
// delivery interface
type redisDelivery struct {
	msg *Message // embeds message so we can refer to the data for logging...
	p   *redismq.Package
}

// Ack send ack packet to consume next message
// it sleeps for `retryInterval` duration when it fails to send it
// it will not stop retrying because it could occur data inconsistency problem
func (d *redisDelivery) Ack(retryInterval time.Duration) error {
	m := d.msg
	for {
		err := d.p.Ack()
		if err != nil {
			// if we failed to connect to redis or received EOF error then retry
			if _, ok := err.(*net.OpError); ok || err == io.EOF {
				logrus.Warnf("uuid=%s Retrying ack for message(%s) : %v", m.UUID, m.Event, err)
				time.Sleep(retryInterval)
				continue
			}

			// redis emits `open connections too fast` when we exceeds the error limit
			if strings.Contains(err.Error(), "open connections too fast") {
				logrus.Warnf("uuid=%s Retrying ack for message(%s) : %v", m.UUID, m.Event, err)
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
func (d *redisDelivery) Fail() {
	err := d.p.Fail()
	if err != nil {
		logrus.Errorf("uuid=%s Failed to push a message(%s) to failed queue: %v",
			d.msg.UUID, d.msg.Payload, err)
	}
}

func (qc *QueueConsumer) Start(stop chan struct{}) error {
	queueName := qc.Consumer.Name
	logrus.Debugf("started queue consumer %s", queueName)

	msg := make(chan *redisDelivery)
	go func() {
		var last_error error
		for {
			logrus.Debugf("waiting for consuming next message from (%s) queue", queueName)
			_, d, err := qc.Consume()
			if err != nil {
				logrus.Errorf("Failed to consume a message from (%s) queue: %+v", queueName, err)
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
			msg <- d.(*redisDelivery)
			<-msg
		}
	}()

	finished := make(chan bool)
	for {
		select {
		case c := <-stop:
			logrus.Debugf("stopped queue consumer %s", queueName)
			stop <- c
			return nil
		case d := <-msg:
			m := d.msg
			logrus.Infof("uuid=%s got (%+v) message in (%s) queue", m.UUID, m, queueName)
			go func() {
				err := qc.handler(m)
				if err != nil {
					logrus.Errorf("uuid=%s Failed to process (%+v) message in (%s) queue: %v", m.UUID, m, queueName, err)
					d.Fail()
				} else {
					err := d.Ack(1 * time.Second)
					if err != nil {
						logrus.Errorf("uuid=%s Failed to send ACK for (%+v) message in (%s) queue: %v", m.UUID, m, queueName, err)
					}
				}
				finished <- true
			}()

			select {
			case <-finished:
				logrus.Debugf("uuid=%s Finished to process (%+v) message in (%s) queue", m.UUID, m, queueName)
			case <-time.After(qc.handlerTimeout):
				logrus.Errorf("uuid=%s Timed out to process (%+v) message in (%s) queue", m.UUID, m, queueName)
				d.Fail()
			}
			msg <- d
		}
	}

	return nil
}

func StartConsumer(redisHost, redisPort string, redisDB int64, queueName string, handler queueHandler, stop chan struct{}, queueHandlerTimeout time.Duration) error {
	var qc Consumer
	var err error
	queue := redismq.CreateQueue(redisHost, redisPort, "", int64(redisDB), queueName)

	for {
		select {
		case c := <-stop:
			logrus.Debugf("shutting down before initialize queue consumer: %s", queueName)
			stop <- c
			return nil
		default:
			qc, err = CreateConsumer(queue, queueName, handler, queueHandlerTimeout)
			if err == nil {
				return qc.Start(stop)
			}

			if !strings.Contains(err.Error(), "consumer with this name is already active") {
				logrus.Fatalf("Failed to start queue consumer: %v", err)
			}

			logrus.Debugf("Waiting for previous queue comsumer to be down")
			time.Sleep(100 * time.Millisecond)
		}
	}
}

func CreateConsumer(q *redismq.Queue, queueName string, handler queueHandler, handlerTimeout time.Duration) (Consumer, error) {
	c, err := q.AddConsumer(queueName)
	if err != nil {
		return nil, fmt.Errorf("failed to add a consumer: %v", err)
	}

	err = c.RequeueWorking() // Requeue all messages that does not send ACK
	if err != nil {
		return nil, fmt.Errorf("failed to requeue not finished job: %v", err)
	}

	return &QueueConsumer{
		Consumer:       c,
		handler:        handler,
		handlerTimeout: handlerTimeout,
	}, nil
}
