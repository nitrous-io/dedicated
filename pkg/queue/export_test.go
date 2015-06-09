package queue

import "time"

func (qc *QueueConsumer) SetHandlerTimeout(timeout time.Duration) {
	qc.handlerTimeout = timeout
}

func (qc *QueueConsumer) GetHandlerTimeout() time.Duration {
	return qc.handlerTimeout
}
