package queue

import (
	"fmt"
	"io"
	"log"
	"net"
	"strings"
	"time"

	"github.com/adjust/redismq"
)

type RedisPublisher struct {
	Queue *redismq.Queue
}

func NewRedisQueue(host string, port string, db int64, name string) Publisher {
	return &RedisPublisher{Queue: redismq.CreateQueue(host, port, "", db, name)}
}

// Publish pushes the message into the queue without retrying if that fails
func (qp *RedisPublisher) Publish(msg *Message) error {
	return qp.PublishRetry(msg, 1, 0)
}

// PublishRetry publishes the message at most `tries` times. When it fails to publish because
// redis is not available, it sleeps for `retryInterval` duration.
// When tries is -1, it keeps trying until it successes to publish to redis
func (qp *RedisPublisher) PublishRetry(m *Message, tries int, retryInterval time.Duration) error {
	b, err := m.Encode()
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
	return fmt.Errorf("timed out sending message %+v", m)
}

// Delete clears all input and failed queues as well as all consumers. It will
// not proceed as long as consumers are running
func (qp *RedisPublisher) Delete() error {
	return qp.Queue.Delete()
}

// ResetInput removes all messages from the input queue
func (qp *RedisPublisher) ResetInput() error {
	return qp.Queue.ResetInput()
}

// GetInputLength returns the number of pending messages in the input queue
func (qp *RedisPublisher) GetInputLength() int64 {
	return qp.Queue.GetInputLength()
}

// GetInputLength returns the number of messages in the failed queue
func (qp *RedisPublisher) GetFailedLength() int64 {
	return qp.Queue.GetFailedLength()
}

// RedisDelivery encapsulates the redis package and makes it behave as the
// delivery interface
type redisDelivery struct {
	msg *Message
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
				log.Printf("uuid=%s Retrying ack for message(%s) : %v", m.UUID, m.Event, err)
				time.Sleep(retryInterval)
				continue
			}

			// redis emits `open connections too fast` when we exceeds the error limit
			if strings.Contains(err.Error(), "open connections too fast") {
				log.Printf("uuid=%s Retrying ack for message(%s) : %v", m.UUID, m.Event, err)
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
		log.Printf("[ERROR] uuid=%s Failed to push a message(%s) to failed queue: %v",
			d.msg.UUID, d.msg.Payload, err)
	}
}

// RedisConsumer represents a consumer to a redis queue.
type RedisConsumer struct {
	Consumer *redismq.Consumer
	handler  Handler
}

// Consume reads the first message in the queue, returns its content as well as
// the delivery to manage the message lifecycle
func (qc *RedisConsumer) Consume() (*Message, Delivery, error) {
	p, err := qc.Consumer.Get()
	if err != nil {
		if qc.Consumer.HasUnacked() {
			log.Println("pushed the message to working queue even if it failes to get a message. Requeue again to process")
			time.Sleep(1 * time.Second)
			qc.Consumer.RequeueWorking()
		}
		return nil, nil, err
	}

	m := &Message{}
	if err := m.Decode([]byte(p.Payload)); err != nil {
		return nil, nil, err
	}
	return m, &redisDelivery{msg: m, p: p}, nil
}

// Start begins consuming the messages. It will keep consuming until we tries
// to stop it by sending a message to the provided stop channel.
func (qc *RedisConsumer) Start(stop chan struct{}) error {
	queueName := qc.Consumer.Name
	log.Printf("started queue consumer %s", queueName)

	msgCh := make(chan *redisDelivery)
	go func() {
		var last_error error
		for {
			_, delivery, err := qc.Consume()
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
			msgCh <- delivery.(*redisDelivery)
			<-msgCh
		}
	}()

	for {
		select {
		case c := <-stop:
			log.Printf("stopped queue consumer %s", queueName)
			stop <- c
			return nil
		case d := <-msgCh:
			m := d.msg
			err := qc.handler(m)
			if err != nil {
				log.Printf("[ERROR] uuid=%s Failed to process (%+v) message in (%s) queue: %v\n", m.UUID, m, queueName, err)
				d.Fail()
			} else {
				err := d.Ack(1 * time.Second)
				if err != nil {
					log.Printf("[ERROR] uuid=%s Failed to send ACK for (%+v) message in (%s) queue: %v\n", m.UUID, m, queueName, err)
				}
			}
			msgCh <- d
		}
	}

	return nil
}

func StartRedisConsumer(host, port string, db int64, queueName string, handler Handler, stop chan struct{}) error {
	var qc *RedisConsumer
	var err error

	for {
		select {
		case c := <-stop:
			log.Printf("shutting down before initialize queue consumer: %s", queueName)
			stop <- c
			return nil
		default:
			qc, err = CreateRedisConsumer(host, port, db, queueName)
			if err == nil {
				qc.handler = handler
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

func CreateRedisConsumer(host, port string, db int64, queueName string) (*RedisConsumer, error) {
	q := redismq.CreateQueue(host, port, "", db, queueName)
	c, err := q.AddConsumer(queueName)
	if err != nil {
		return nil, fmt.Errorf("Failed to add a consumer: %v", err)
	}

	err = c.RequeueWorking() // Requeue all messages that does not send ACK
	if err != nil {
		return nil, fmt.Errorf("Failed to requeue not finished job: %v", err)
	}

	return &RedisConsumer{Consumer: c}, nil
}
