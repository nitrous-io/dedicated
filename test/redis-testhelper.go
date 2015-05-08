package test

import (
	"fmt"

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
