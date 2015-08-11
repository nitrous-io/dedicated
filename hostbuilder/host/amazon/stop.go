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
	vol, err := h.getVolume(h.Config.VolumeID)
	if err != nil {
		return nil, mh.Error(err, label)
	}

	if h.supportsEBSHotswapping(vol) {
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
			Description: "Release Elastic IP",
			Func: func(ctx *host.HostContext) error {
				h := ctx.Host.(*Host)
				return h.releaseEIP()
			},
		},
	}
	for _, step := range steps {
		if err := hctx.Run(step); err != nil {
			h.Printf("ERROR: failed in step %q: %v", step.Description, err)

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

	poolFull, err := h.isPoolFull(h.provider, instanceType)
	if err != nil {
		h.publishHostError(qp, queueInfo, event.HostStopFailed, queue.Stop, err)
		return nil, mh.Error(err, label)
	}

	// If instance pool is full, we terminate the instance.
	// Otherwise, we release the (still running) instance to the pool.
	if poolFull {
		h.Printf("pool for %q is full, terminating instance", instanceType)
		if _, err := h.provider.TerminateInstances([]string{instanceID}); err != nil {
			err = fmt.Errorf("amazon: failed to terminate instance %s, err %v", instanceID, err)
			h.publishHostError(qp, queueInfo, event.HostStopFailed, queue.Stop, err)
			return nil, mh.Error(err, label)
		}
	} else {
		h.Printf("releasing instance %s of stopped workspace to pool for %q", instanceID, instanceType)
		if err := h.releaseInstance(h.provider); err != nil {
			err = fmt.Errorf("amazon: failed to release instance %s to pool, err %v", instanceID, err)
			h.publishHostError(qp, queueInfo, event.HostStopFailed, queue.Stop, err)
			return nil, mh.Error(err, label)
		}
	}

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
