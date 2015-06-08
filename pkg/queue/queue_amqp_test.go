package queue_test

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/nitrous-io/dedicated/pkg/queue"
	"github.com/nitrous-io/dedicated/pkg/reexec"
	"github.com/nitrous-io/dedicated/test"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func init() {
	reexec.Register("amqp1", func() {
		log.SetPrefix("amqp1 :")
		stop := make(chan struct{})
		h := func(m *queue.Message) error {
			redisClient, err := test.PrepareRedis(test.RedisHost, test.RedisPort, test.RedisTestDb)
			if err != nil {
				return err
			}
			redisClient.Set("amqp-handler", "amqp1")
			return nil
		}
		err := queue.StartAmqpConsumer(test.RabbitMQAddr, "test", h, stop)
		log.Fatalf("amqp1 failed to start %v", err)
	})

	reexec.Register("amqp2", func() {
		log.SetPrefix("amqp2 :")
		stop := make(chan struct{})
		h := func(m *queue.Message) error {
			redisClient, err := test.PrepareRedis(test.RedisHost, test.RedisPort, test.RedisTestDb)
			if err != nil {
				return err
			}
			redisClient.Set("amqp-handler", "amqp2")
			return nil
		}
		err := queue.StartAmqpConsumer(test.RabbitMQAddr, "test", h, stop)
		log.Fatalf("amqp2 failed to start %v", err)
	})

	reexec.Register("handle-failure", func() {
		log.SetPrefix("handle-failure :")
		stop := make(chan struct{})
		h := func(m *queue.Message) error {
			redisClient, err := test.PrepareRedis(test.RedisHost, test.RedisPort, test.RedisTestDb)
			if err != nil {
				return err
			}
			if m.UUID == "success" {
				redisClient.Set("amqp-handler", "handle-failure")
				return nil
			}
			return fmt.Errorf("Error!!!")
		}
		err := queue.StartAmqpConsumer(test.RabbitMQAddr, "test", h, stop)
		log.Fatalf("amqp2 failed to start %v", err)
	})

	reexec.Register("handle-amqp-stop", func() {
		log.SetPrefix("handle-amqp-stop :")
		syncFd := os.NewFile(3, "syncfd")
		defer syncFd.Close()

		stop := make(chan struct{})
		blocking := make(chan struct{})
		h := func(m *queue.Message) error {
			<-blocking
			redisClient, err := test.PrepareRedis(test.RedisHost, test.RedisPort, test.RedisTestDb)
			if err != nil {
				return err
			}
			redisClient.Set("amqp-message", m.UUID)
			return nil
		}

		go queue.StartAmqpConsumer(test.RabbitMQAddr, "test", h, stop)

		time.Sleep(500 * time.Millisecond)

		// sending SIGHUP will make the consumer processing the message
		// sending SIGTERM will try to stop the consumer
		c := make(chan os.Signal, 2)
		signal.Notify(c, syscall.SIGHUP, syscall.SIGTERM)

		// sync with parent to let it know that signal can be sent
		syncFd.Write([]byte("1"))

		for {
			s := <-c
			switch s {
			case syscall.SIGHUP:
				blocking <- struct{}{}
			case syscall.SIGTERM:
				stop <- struct{}{}
				<-stop
				os.Exit(0)
			}
		}
	})
}

