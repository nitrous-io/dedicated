package amazon

import (
	"time"

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

	h.Printf("stopping hotswap instance")

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
			Description: "Disassociate Elastic IP",
			Func:        detachElasticIP,
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

	// Terminate the EC2 instance of the host, which will leave the user's
	// volume alone since it was created with `DeleteOnTermination` set to
	// false.
	// Note that we terminate instances instead of returning them to the
	// instance pool to avoid any security issues from users breaking out of
	// their container. The core invariant is: an instance will only ever be
	// used by a single user.
	instanceID := h.instance.InstanceId
	h.Printf("stopping host by terminating instance %s (user volume will be preserved)", instanceID)
	if _, err := h.provider.TerminateInstances([]string{instanceID}); err != nil {
		h.Printf("failed to terminate instance %s, err: %v", instanceID, err)
		return nil, mh.Error(err, label)
	}
	h.Printf("waiting for state to become terminated")
	s := StateChangeConf{
		Pending: []string{"pending", "running", "shutting-down", "stopped", "stopping"},
		Refresh: instanceStateRefreshFunc(h.provider, h.instance),
		Target:  "terminated",
	}
	if _, err := WaitForState(&s); err != nil {
		return nil, mh.Error(err, label)
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
