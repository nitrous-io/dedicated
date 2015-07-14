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

func PrepareRedis(redisHost string, redisPort string, redisDB int64) (*redis.Client, error) {
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

	err = redisClient.FlushDb().Err()
	if err != nil {
		redisClient.Close()
		return nil, err
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

func runWhileRestartService(service string, checkFn func() error, fn func()) error {
	// stop redis before publish and then restart when it's retrying
	output, err := exec.Command("service", service, "stop").CombinedOutput()
	if err != nil {
		logrus.Println("failed to stop", service, string(output))
		return err
	}
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		time.Sleep(100 * time.Millisecond)
		exec.Command("service", service, "start").Run()
		for i := 0; i < 5; i++ {
			if err := checkFn(); err != nil {
				logrus.Printf("failed to connect to %s: %+v", service, err)
				time.Sleep(10 * time.Millisecond)
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
