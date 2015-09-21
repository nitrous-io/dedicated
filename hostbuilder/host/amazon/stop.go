package amazon

import (
	"fmt"
	"time"

	"github.com/Sirupsen/logrus"
	"github.com/mitchellh/goamz/ec2"
	"github.com/nitrous-io/dedicated/event"
	"github.com/nitrous-io/dedicated/hostbuilder/host"
	"github.com/nitrous-io/dedicated/hostbuilder/state"
	mh "github.com/nitrous-io/dedicated/pkg/metricshelper"
	"github.com/nitrous-io/dedicated/pkg/queue"
	"golang.org/x/net/context"
)

func (h *Host) StopWithQueue(qp queue.Publisher, queueInfo *host.QueueInfo) (*host.Info, error) {
	label := h.MetricsLabel("stop")
	defer mh.UpdateSince(label, time.Now())

	host.PublishMessage(qp, event.HostStopping, h.requestID, queueInfo, "Stopping host", queue.Stop)

	if err := h.reloadInstance(h.Config.ID); err != nil {
		h.publishHostError(qp, queueInfo, event.HostStopFailed, queue.Stop, err)
		return nil, mh.Error(err, label)
	}

	// [Feature flag] - requires EBS hotswapping.
	if h.Config.Hotswap {
		h.Printf("stopping hotswap instance")
		return h.stopWithPool(qp, queueInfo)
	}

	h.Printf("stopping pre-EBS hotswapping instance normally")

	if _, err := h.provider.StopInstances(h.instance.InstanceId); err != nil {
		h.publishHostError(qp, queueInfo, event.HostStopFailed, queue.Stop, err)
		return nil, mh.Error(err, label)
	}

	state := StateChangeConf{
		Pending: []string{"pending", "running", "stopping"},
		Refresh: instanceStateRefreshFunc(h.provider, h.instance),
		Target:  "stopped",
	}
	updatedInstance, err := WaitForState(&state)
	if err != nil {
		h.publishHostError(qp, queueInfo, event.HostStopFailed, queue.Stop, err)
		return nil, mh.Error(err, label)
	}
	h.instance = updatedInstance.(*ec2.Instance)
	info, err := h.Info()
	if err == nil {
		host.PublishHost(qp, event.HostStopped, h.requestID, queueInfo, info, queue.Stop, "")
	} else {
		logrus.Debugf("[ERROR] uuid=%s amazon: failed to send a message due to failure on getting host info: %v", h.requestID, err)
	}

	return info, err
}

func (h *Host) stopWithPool(qp queue.Publisher, queueInfo *host.QueueInfo) (*host.Info, error) {
	label := h.MetricsLabel("stop_with_pool")
	defer mh.UpdateSince(label, time.Now())

	var (
		ctx    context.Context
		cancel context.CancelFunc
	)
	ctx, cancel = context.WithTimeout(context.Background(), stopTimeout)
	defer cancel()

	userVolume, err := h.getVolumeByDeviceName(userDeviceName)
	if err != nil {
		return nil, mh.Error(err, label)
	}

	hctx := host.NewContext(ctx, h, h, nil)
	defer func() {
		if hctx.Executor != nil {
			hctx.Executor.Close()
		}
	}()

	steps := []host.Step{
		{
			Description: "Connecting to workspace",
			Func:        host.WaitForSSH,
			Timeout:     3 * time.Minute,
		},
		{
			Description: "Unmounting user volume",
			Func:        unmountUserVolume,
		},
		{
			Description: "Detaching user volume",
			Func:        detachUserVolume,
		},
		{
			Description: "Disassociate Elastic IP",
			Func:        detachElasticIP,
		},
	}
	for _, step := range steps {
		if err := hctx.Run(step); err != nil {
			h.Printf("ERROR: failed in step %q: %v", step.Description, err)

			// Attempt to recover workspace back to its original condition
			// prior to being stopped.
			if err := h.recoverFromStop(); err != nil {
				h.Printf("failed to recover from stop failure, err: %v", err)
			}

			h.publishHostError(qp, queueInfo, event.HostStopFailed, queue.Stop, err)

			if err == context.DeadlineExceeded {
				return nil, ErrTimeout
			}
			return nil, mh.Error(err, label)
		}

		host.PublishMessage(qp, event.HostStopping, h.requestID, queueInfo, step.Description, queue.Stop)
	}

	instanceID := h.instance.InstanceId
	instanceType := h.instance.InstanceType
	h.Printf("successfully detached user volume %s from instance %s", userVolume.VolumeId, instanceID)

	go func() {
		poolFull, err := h.isPoolFull(h.provider, instanceType)
		if err != nil {
			h.Printf("failed to check whether pool for %q is full, err: %v", instanceType, err)

			// We assume the pool is full if we're unable to check its status.
			poolFull = true
		}

		// If instance pool is full, we terminate the instance.
		// Otherwise, we release the (still running) instance to the pool.
		if poolFull {
			h.Printf("pool for %q is full, terminating instance %s after stopping workspace", instanceType, instanceID)
			if _, err := h.provider.TerminateInstances([]string{instanceID}); err != nil {
				h.Printf("failed to terminate instance %s, err: %v", instanceID, err)
			}
			return
		}

		h.Printf("releasing instance %s of stopped workspace to pool for %q", instanceID, instanceType)
		if err := h.releaseInstance(h.provider); err != nil {
			h.Printf("failed to release instance %s to pool, err: %v", instanceID, err)
			return
		}
	}()

	info := &host.Info{
		VolumeID: userVolume.VolumeId,
		Provider: h.DriverName(),
		Region:   h.provider.Region.Name,
		State:    state.Detached,
	}

	host.PublishHost(qp, event.HostStopped, h.requestID, queueInfo, info, queue.Stop, "")

	return info, err
}

