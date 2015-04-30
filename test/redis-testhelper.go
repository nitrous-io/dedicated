package test

import (
	"fmt"

	"gopkg.in/redis.v2"
)

var RedisTestDb int64 = 12
var RedisHost string = "localhost"
var RedisPort string = "6379"

func ConnectToRedisClient() (*redis.Client, error) {
	redisClient := redis.NewTCPClient(&redis.Options{
		Addr:     fmt.Sprintf("%s:%s", RedisHost, RedisPort),
		Password: "",
		DB:       RedisTestDb,
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
