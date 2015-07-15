package test

import (
	"fmt"
	"net"
	"os/exec"
	"sync"
	"time"

	"github.com/Sirupsen/logrus"
	"gopkg.in/redis.v2"
)

var RedisTestDb int64 = 12
var RedisHost string = "localhost"
var RedisPort string = "6379"
var RabbitMQAddr = "amqp://localhost:5672"

func PrepareRedis(redisHost string, redisPort string, redisDB int64, reset bool) (*redis.Client, error) {
	redisClient := redis.NewTCPClient(&redis.Options{
		Addr:     fmt.Sprintf("%s:%s", redisHost, redisPort),
		Password: "",
		DB:       redisDB,
	})
	err := redisClient.Ping().Err()
	if err != nil {
		redisClient.Close()
		return nil, err
	}

	if reset {
		err = redisClient.FlushDb().Err()
		if err != nil {
			redisClient.Close()
			return nil, err
		}
	}

	return redisClient, nil
}

func RunWhileRestartingRedis(fn func()) error {
	return runWhileRestartService("redis_6379", func() error {
		_, err := net.Dial("tcp", fmt.Sprintf("%s:%s", RedisHost, RedisPort))
		return err
	}, fn)
}

func RunWhileRestartingRabbitMq(fn func()) error {
	return runWhileRestartService("rabbitmq-server", func() error {
		_, err := net.Dial("tcp", fmt.Sprintf("%s:%s", "localhost", "5672"))
		return err
	}, fn)
}

func RestartRabbitMq() {
	exec.Command("rabbitmqctl", "stop_app").Run()
	exec.Command("rabbitmqctl", "start_app").Run()
}

func runWhileRestartService(service string, checkFn func() error, fn func()) error {
	// stop redis before publish and then restart when it's retrying
	var err error
	var output []byte

	switch service {
	case "rabbitmq-server":
		output, err = exec.Command("rabbitmqctl", "stop_app").CombinedOutput()
	case "redis_6379":
		output, err = exec.Command("service", "redis_6379", "stop").CombinedOutput()
	default:
		return fmt.Errorf("Expected service: %s", service)
	}

	if err != nil {
		logrus.Println("failed to stop", service, string(output))
		return err
	}

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		time.Sleep(100 * time.Millisecond)
		switch service {
		case "rabbitmq-server":
			exec.Command("rabbitmqctl", "start_app").Run()
		case "redis_6379":
			exec.Command("service", service, "start").Run()
		default:
			return
		}

		for i := 0; i < 5; i++ {
			if err := checkFn(); err != nil {
				logrus.Printf("failed to connect to %s: %+v", service, err)
				time.Sleep(100 * time.Millisecond)
			} else {
				break
			}
		}
		logrus.Println("started ", service)
		wg.Done()
	}()

	fn()
	wg.Wait()
	return nil
}