func (h *Host) Stop() (*host.Info, error) {
	// WARNING You should probably be changing StopWithQueue() instead.
	label := h.MetricsLabel("stop")
	defer mh.UpdateSince(label, time.Now())

	if err := h.reloadInstance(h.Config.ID); err != nil {
		return nil, mh.Error(err, label)
	}

	if err := h.stopInstance(); err != nil {
		return nil, mh.Error(err, label)
	}

	return h.Info()
}

// recoverFromStop attempts to recover from an error from any of the steps that
// make up a Stop() operation by:
// 1. Re-attaching user volume if it was detached.
// 2. Mounting user device if it was unmounted.
// 3. Starting docker if it was stopped.
// 4. Starting freighterd if it was stopped.
func (h *Host) recoverFromStop() error {
	instanceID := h.Config.ID
	volumeID := h.Config.VolumeID

	vol, err := h.getVolume(volumeID)
	if err != nil {
		h.Printf("Stop() recovery: failed to get volume %s info, err: %v", volumeID, err)
		return err
	}

	if len(vol.Attachments) == 0 {
		h.Printf("Stop() recovery: re-attaching user's volume %s to instance %s", volumeID, instanceID)
		if _, err := attachVolume(h.provider, volumeID, instanceID, userDeviceName); err != nil {
			return fmt.Errorf("amazon: failed to re-attach user volume %s to %s, err: %v", volumeID, instanceID, err)
		}
		h.Printf("Stop() recovery: re-attached user's volume %s to instance %s", volumeID, instanceID)
	}

	hctx := host.NewContext(context.Background(), h, h, nil)

	if err := host.WaitForSSH(hctx); err != nil {
		h.Printf("Stop() recovery: failed to SSH into instance %s, err: %v", instanceID, err)
		return err
	}
	defer hctx.Executor.Close()

	h.Printf("Stop() recovery: mounting %s if necessary", userDeviceOSName)
	mountCmd := fmt.Sprintf(`sudo grep -qs '^%s\s' /proc/mounts || sudo mount %s`, userDeviceOSName, userDeviceOSName)
	if out, err := hctx.Executor.CombinedOutput(mountCmd); err != nil {
		h.Printf("Stop() recovery: failed to mount %s on instance %s, err: %v, output: %s",
			userDeviceOSName, instanceID, err, out)
		return err
	}

	h.Printf("Stop() recovery: starting docker if necessary")
	dCmd := fmt.Sprintf(`sudo service docker status | grep -q 'docker start' || sudo service docker start`)
	if out, err := hctx.Executor.CombinedOutput(dCmd); err != nil {
		h.Printf("Stop() recovery: failed to start docker on instance %s, err: %v, output: %s",
			instanceID, err, out)
		return fmt.Errorf("failed to start docker, err: %v, output: %s", err, out)
	}

	h.Printf("Stop() recovery: starting freighterd if necessary")
	fCmd := fmt.Sprintf(`sudo service freighterd status | grep -q 'freighterd start' || sudo service freighterd start`)
	if out, err := hctx.Executor.CombinedOutput(fCmd); err != nil {
		h.Printf("Stop() recovery: failed to start freighterd on instance %s, err: %v, output: %s",
			instanceID, err, out)
		return fmt.Errorf("failed to start freighterd, err: %v, output: %s", err, out)
	}

	return nil
}
