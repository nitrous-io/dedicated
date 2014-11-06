package gce

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"code.google.com/p/google-api-go-client/compute/v1"
)

// stateRefreshFunc is used to refresh the state of a thing and is
// used in conjunction with waitForState.
type stateRefreshFunc func() (string, error)

func (h *Host) refreshOp(zone string, op *compute.Operation) stateRefreshFunc {
	return func() (string, error) {
		newOp, err := h.service.ZoneOperations.Get(h.projectID, zone, op.Name).Do()
		if err != nil {
			return "", err
		}

		// If the op is done, check for errors
		err = nil
		if newOp.Status == "DONE" {
			if newOp.Error != nil {
				var errMsgs []string
				for _, e := range newOp.Error.Errors {
					errMsgs = append(errMsgs, e.Message)
				}

				err = errors.New(strings.Join(errMsgs, "\n"))
			}
		}

		return newOp.Status, err
	}
}

func (h *Host) WaitForInstance(state, zone, name string) error {
	errCh := make(chan error, 1)
	go waitForState(errCh, state, h.refreshInstanceState(zone, name))
	select {
	case err := <-errCh:
		return err
	case <-time.After(StateTimeout):
		return fmt.Errorf("timeout while waiting for instance %q to be running", h.instance.Name)
	}
}

func (h *Host) refreshInstanceState(zone, name string) stateRefreshFunc {
	return func() (string, error) {
		instance, err := h.service.Instances.Get(h.projectID, zone, name).Do()
		if err != nil {
			return "", err
		}
		return instance.Status, nil
	}
}

// waitForState will spin in a loop forever waiting for state to
// reach a certain target.
func waitForState(errCh chan<- error, target string, refresh stateRefreshFunc) {
	for {
		state, err := refresh()
		if err != nil {
			errCh <- err
			return
		}
		if state == target {
			errCh <- nil
			return
		}

		time.Sleep(StateWaitTime)
	}
}
