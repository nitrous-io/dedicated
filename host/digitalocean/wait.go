package digitalocean

import (
	"fmt"
	"time"
)

const StateWaitTime = 3 * time.Second

// waitForState simply blocks until the droplet is in
// a state we expect, while eventually timing out.
func waitForState(
	desiredState string,
	dropletId uint,
	client *DigitalOceanClient,
	timeout time.Duration) error {
	// control channel to make sure that we can exit goroutine properly
	done := make(chan struct{})
	defer close(done)

	result := make(chan error, 1)
	go func() {
		attempts := 0
		for {
			attempts += 1
			_, status, err := client.DropletStatus(dropletId)
			if err != nil {
				result <- err
				return
			}
			// got the correct state
			if status == desiredState {
				result <- nil
				return
			}

			time.Sleep(StateWaitTime)
			select {
			case <-done:
				// We finished, so just exit the goroutine
				return
			}
		}
	}()
	select {
	case err := <-result:
		return err
	case <-time.After(timeout):
		err := fmt.Errorf("Timeout while waiting to for droplet to become '%s'", desiredState)
		return err
	}
}
