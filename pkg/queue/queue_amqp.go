package queue

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/Sirupsen/logrus"
	"github.com/streadway/amqp"
)

var (
	reconnectWait = 500 * time.Millisecond // time wait between reconnect
	consumeWait   = 500 * time.Millisecond // time wait when consume returns error
	ackWait       = 500 * time.Millisecond // time wait when ack returns error
	// number of maximum outstanding publishings. This is a theoretical number
	// used to configure ack/nack notification channel.
	// http://godoc.org/github.com/streadway/amqp#Channel.NotifyConfirm
	nPublishing = 100

	errDeliveryClosed = errors.New("delivery channel is closed")
)

type AmqpPublisher struct {
	addr string
	name string // queue-name
	dlex string // dead-letter exchange name
	dlq  string // dead-letter queue name
	conn *amqp.Connection
	ch   *amqp.Channel

	acks, nacks chan uint64
}

// Publish pushes the message into the queue without retrying if that fails
func (p *AmqpPublisher) Publish(msg *Message) error {
	return p.PublishRetry(msg, 1, 0)
}

// PublishRetry publishes the message at most `tries` times. When it fails to publish because
// the server is not available, it sleeps for `retryInterval` duration.
// When tries is -1, it keeps trying until it successesfully publishes
func (p *AmqpPublisher) PublishRetry(m *Message, tries int, retryInterval time.Duration) error {
	b, err := json.Marshal(m)
	if err != nil {
		return err
	}
	forever := tries == -1
PUBLISH:
	for forever || tries > 0 {
		err := p.ch.Publish(
			"", p.name, // publish to the default exchange with routingKey=name
			true,  // mandatory=true, there must be a queue that matches this key
			false, // immediate=false, dont need an active consumer
			amqp.Publishing{
				DeliveryMode: amqp.Persistent,
				Timestamp:    time.Now(),
				Type:         m.Action,
				AppId:        fmt.Sprintf("%s-%d", m.ResourceName, m.ResourceID),
				Body:         b,
			},
		)
		if err != nil {
			if err == amqp.ErrClosed {
				logrus.Printf("uuid=%s Retrying event %s in (%s) queue: %v", m.UUID, m.Event, p.name, err)
				tries--
				time.Sleep(retryInterval)
				if err := p.reconnect(); err != nil {
					logrus.Errorf("uuid=%s amqp: reconnect %s/%s failed, err: %v", m.UUID, p.addr, p.name, err)
				}
				continue PUBLISH
			}
			return err
		}
		tries--
		select {
		case <-p.acks:
			return nil
		case <-p.nacks:
			// Republish the message when it is NACKed
			// if tries is exhausted, it means that the message will be
			// discarded
			logrus.Errorf("uuid=%s queue=%s message=%s nacked", m.UUID, p.name, m.Event)
			continue PUBLISH
		case <-time.After(10 * time.Second):
			// timed out waiting for server to confirm.
			// reconnect and retry publishing
			logrus.Printf("uuid=%s queue=%s message=%s timed out", m.UUID, p.name, m.Event)
			if err := p.reconnect(); err != nil {
				logrus.Errorf("uuid=%s amqp: reconnect %s/%s failed, err: %v", m.UUID, p.addr, p.name, err)
			}
			continue PUBLISH
		}
		return nil
	}
	return fmt.Errorf("failed to send message %+v", m)
}

// Delete clears all input and failed queues as well as all consumers. It will
// not proceed as long as consumers are running. This is typically used in
// tests
func (p *AmqpPublisher) Delete() error {
	n, err := p.ch.QueueDelete(p.name,
		true,  // ifUnused=true, will not delete if there's consumer
		false, // ifEmpty=false, delete all messages in the queue
		false) // noWait=true, wait until all messages are deleted
	if n > 0 {
		logrus.Printf("amqp: Delete %d messages from %s", n, p.name)
	}
	return err
}

