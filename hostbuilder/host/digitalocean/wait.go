package digitalocean

import (
	"fmt"
	"log"
	"time"

	"github.com/digitalocean/godo"
)

const (
	inProgressStatus = "in-progress"
	erroredStatus    = "errored"
	completedStatus  = "completed"
)

// WaitForActions waits until the action is completed
func WaitForAction(service godo.ActionsService, id int, wait, timeout time.Duration) (*godo.Action, error) {
	// control channel to make sure that we can exit goroutine properly
	done := make(chan struct{})
	defer close(done)

	result := make(chan error, 1)
	var finalAction *godo.Action
	go func() {
		for {
			select {
			case <-done:
				// We finished, so just exit the goroutine
				return
			default:
				tmp, _, err := service.Get(id)
				if err != nil {
					result <- err
					return
				}
				log.Printf("Waiting for event %d to complete, got status %s", id, tmp.Status)
				if tmp.Status == erroredStatus {
					result <- fmt.Errorf("action %v failed", tmp)
					return
				} else if tmp.Status == completedStatus {
					finalAction = tmp
					result <- nil
					return
				}
				time.Sleep(wait)
			}
		}
	}()
	select {
	case err := <-result:
		if err != nil {
			return nil, err
		}
		return finalAction, nil
	case <-time.After(timeout):
		return nil, fmt.Errorf("timeout while waiting to for event %d to complete", id)
	}
}
