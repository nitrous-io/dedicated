package redispool

import (
	"time"

	"github.com/garyburd/redigo/redis"
)

const (
	DefaultPoolSize    = 5
	DefaultAddress     = ":6379"
	DefaultIdleTimeout = 5 * time.Minute
)

type Pool struct {
	*redis.Pool
}

func New(address string) *Pool {
	redisAddr := DefaultAddress
	if address != "" {
		redisAddr = address
	}

	pool := &redis.Pool{
		MaxIdle:     DefaultPoolSize,
		IdleTimeout: DefaultIdleTimeout,
		Dial: func() (redis.Conn, error) {
			return redis.Dial("tcp", redisAddr)
		},
		TestOnBorrow: func(c redis.Conn, t time.Time) error {
			_, err := c.Do("PING")
			return err
		},
	}

	return &Pool{pool}
}