// ResetInput removes all messages from the input queue. This is typically used
// in tests
func (p *AmqpPublisher) ResetInput() error {
	n, err := p.ch.QueuePurge(p.name, false) // wait until all messages are deleted
	if n > 0 {
		logrus.Printf("amqp: ResetInput %d messages from %s", n, p.name)
	}
	return err
}

// GetInputLength returns the number of pending messages in the input queue
func (p *AmqpPublisher) GetInputLength() int64 {
	q, err := p.ch.QueueInspect(p.name)
	if err != nil {
		logrus.Errorf("amqp: failed to get %s queue, err: %v", p.name, err)
		return 0 // invalid input length because of error
	}
	return int64(q.Messages)
}

// GetInputLength returns the number of messages in the failed queue
func (p *AmqpPublisher) GetFailedLength() int64 {
	dlq := p.name + "-dlq"
	// declare the dead-letter queue
	q, err := p.ch.QueueInspect(dlq)
	if err != nil {
		logrus.Errorf("amqp: failed to get %s queue, err: %v", dlq, err)
		return 0
	}
	return int64(q.Messages)
}

func (p *AmqpPublisher) CreateConsumer() (Consumer, error) {
	panic("not implemented")
}

// reconnect will keep connecting to the queue until it's successfully
// connected. Error will be returned when the queue is connected but is not
// behaving correctly.
func (p *AmqpPublisher) reconnect() error {
CONNECT:
	conn, ch, err := setup(p.addr, p.name, p.dlex, p.dlq)
	if err != nil {
		logrus.Errorf("amqp: connect %s/%s failed, err: %v", p.addr, p.name, err)
		time.Sleep(reconnectWait)
		goto CONNECT
	}
	acks, nacks := ch.NotifyConfirm(
		make(chan uint64, nPublishing),
		make(chan uint64, nPublishing))
	if err := ch.Confirm(false); err != nil {
		return err
	}
	p.conn = conn
	p.ch = ch
	p.acks = acks
	p.nacks = nacks
	return nil
}

func NewAmqpQueue(addr string, name string) (Publisher, error) {
	p := &AmqpPublisher{
		addr: addr,
		name: name,
		dlex: name + "-dlex",
		dlq:  name + "-dlq",
	}
	if err := p.reconnect(); err != nil {
		return nil, err
	}
	return p, nil
}

// amqpDelivery encapsulates a single amqp.Delivery and makes it behave as the
// delivery interface
type amqpDelivery struct {
	msg      *Message
	delivery amqp.Delivery
	consumer *AmqpConsumer
}

// Ack acknowledges a message. This allows its consumer to continue consuming
// the next message.
func (d *amqpDelivery) Ack(retryInterval time.Duration) error {
	for {
		d.consumer.mu.RLock()
		err := d.consumer.ch.Ack(d.delivery.DeliveryTag, false)
		d.consumer.mu.RUnlock()
		if err != nil {
			if _, ok := err.(*net.OpError); ok {
				logrus.Printf("uuid=%s Retrying ack for message(%s) : %v",
					d.msg.UUID, d.msg.Event, err)
				time.Sleep(retryInterval)
				continue
			}
		}
		return err
	}
}

// Fail push failed message to failed queue.
// It will not retry since we can submit a message manually
func (d *amqpDelivery) Fail() {
	// nack only this message and sends it to dead-letter queue
	d.consumer.mu.RLock()
	err := d.consumer.ch.Nack(d.delivery.DeliveryTag, false, false)
	d.consumer.mu.RUnlock()
	if err != nil {
		logrus.Errorf("uuid=%s Failed to push a message(%s) to failed queue: %v",
			d.msg.UUID, d.msg.Event, err)
	}
}