var _ = Describe("pkg/queue/amqp", func() {
	var qp queue.Publisher

	BeforeEach(func() {
		output, err := exec.Command("service", "rabbitmq-server", "start").CombinedOutput()
		Expect(err).To(BeNil(), "failed to start rabbitmq %s", string(output))
		if !strings.Contains(string(output), "already running") {
			time.Sleep(100 * time.Millisecond)
		}

		// reset rabbitmq state
		Expect(exec.Command("rabbitmqctl", "stop_app").Run()).To(BeNil())
		Expect(exec.Command("rabbitmqctl", "reset").Run()).To(BeNil())
		Expect(exec.Command("rabbitmqctl", "start_app").Run()).To(BeNil())

		qp, err = queue.NewAmqpQueue(test.RabbitMQAddr, "test")
		Expect(err).To(BeNil())
	})

	Describe("Publish()", func() {
		It("publishes a message to queue", func() {
			Expect(qp.GetInputLength()).To(Equal(int64(0)))
			msg := &queue.Message{
				UUID:         "1234",
				Action:       "create",
				Event:        "image.created",
				ResourceName: "host",
				ResourceID:   "1",
			}
			err := qp.Publish(msg)
			Expect(err).To(BeNil())
			Expect(qp.GetInputLength()).To(Equal(int64(1)))
		})
	})

	Describe("PublishRetry()", func() {
		It("retries when failed to send msg", func() {
			msg := &queue.Message{
				UUID:         "1234",
				Action:       "create",
				Event:        "image.created",
				ResourceName: "host",
				ResourceID:   "1",
			}
			err := test.RunWhileRestartingRabbitMq(func() {
				err := qp.PublishRetry(msg, 11, 500*time.Millisecond)
				Expect(err).To(BeNil())
			})
			Expect(err).To(BeNil())
			Expect(qp.GetInputLength()).To(Equal(int64(1)))
		})

		It("retries until it successes when retry param is -1", func() {
			msg := &queue.Message{
				UUID:         "1234",
				Action:       "create",
				Event:        "image.created",
				ResourceName: "host",
				ResourceID:   "1",
			}
			err := test.RunWhileRestartingRedis(func() {
				err := qp.PublishRetry(msg, -1, 500*time.Millisecond)
				Expect(err).To(BeNil())
			})
			Expect(err).To(BeNil())
			Expect(qp.GetInputLength()).To(Equal(int64(1)))
		})
	})

	Describe("Consume", func() {
		It("consumes a message from queue", func() {
			qp.Publish(&queue.Message{
				UUID:         "1234",
				Action:       "create",
				Event:        "image.created",
				ResourceName: "host",
				ResourceID:   "1",
			})

			qc, err := queue.CreateAmqpConsumer(test.RabbitMQAddr, "test")
			Expect(err).To(BeNil())
			defer qc.(*queue.AmqpConsumer).Close()

			m, _, err := qc.Consume()
			Expect(err).To(BeNil())

			Expect(m.UUID).To(Equal("1234"))
			Expect(m.Action).To(Equal("create"))
			Expect(m.Event).To(Equal("image.created"))
			Expect(m.ResourceName).To(Equal("host"))
			Expect(m.ResourceID).To(Equal("1"))
			Expect(m.Payload).To(BeEmpty())
		})
	})

	Describe("Message", func() {
		var msg *queue.Message
		var d queue.Delivery

		Describe("Ack", func() {
			It("sends ack for the message to consume next message successfully", func() {
				err := qp.Publish(&queue.Message{
					UUID:         "1234",
					Action:       "create",
					Event:        "image.created",
					ResourceName: "host",
					ResourceID:   "1",
				})
				Expect(err).To(BeNil())
				err = qp.Publish(&queue.Message{
					UUID:         "4567",
					Action:       "create",
					Event:        "image.created",
					ResourceName: "host",
					ResourceID:   "2",
				})
				Expect(err).To(BeNil())

				qc, err := queue.CreateAmqpConsumer(test.RabbitMQAddr, "test")
				Expect(err).To(BeNil())
				defer qc.(*queue.AmqpConsumer).Close()

				msg, d, err = qc.Consume()
				Expect(err).To(BeNil())
				d.Ack(50 * time.Millisecond)

				nextMsg, _, err := qc.Consume()
				Expect(err).To(BeNil())
				Expect(nextMsg.UUID).To(Equal("4567"))
			})

			It("retries when it failes to send a ACK", func() {
				err := qp.Publish(&queue.Message{
					UUID:         "1234",
					Action:       "create",
					Event:        "image.created",
					ResourceName: "host",
					ResourceID:   "1",
				})
				Expect(err).To(BeNil())

				qc, err := queue.CreateAmqpConsumer(test.RabbitMQAddr, "test")
				Expect(err).To(BeNil())
				defer qc.(*queue.AmqpConsumer).Close()

				_, d, err = qc.Consume()
				Expect(err).To(BeNil())

				ackRes := make(chan error, 1)
				err = test.RunWhileRestartingRabbitMq(func() {
					ackRes <- d.Ack(1000 * time.Millisecond)
				})
				Expect(err).To(BeNil())
				select {
				case err = <-ackRes:
					Expect(err).To(BeNil())
				case <-time.After(1 * time.Second):
					Fail("timed out waiting for ack to finish")
				}
			})
		})

		Describe("Fail", func() {
			It("send a message to failed queue", func() {
				err := qp.Publish(&queue.Message{
					UUID:         "1234",
					Action:       "create",
					Event:        "image.created",
					ResourceName: "host",
					ResourceID:   "1",
				})
				Expect(err).To(BeNil())
				Expect(qp.GetFailedLength()).To(Equal(int64(0)))

				qc, err := queue.CreateAmqpConsumer(test.RabbitMQAddr, "test")
				Expect(err).To(BeNil())
				defer qc.(*queue.AmqpConsumer).Close()

				_, d, err = qc.Consume()
				Expect(err).To(BeNil())
				d.Fail()
				time.Sleep(100 * time.Millisecond)
				Expect(qp.GetFailedLength()).To(Equal(int64(1)))
			})
		})
	})

	Describe("StartAmqpConsumer", func() {
		It("starts queue consumer", func() {
			consumer := reexec.Command("amqp1")
			Expect(consumer.Start()).To(BeNil())
			defer consumer.Process.Kill()
			err := qp.Publish(&queue.Message{UUID: "1234"})
			Expect(err).To(BeNil())

			redisClient, err := test.PrepareRedis(test.RedisHost, test.RedisPort, test.RedisTestDb)
			Expect(err).To(BeNil())
			Eventually(func() string {
				return redisClient.Get("amqp-handler").Val()
			}, 10*time.Second).Should(Equal("amqp1"))
		})

		It("reconnects when the queue server restarts", func() {
			consumer := reexec.Command("amqp1")
			consumer.Stdout = os.Stdout
			consumer.Stderr = os.Stderr
			Expect(consumer.Start()).To(BeNil())
			defer consumer.Process.Kill()

			Expect(exec.Command("rabbitmqctl", "stop_app").Run()).To(BeNil())
			Expect(exec.Command("rabbitmqctl", "start_app").Run()).To(BeNil())

			err := qp.Publish(&queue.Message{UUID: "1234"})
			Expect(err).To(BeNil())

			redisClient, err := test.PrepareRedis(test.RedisHost, test.RedisPort, test.RedisTestDb)
			Expect(err).To(BeNil())
			Eventually(func() string {
				return redisClient.Get("amqp-handler").Val()
			}, 10*time.Second).Should(Equal("amqp1"))
		})

		It("finishes current job before stop", func() {
			r, w, err := os.Pipe()
			Expect(err).To(BeNil(), "failed to create pipe")
			q := reexec.Command("handle-amqp-stop")
			q.ExtraFiles = []*os.File{w} // send the pipe over as a sync pipe
			defer func() {
				r.Close()
				if q.Process != nil {
					q.Process.Kill()
				}
			}()

			Expect(q.Start()).To(BeNil(), "queue cannot start")
			w.Close()

			err = qp.Publish(&queue.Message{UUID: "1234"})
			Expect(err).To(BeNil())

			b := make([]byte, 1)
			n, err := r.Read(b)
			Expect(err).To(BeNil())
			Expect(n).To(Equal(1), "failed to receive synchronization byte")

			q.Process.Signal(syscall.SIGTERM)
			q.Process.Signal(syscall.SIGHUP)

			redisClient, err := test.PrepareRedis(test.RedisHost, test.RedisPort, test.RedisTestDb)
			Expect(err).To(BeNil())
			Eventually(func() string {
				return redisClient.Get("amqp-message").Val()
			}, 10*time.Second).Should(Equal("1234"))

			Eventually(func() error {
				return q.Wait()
			}, 10*time.Second).Should(BeNil())
		})

		It("tries when there is already queue consumer for same name", func() {
			queue1 := reexec.Command("amqp1")
			queue1.Stdout = os.Stdout
			queue1.Stderr = os.Stderr
			queue2 := reexec.Command("amqp2")
			queue2.Stdout = os.Stdout
			queue2.Stderr = os.Stderr
			defer func() {
				if queue1.Process != nil {
					queue1.Process.Kill()
				}
				if queue2.Process != nil {
					queue2.Process.Kill()
				}
			}()

			// start queue1
			Expect(queue1.Start()).To(BeNil(), "queue1 cannot start")

			// make sure that queue1 is running and accepting messages
			redisClient, err := test.PrepareRedis(test.RedisHost, test.RedisPort, test.RedisTestDb)
			Expect(err).To(BeNil())

			err = qp.Publish(&queue.Message{UUID: "1234"})
			Expect(err).To(BeNil())
			Eventually(func() string {
				return redisClient.Get("amqp-handler").Val()
			}, 10*time.Second).Should(Equal("amqp1"))

			// start queue2. it should block waiting for the queue to finish
			Expect(queue2.Start()).To(BeNil(), "queue2 cannot start")

			// stop queue1
			time.Sleep(500 * time.Millisecond)
			queue1.Process.Kill()
			time.Sleep(500 * time.Millisecond)

			err = qp.Publish(&queue.Message{UUID: "1234"})
			Expect(err).To(BeNil())
			Eventually(func() string {
				return redisClient.Get("amqp-handler").Val()
			}, 10*time.Second).Should(Equal("amqp2"))
		})

		It("submits a failed message to failed queue", func() {
			var err error
			consumer := reexec.Command("handle-failure")
			Expect(consumer.Start()).To(BeNil())
			defer consumer.Process.Kill()

			err = qp.Publish(&queue.Message{UUID: "failure"})
			Expect(err).To(BeNil())
			err = qp.Publish(&queue.Message{UUID: "success"})
			Expect(err).To(BeNil())

			redisClient, err := test.PrepareRedis(test.RedisHost, test.RedisPort, test.RedisTestDb)
			Expect(err).To(BeNil())
			// expect to received 1 failure message and 1 success message
			Eventually(func() string {
				return redisClient.Get("amqp-handler").Val()
			}, 10*time.Second).Should(Equal("handle-failure"))
			Eventually(func() int64 {
				return qp.GetFailedLength()
			}, 10*time.Second).Should(Equal(int64(1)))
		})
	})
})
