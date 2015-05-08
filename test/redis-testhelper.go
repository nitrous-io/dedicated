package test

import (
	"fmt"
	"os/exec"
	"sync"
	"time"

	"gopkg.in/redis.v2"
)

type subject func()

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

func RunWhileRestartingRedis(fn subject) error {
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
		time.Sleep(10 * time.Millisecond) // wait until redis is up.
		wg.Done()
	}()

	fn()
	wg.Wait()
	return nil
}
