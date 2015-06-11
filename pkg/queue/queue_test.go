package queue_test

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"syscall"
	"testing"
	"time"

	"gopkg.in/redis.v2"

	"github.com/adjust/redismq"
	"github.com/nitrous-io/dedicated/pkg/queue"
	"github.com/nitrous-io/dedicated/pkg/reexec"
	"github.com/nitrous-io/dedicated/test"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func init() {
	reexec.Register("queue1", func() {
		stop := make(chan struct{})
		h := func(m *queue.Message) error {
			redisClient, err := test.PrepareRedis(test.RedisHost, test.RedisPort, test.RedisTestDb)
			if err != nil {
				return err
			}
			redisClient.Set("test", "handler1")
			return nil
		}
		err := queue.StartConsumer(test.RedisHost, test.RedisPort, test.RedisTestDb, "test", h, stop, 5*time.Second)
		log.Fatalf("queue1 failed to start %v", err)
	})

	reexec.Register("queue2", func() {
		stop := make(chan struct{})
		h := func(m *queue.Message) error {
			redisClient, err := test.PrepareRedis(test.RedisHost, test.RedisPort, test.RedisTestDb)
			if err != nil {
				return err
			}
			redisClient.Set("test", "handler2")
			return nil
		}
		err := queue.StartConsumer(test.RedisHost, test.RedisPort, test.RedisTestDb, "test", h, stop, 5*time.Second)
		log.Fatalf("queue2 failed to start %v", err)
	})

	reexec.Register("handle-stop", func() {
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
			redisClient.Set("test", m.UUID)
			return nil
		}

		go queue.StartConsumer(test.RedisHost, test.RedisPort, test.RedisTestDb, "test", h, stop, 5*time.Second)

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

	reexec.Register("queue-timeout", func() {
		stop := make(chan struct{})
		h := func(m *queue.Message) error {
			if m.UUID == "success" {
				redisClient, err := test.PrepareRedis(test.RedisHost, test.RedisPort, test.RedisTestDb)
				if err != nil {
					return err
				}
				redisClient.Set("test", m.UUID)
			} else {
				time.Sleep(5 * time.Second)
			}
			return nil
		}
		err := queue.StartConsumer(test.RedisHost, test.RedisPort, test.RedisTestDb, "test", h, stop, 100*time.Millisecond)
		log.Fatalf("queue-timeout failed to start %v", err)
	})

	// return if running in reexec mode, and the reexec func has already finished
	if reexec.Init() {
		return
	}
}

func Test(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "pkg/queue")
}

