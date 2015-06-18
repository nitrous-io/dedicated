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
	// stop redis before publish and then restart when it's retrying
	err := exec.Command("service", "redis_6379", "stop").Run()
	if err != nil {
		return err
	}
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		time.Sleep(100 * time.Millisecond)
		exec.Command("service", "redis_6379", "start").Run()
		var err error
		for i := 0; i < 5; i++ {
			_, err = net.Dial("tcp", fmt.Sprintf("%s:%s", RedisHost, RedisPort))
			if err != nil {
				logrus.Printf("failed to connect to redis: %+v", err)
				time.Sleep(10 * time.Millisecond)
			} else {
				break
			}
		}
		wg.Done()
	}()

	fn()
	wg.Wait()
	return nil
}
