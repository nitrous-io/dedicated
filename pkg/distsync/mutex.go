package distsync

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"time"

	"github.com/garyburd/redigo/redis"
)

const (
	// DefaultExpiry is used when Mutex Expiry is 0.
	DefaultExpiry = 10 * time.Second
	// DefaultTries is used when Mutex Tries is 0.
	DefaultTries = 5
	// DefaultDelay is used when Mutex Delay is 0.
	DefaultDelay = 500 * time.Millisecond
)

var ErrLockFailed = errors.New("failed to acquire lock")

type Mutex struct {
	Name string
	// Expiry is the duration for which the lock is valid.
	Expiry time.Duration
	// Tries is the number of attempts to acquire lock.
	Tries int
	// Delay is the delay between 2 tries to acquire lock.
	Delay time.Duration

	redisPool *redis.Pool

	// value is the value that we SET in Redis (i.e. "SET .Name .value").
	value string
}

func NewMutex(name string, address string) *Mutex {
	pool := &redis.Pool{
		MaxIdle:     3,
		IdleTimeout: 240 * time.Second,
		Dial: func() (redis.Conn, error) {
			return redis.Dial("tcp", address)
		},
		TestOnBorrow: func(c redis.Conn, t time.Time) error {
			_, err := c.Do("PING")
			return err
		},
	}

	return &Mutex{
		Name:      name,
		redisPool: pool,
	}
}

func (mu *Mutex) Lock() error {
	expiry := mu.Expiry
	if expiry == 0 {
		expiry = DefaultExpiry
	}

	delay := mu.Delay
	if delay == 0 {
		delay = DefaultDelay
	}

	tries := mu.Tries
	if tries == 0 {
		tries = DefaultTries
	}

	// Generate a random value to SET. When releasing the lock, we will send a
	// script to Redis that only removes the key we previously SET if the value
	// matches. This ensures that we will not incorrectly delete a key created
	// by another client that acquired the lock later.
	// See http://redis.io/commands/set. Note that as detailed on that page,
	// http://redis.io/topics/distlock is a better distributed lock
	// implementation, but we're not using it because we want to only run 1
	// instance of Redis at this time.
	b := make([]byte, 16)
	_, err := rand.Read(b)
	if err != nil {
		return err
	}
	value := base64.StdEncoding.EncodeToString(b)

	conn := mu.redisPool.Get()
	defer conn.Close()

	for i := 0; i < tries; i++ {
		reply, err := redis.String(conn.Do("SET", mu.Name, value, "NX", "EX", int(expiry.Seconds())))
		if err != nil {
			time.Sleep(delay)
			continue
		}

		if reply != "OK" {
			time.Sleep(delay)
			continue
		}

		// Save random value so we can safely delete the key on Unlock().
		mu.value = value

		return nil
	}

	return ErrLockFailed
}

func (mu *Mutex) Unlock() {
	value := mu.value
	if value == "" {
		panic("distsync: unlock of unlocked mutex") // Corollary to sync.Mutex's Unlock().
	}

	mu.value = "" // Reset random value for next Lock(().

	conn := mu.redisPool.Get()
	delScript.Do(conn, mu.Name, value)
	conn.Close()
}

var delScript = redis.NewScript(1, `
if redis.call("get", KEYS[1]) == ARGV[1] then
	return redis.call("del", KEYS[1])
else
	return 0
end`)
