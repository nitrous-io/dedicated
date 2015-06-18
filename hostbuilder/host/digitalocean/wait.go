package digitalocean

import (
	"fmt"
	"time"

	"github.com/Sirupsen/logrus"
	"github.com/digitalocean/godo"
	"golang.org/x/net/context"
)

const (
	inProgressStatus = "in-progress"
	erroredStatus    = "errored"
	completedStatus  = "completed"
)

// WaitForAction waits until the action is completed.
func WaitForAction(ctx context.Context, service godo.ActionsService, id int, wait time.Duration, cxt map[string]interface{}) (*godo.Action, error) {
	baseString := ""
	for k, v := range cxt {
		baseString += fmt.Sprintf("%s=%v ", k, v)
	}

	result := make(chan error, 1)
	var finalAction *godo.Action
	go func() {
		ticker := time.NewTicker(wait)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				action, _, err := service.Get(id)
				if err != nil {
					result <- err
					return
				}
				logrus.Debugf("%sWaiting for event %d to complete, got status %s", baseString, id, action.Status)
				switch action.Status {
				case erroredStatus:
					result <- fmt.Errorf("%saction %v failed", baseString, action)
					return
				case completedStatus:
					finalAction = action
					result <- nil
					return
				}
			}
		}
	}()

	select {
	case <-ctx.Done():
		switch ctx.Err() {
		case context.DeadlineExceeded:
			return nil, fmt.Errorf("%stimeout while waiting to for event %d to complete", baseString, id)
		case context.Canceled:
			return nil, fmt.Errorf("%scanceled wait for event %d to complete", baseString, id)
		}
		return nil, ctx.Err()
	case err := <-result:
		if err != nil {
			return nil, err
		}
		return finalAction, nil
	}
}
