package digitalocean

import (
	"fmt"
	"log"
	"time"
)

const StateWaitTime = 3 * time.Second

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