// RedisConsumer represents a consumer to a redis queue.
type AmqpConsumer struct {
	addr string
	name string // queue-name
	dlex string // dead-letter exchange name
	dlq  string // dead-letter queue name

	handler        queueHandler
	handlerTimeout time.Duration

	mu       sync.RWMutex
	conn     *amqp.Connection
	ch       *amqp.Channel
	delivery <-chan amqp.Delivery
}

// Consume reads the first message in the queue, returns its content as well as
// the delivery to manage the message lifecycle
func (qc *AmqpConsumer) Consume() (*Message, Delivery, error) {
	qc.mu.RLock()
	d, ok := <-qc.delivery
	qc.mu.RUnlock()
	if !ok {
		return nil, nil, errDeliveryClosed
	}
	m := &Message{}
	if err := json.Unmarshal(d.Body, m); err != nil {
		return nil, nil, err
	}
	return m, &amqpDelivery{msg: m, delivery: d, consumer: qc}, nil
}

// Close closes the underlying connection and reset all resources
func (qc *AmqpConsumer) Close() error {
	qc.mu.Lock()
	err := qc.conn.Close()
	qc.mu.Unlock()
	return err
}

// reconnect setups a new connection to Rabbitmq server and establishes the
// delivery channel. When the server is down, it assumes that the server will
// goes up eventually and keep retrying to connect to the server without user
// intervention.
func (qc *AmqpConsumer) reconnect() error {
	qc.mu.Lock()
	defer qc.mu.Unlock()
CONNECT:
	conn, ch, err := setup(qc.addr, qc.name, qc.dlex, qc.dlq)
	if err != nil {
		logrus.Errorf("amqp: connect %s/%s failed, err: %v", qc.addr, qc.name, err)
		time.Sleep(reconnectWait)
		goto CONNECT
	}
	qc.conn = conn
	qc.ch = ch
	delivery, err := qc.ch.Consume(qc.name,
		"",    // auto-generate an unique id for this consumer
		false, // autoAck = false, always manually ack/nack
		true,  // exclusive=true, enforce 1 consumer per queue for now
		false, // noLocal=false, allow sharing of publish/consume TCP conn
		false, // noWait=false, wait for server to confirm request
		nil)
	if err != nil {
		rerr, ok := err.(*amqp.Error)
		// if there's a consumer in the queue, keep reconnect until we can
		// create a new consumer
		if ok && rerr.Code == amqp.AccessRefused && strings.Contains(rerr.Reason, "in exclusive use") {
			logrus.Errorf("amqp: waiting for previous queue comsumer to be down %s", err)
			time.Sleep(reconnectWait)
			goto CONNECT
		}
		return err
	}
	qc.delivery = delivery
	logrus.Printf("amqp: consumer connected to queue %s/%s", qc.addr, qc.name)
	return nil
}

// Start begins consuming the messages. It will keep consuming until we tries
// to stop it by sending a message to the provided stop channel.
func (qc *AmqpConsumer) Start(stop chan struct{}) error {
	logrus.Printf("started queue consumer %s", qc.name)
	msgCh := make(chan *amqpDelivery)
	go func() {
		for {
			_, delivery, err := qc.Consume()
			if err != nil {
				if err == errDeliveryClosed {
					logrus.Warnf("queue %s consumer connection is closed", qc.name)
					time.Sleep(consumeWait)
				} else {
					logrus.Errorf("Failed to consume a message from (%s) queue: %+v", qc.name, err)
				}
				continue
			}
			msgCh <- delivery.(*amqpDelivery)
			<-msgCh
		}
	}()

	done := make(chan struct{})
	for {
		select {
		case c := <-stop: // when we want to stop the consumer
			logrus.Printf("stopped queue consumer %s", qc.name)
			if err := qc.conn.Close(); err != nil {
				logrus.Errorf("amqp: failed to close consumer for %s, err: %v", qc.name, err)
			}
			stop <- c
			return nil
		case d := <-msgCh: // when there's new message
			m := d.msg
			logrus.Infof("uuid=%s got (%+v) message in (%s) queue", m.UUID, m, qc.name)
			go func() {
				err := qc.handler(m)
				if err != nil {
					logrus.Errorf("uuid=%s Failed to process (%+v) message in (%s) queue: %v", m.UUID, m, qc.name, err)
					d.Fail()
				} else {
					err := d.Ack(ackWait)
					if err != nil {
						logrus.Errorf("uuid=%s Failed to send ACK for (%+v) message in (%s) queue: %v", m.UUID, m, qc.name, err)
					}
				}
				done <- struct{}{}
			}()
			select {
			case <-done:
				logrus.Debugf("uuid=%s Finished to process (%+v) message in (%s) queue", m.UUID, m, qc.name)
			case <-time.After(qc.handlerTimeout):
				logrus.Errorf("uuid=%s Timed out to process (%+v) message in (%s) queue", m.UUID, m, qc.name)
				d.Fail()
			}
			msgCh <- d
		}
	}

	return nil
}