var _ = Describe("pkg/queue", func() {
	var qp queue.Publisher
	var redisClient *redis.Client

	BeforeEach(func() {
		output, err := exec.Command("service", "redis_6379", "start").CombinedOutput()
		Expect(err).To(BeNil(), "failed to start redis")
		if !strings.Contains(string(output), "already running") {
			time.Sleep(100 * time.Millisecond)
		}

		redisClient, err = test.PrepareRedis(test.RedisHost, test.RedisPort, test.RedisTestDb)
		Expect(err).To(BeNil())
	})

	AfterEach(func() {
		if qp != nil {
			qp.Delete()
		}
		redisClient.Close()
	})

	Describe("QueuePublisher", func() {
		Describe("Publish()", func() {
			BeforeEach(func() {
				qp = queue.NewQueue(test.RedisHost, test.RedisPort, test.RedisTestDb, "test")
				err := qp.ResetInput()
				Expect(err).To(BeNil())
			})

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
			var msg *queue.Message
			BeforeEach(func() {
				qp = queue.NewQueue(test.RedisHost, test.RedisPort, test.RedisTestDb, "test")
				err := qp.ResetInput()
				Expect(err).To(BeNil())
				msg = &queue.Message{
					UUID:         "1234",
					Action:       "create",
					Event:        "image.created",
					ResourceName: "host",
					ResourceID:   "1",
				}
			})

			It("ignore ratelimit error from redis library", func() {
				err := test.RunWhileRestartingRedis(func() {
					err := qp.PublishRetry(msg, -1, 5*time.Millisecond)
					Expect(err).To(BeNil())
				})
				Expect(err).To(BeNil())
				Expect(qp.GetInputLength()).To(Equal(int64(1)))
			})

			It("retries when failed to send msg to redis", func() {
				err := test.RunWhileRestartingRedis(func() {
					err := qp.PublishRetry(msg, 11, 50*time.Millisecond)
					Expect(err).To(BeNil())
				})
				Expect(err).To(BeNil())
				Expect(qp.GetInputLength()).To(Equal(int64(1)))
			})

			It("retries until it successes when retry param is -1", func() {
				err := test.RunWhileRestartingRedis(func() {
					err := qp.PublishRetry(msg, -1, 50*time.Millisecond)
					Expect(err).To(BeNil())
				})
				Expect(err).To(BeNil())
				Expect(qp.GetInputLength()).To(Equal(int64(1)))
			})
		})
	})

	Describe("QueueConsumer", func() {
		var qc queue.Consumer
		var err error
		var noopHandler = func(msg *queue.Message) error {
			return nil
		}

		BeforeEach(func() {
			redismqQueue := redismq.CreateQueue(test.RedisHost, test.RedisPort, "", test.RedisTestDb, "test")
			qc, err = queue.CreateConsumer(redismqQueue, "test", noopHandler, 5*time.Second)
			Expect(err).To(BeNil())
			qp = queue.NewQueue(test.RedisHost, test.RedisPort, test.RedisTestDb, "test")
			err := qp.ResetInput()
			Expect(err).To(BeNil())
			msg := &queue.Message{
				UUID:         "1234",
				Action:       "create",
				Event:        "image.created",
				ResourceName: "host",
				ResourceID:   "1",
			}
			err = qp.Publish(msg)
			Expect(err).To(BeNil())
		})

		AfterEach(func() {
			if qp != nil {
				qp.Delete()
			}
		})

		Describe("Consume", func() {
			It("consumes a message from queue", func() {
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
	})

	Describe("Message", func() {
		var qc queue.Consumer
		var err error
		var msg *queue.Message
		var d queue.Delivery
		var noopHandler = func(msg *queue.Message) error {
			return nil
		}

		BeforeEach(func() {
			redismqQueue := redismq.CreateQueue(test.RedisHost, test.RedisPort, "", test.RedisTestDb, "test")
			qc, err = queue.CreateConsumer(redismqQueue, "test", noopHandler, 5*time.Second)
			Expect(err).To(BeNil())
			qp = queue.NewQueue(test.RedisHost, test.RedisPort, test.RedisTestDb, "test")
			err = qp.ResetInput()
			Expect(err).To(BeNil())
		})

		AfterEach(func() {
			if qp != nil {
				qp.Delete()
			}
		})

		Describe("Ack", func() {
			BeforeEach(func() {
				err = qp.Publish(&queue.Message{
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
				msg, d, err = qc.Consume()
				Expect(err).To(BeNil())
			})

			It("sends ask for the message to consume next message successfully", func() {
				d.Ack(50 * time.Millisecond)
				nextMsg, _, err := qc.Consume()
				Expect(err).To(BeNil())
				Expect(nextMsg.UUID).To(Equal("4567"))
			})

			It("retries when it failes to send a ACK", func() {
				err := test.RunWhileRestartingRedis(func() {
					d.Ack(50 * time.Millisecond)
				})
				Expect(err).To(BeNil())
				nextMsg, _, err := qc.Consume()
				Expect(err).To(BeNil())
				Expect(nextMsg.UUID).To(Equal("4567"))
			})
		})

		Describe("Fail", func() {
			It("send a message to failed queue", func() {
				err = qp.Publish(&queue.Message{
					UUID:         "1234",
					Action:       "create",
					Event:        "image.created",
					ResourceName: "host",
					ResourceID:   "1",
				})
				Expect(err).To(BeNil())
				Expect(qp.GetFailedLength()).To(Equal(int64(0)))
				_, d, err := qc.Consume()
				Expect(err).To(BeNil())
				d.Fail()
				Expect(qp.GetFailedLength()).To(Equal(int64(1)))
			})
		})
	})

	Describe("StartConsumer", func() {
		It("starts queue consumer", func() {
			stopCh := make(chan struct{})

			handler := func(m *queue.Message) error {
				redisClient.Set("test", m.UUID)
				return nil
			}

			go queue.StartConsumer(test.RedisHost, test.RedisPort, test.RedisTestDb, "test", handler, stopCh, 5*time.Second)
			time.Sleep(50 * time.Millisecond)

			qp = queue.NewQueue(test.RedisHost, test.RedisPort, test.RedisTestDb, "test")
			err := qp.ResetInput()
			Expect(err).To(BeNil())

			err = qp.Publish(&queue.Message{
				UUID: "1234",
			})
			Expect(err).To(BeNil())
			time.Sleep(50 * time.Millisecond)
			Expect(redisClient.Get("test").Val()).To(Equal("1234"))
			stopCh <- struct{}{}
			select {
			case _ = <-stopCh:
				err = qp.Publish(&queue.Message{
					UUID: "4567",
				})
				Expect(err).To(BeNil())
				time.Sleep(50 * time.Millisecond)
				Expect(redisClient.Get("test").Val()).To(Equal("1234"))
			case <-time.After(50 * time.Millisecond):
				Fail("Timeout waiting for queue consumer to be down")
			}
		})

		It("finishes current job before stop", func() {
			r, w, err := os.Pipe()
			Expect(err).To(BeNil(), "failed to create pipe")
			q := reexec.Command("handle-stop")
			q.ExtraFiles = []*os.File{w} // send the pipe over as a sync pipe
			defer func() {
				r.Close()
				if q.Process != nil {
					q.Process.Kill()
				}
			}()

			Expect(q.Start()).To(BeNil(), "queue cannot start")
			w.Close()

			qp = queue.NewQueue(test.RedisHost, test.RedisPort, test.RedisTestDb, "test")
			err = qp.Publish(&queue.Message{UUID: "1234"})
			Expect(err).To(BeNil())

			b := make([]byte, 1)
			n, err := r.Read(b)
			Expect(err).To(BeNil())
			Expect(n).To(Equal(1), "failed to receive synchronization byte")

			q.Process.Signal(syscall.SIGTERM)
			q.Process.Signal(syscall.SIGHUP)

			Eventually(func() error {
				return q.Wait()
			}, 1*time.Second).Should(BeNil())

			Eventually(func() string {
				return redisClient.Get("test").Val()
			}, 1*time.Second).Should(Equal("1234"))
		})

		It("tries when there is already queue consumer for same name", func() {
			queue1 := reexec.Command("queue1")
			queue2 := reexec.Command("queue2")
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
			qp = queue.NewQueue(test.RedisHost, test.RedisPort, test.RedisTestDb, "test")
			err := qp.Publish(&queue.Message{UUID: "1234"})
			Expect(err).To(BeNil())
			Eventually(func() string {
				return redisClient.Get("test").Val()
			}, 10*time.Second).Should(Equal("handler1"))

			// start queue2. it should block waiting for the queue to finish
			Expect(queue2.Start()).To(BeNil(), "queue2 cannot start")

			// stop queue1
			queue1.Process.Kill()
			time.Sleep(500 * time.Millisecond)

			err = qp.Publish(&queue.Message{UUID: "1234"})
			Expect(err).To(BeNil())
			Eventually(func() string {
				return redisClient.Get("test").Val()
			}, 10*time.Second).Should(Equal("handler2"))
		})

		It("submits a failed message to failed queue", func() {
			stopCh := make(chan struct{})

			handler := func(m *queue.Message) error {
				if m.UUID == "success" {
					redisClient.Set("test", m.UUID)
					return nil
				} else {
					return fmt.Errorf("Error!!!")
				}
			}

			go queue.StartConsumer(test.RedisHost, test.RedisPort, test.RedisTestDb, "test", handler, stopCh, 5*time.Minute)
			time.Sleep(50 * time.Millisecond)

			qp = queue.NewQueue(test.RedisHost, test.RedisPort, test.RedisTestDb, "test")
			err := qp.ResetInput()
			Expect(err).To(BeNil())

			Expect(qp.GetFailedLength()).To(Equal(int64(0)))
			err = qp.Publish(&queue.Message{
				UUID: "1234",
			})
			Expect(err).To(BeNil())
			time.Sleep(50 * time.Millisecond)
			Expect(qp.GetFailedLength()).To(Equal(int64(1)))

			// It still process next message
			err = qp.Publish(&queue.Message{
				UUID: "success",
			})
			Expect(err).To(BeNil())
			time.Sleep(50 * time.Millisecond)
			Expect(redisClient.Get("test").Val()).To(Equal("success"))

			stopCh <- struct{}{}
			select {
			case _ = <-stopCh:
			case <-time.After(100 * time.Millisecond):
				Fail("Timeout waiting for queue consumer to be down")
			}
		})

		It("submits a timed out message to failed queue start consuming next message", func() {
			q := reexec.Command("queue-timeout")
			q.Stdout = os.Stdout
			q.Stderr = os.Stderr

			defer func() {
				if q.Process != nil {
					q.Process.Kill()
				}
			}()

			// start queue
			Expect(q.Start()).To(BeNil(), "queue-timeout cannot start")

			qp = queue.NewQueue(test.RedisHost, test.RedisPort, test.RedisTestDb, "test")

			Expect(qp.GetFailedLength()).To(Equal(int64(0)))
			err := qp.Publish(&queue.Message{
				UUID: "1234",
			})
			Expect(err).To(BeNil())

			// Wait until timing out
			time.Sleep(1 * time.Second)
			Expect(qp.GetFailedLength()).To(Equal(int64(1)))

			// It still process next message
			err = qp.Publish(&queue.Message{
				UUID: "success",
			})
			Expect(err).To(BeNil())
			time.Sleep(50 * time.Millisecond)
			Expect(redisClient.Get("test").Val()).To(Equal("success"))
		})
	})
})
