package digitalocean

import (
	"fmt"
	"log"
	"time"
)

const StateWaitTime = 3 * time.Second

// WaitForEvent waits until the event is completed i.e. status is done and
// percentage is 100
func WaitForEvent(eventID int, c *Client, sleep time.Duration, timeout time.Duration) error {
	// control channel to make sure that we can exit goroutine properly
	done := make(chan struct{})
	defer close(done)

	result := make(chan error, 1)
	go func() {
		var ev *Event
		var err error
		for {
			ev, err = c.Event(eventID)
			if err != nil {
				result <- err
				return
			}
			log.Printf("Waiting for event %d to complete, got status %s, progress %s",
				eventID, ev.Status, ev.Percentage)
			if ev.Status == "done" && ev.Percentage == "100" {
				result <- nil
				return
			}
			time.Sleep(sleep)
			select {
			case <-done:
				// We finished, so just exit the goroutine
				return
			default:
				// continue
			}
		}
	}()
	select {
	case err := <-result:
		return err
	case <-time.After(timeout):
		return fmt.Errorf("timeout while waiting to for event %d to complete",
			eventID)
	}
}

// waitForState simply blocks until the droplet is in
// a state we expect, while eventually timing out.
func WaitForState(
	desiredState string,
	dropletID int,
	client *Client,
	sleep time.Duration,
	timeout time.Duration) (*Droplet, error) {
	// control channel to make sure that we can exit goroutine properly
	done := make(chan struct{})
	defer close(done)

	result := make(chan error, 1)
	var droplet *Droplet
	go func() {
		var err error
		for {
			droplet, err = client.DropletInfo(dropletID)
			if err != nil {
				result <- err
				return
			}
			log.Printf("Waiting for droplet %v to have state %v, got %v",
				dropletID, desiredState, droplet.Status)
			if droplet.Status == desiredState {
				result <- nil
				return
			}
			time.Sleep(sleep)
			select {
			case <-done:
				// We finished, so just exit the goroutine
				return
			default:
				// continue
			}
		}
	}()
	select {
	case err := <-result:
		return droplet, err
	case <-time.After(timeout):
		return nil, fmt.Errorf("Timeout while waiting to for droplet to become '%s'",
			desiredState)
	}
}
