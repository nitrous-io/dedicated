package queue

import "time"

func SetConsumeTimeout(timeout time.Duration) {
	consumeTimeout = timeout
}

func GetConsumeTimeout() time.Duration {
	return consumeTimeout
}