func StartAmqpConsumer(addr, name string, handler queueHandler, stop chan struct{}, handlerTimeout time.Duration) error {
	qc, err := CreateAmqpConsumer(addr, name, handler, handlerTimeout)
	if err == nil {
		return qc.Start(stop)
	}

	logrus.Errorf("amqp: failed to create consumer, err: %v", err)
	panic(err)
}

func CreateAmqpConsumer(addr, name string, handler queueHandler, handlerTimeout time.Duration) (Consumer, error) {
	qc := &AmqpConsumer{
		addr: addr,
		name: name,
		dlex: name + "-dlex",
		dlq:  name + "-dlq",
		// maximum duration of a handler
		handlerTimeout: handlerTimeout,
	}
	if err := qc.reconnect(); err != nil {
		return nil, err
	}
	qc.handler = handler

	// start a separate goroutine to monitor current connection
	go func() {
		for err := range qc.conn.NotifyClose(make(chan *amqp.Error)) {
			if err != nil {
				logrus.Errorf("amqp: consumer closed %s", err)
				qc.reconnect()
			}
		}
	}()

	return qc, nil
}

// setup creates a correct topology for our queue.
// We want to configure 1 queue + 1 dead-leter exchange + 1 dead-letter queue
func setup(addr, name, dlex, dlq string) (*amqp.Connection, *amqp.Channel, error) {
	conn, err := amqp.Dial(addr)
	if err != nil {
		return nil, nil, err
	}
	ch, err := conn.Channel()
	if err != nil {
		return nil, nil, err
	}

	// declare the dead-letter exchange
	err = ch.ExchangeDeclare(dlex,
		"fanout", // all queue connects to the dead-letter exchange gets the messages
		true,     // durable = true
		false,    // autoDelete = false
		false,    // exclusive = false
		false,    // noWait = false
		nil)
	if err != nil {
		return nil, nil, fmt.Errorf("amqp: failed to declare %s, err: %v", dlex, err)
	}
	// declare the dead-letter queue
	_, err = ch.QueueDeclare(dlq,
		true,  // durable = true
		false, // autoDelete = false
		false, // exclusive = false
		false, // noWait = false
		nil)
	if err != nil {
		return nil, nil, fmt.Errorf("amqp: failed to declare %s, err: %v", dlex, err)
	}
	// bind the dead-letter queue to the dead-letter exchange
	if err := ch.QueueBind(dlq, "#", dlex, false, nil); err != nil {
		return nil, nil, fmt.Errorf("amqp: failed to bind %s to %s, err: %v", dlq, dlex, err)
	}

	_, err = ch.QueueDeclare(name,
		true,  // durable = true
		false, // autoDelete = false
		false, // exclusive = false
		false, // noWait = false
		amqp.Table{
			"x-dead-letter-exchange": dlex,
		})
	if err != nil {
		return nil, nil, fmt.Errorf("amqp: failed to declare %s, err: %v", name, err)
	}

	return conn, ch, nil
}
