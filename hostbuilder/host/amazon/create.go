package amazon

import (
	"fmt"
	"time"

	"github.com/mitchellh/goamz/ec2"
	"github.com/nitrous-io/dedicated/event"
	"github.com/nitrous-io/dedicated/hostbuilder/host"
	"github.com/nitrous-io/dedicated/pkg/queue"
	"golang.org/x/net/context"
)

func (h *Host) CreateWithQueue(qp queue.Publisher, queueInfo *host.QueueInfo) (*host.Info, error) {
	var (
		ctx    context.Context
		cancel context.CancelFunc
	)
	ctx, cancel = context.WithTimeout(context.Background(), h.createTimeout())
	defer cancel()

	// Check if we should use the instance pool.
	var useInstancePool bool

	ami, err := getImage(h.provider, h.Config.SourceAMI)
	if err != nil {
		return nil, err
	}
	h.ami = ami

	// If the AMI specifies a block device mapping for a user device, we use the
	// instance pool.
	if h.Config.Hotswap && containsUserDevice(ami.BlockDevices) {
		h.Printf("creating hotswap instance")
		useInstancePool = true
	}

	var steps []host.Step

	var instanceFromPool *ec2.Instance
	if useInstancePool {
		h.Printf("attempting to acquire instance of type %q from pool", h.Config.InstanceType)

		var err error
		instanceFromPool, err = h.acquireInstance(h.provider, h.Config.InstanceType)
		if err != nil {
			h.Printf("could not acquire instance from pool, err: %v", err)

			useInstancePool = false
		}
	}

	if !useInstancePool {
		h.Printf("creating a new instance")

		steps = []host.Step{
			{
				Description: "Creating workspace",
				Func:        createInstance,
				Timeout:     5 * time.Minute,
			},
			/*
				  From @dqminh
					to prevent ec2 from assigning network interfaces to a not running instance
					(i.e., maybe the API reported a false positive ? ), we actually wait for SSH to
					be ready right after finishing creating the instance.

					This waits for SSH to be accessible using the instance's dynamic public IP
					instead of the elasic IP. If the instance is accessible, it should be in
					running state.

					After assigning the elastic IP, ssh check should be really quick, so it's safe
					to double-check it.
			*/
			{
				Func:    host.WaitForSSH,
				Timeout: 5 * time.Minute,
			},
		}
	} else {
		h.Printf("acquired instance %s from pool, using to create host", instanceFromPool.InstanceId)

		h.instance = instanceFromPool

		steps = []host.Step{
			{
				Description: "Initializing disk",
				Func:        attachNewUserVolume,
			},
			{
				Description: "Connecting to workspace",
				Func:        host.WaitForSSH,
				Timeout:     5 * time.Minute,
			},
			{
				Description: "Mounting disk",
				Func:        mountUserVolume,
			},
		}
	}

	// These steps are required regardless of whether instance is from pool or
	// a newly created one.
	commonSteps := []host.Step{
		{
			Description: "Setting workspace's name",
			Func:        associateInstanceTags,
		},
		{
			Description: "Assigning network address",
			Func:        allocateElasticIP,
		},
		{
			Description: "Waiting for the workspace to be ready",
			Func:        host.WaitForSSH,
			Timeout:     5 * time.Minute,
		},
		{
			Description: "Populating settings",
			Func:        host.WriteSettings,
		},
		{
			Description: "Fetching updates",
			Func:        host.UpdateSoftware,
		},
		{
			Description: "Applying settings",
			Func:        host.RestartServices,
		},
	}
	for _, step := range commonSteps {
		steps = append(steps, step)
	}

	hctx := host.NewContext(ctx, h, h, nil)
	defer func() {
		if hctx.Executor != nil {
			hctx.Executor.Close()
		}
	}()

	for _, step := range steps {
		if err := hctx.Run(step); err != nil {
			errorMsg := fmt.Sprintf("ERROR: Failed in create host step %q: %v", step.Description, err)
			h.Printf(errorMsg)
			h.logFailed()
			host.PublishMessage(qp, event.HostCreateFailed, h.requestID, queueInfo, errorMsg, queue.Create)
			inf, _ := h.Info()
			if err == context.DeadlineExceeded {
				return inf, ErrCreateTimeout
			}

			return inf, err
		}

		host.PublishMessage(qp, event.HostCreating, h.requestID, queueInfo, step.Description, queue.Create)
	}

	info, err := h.Info()
	host.PublishHost(qp, event.HostCreated, h.requestID, queueInfo, info, queue.Create, "")

	return info, err
}

func attachNewUserVolume(ctx *host.HostContext) error {
	h := ctx.Host.(*Host)

	// Get snapshot ID of user volume from AMI.
	ami, err := getImage(h.provider, h.Config.SourceAMI)
	if err != nil {
		return err
	}

	var snapshotID string
	for _, dev := range ami.BlockDevices {
		if dev.DeviceName == userDeviceName {
			snapshotID = dev.SnapshotId
			break
		}
	}

	if snapshotID == "" {
		return fmt.Errorf("could not find snapshot of user volume for AMI %s", h.Config.SourceAMI)
	}

	h.Printf("creating new user volume from snapshot %s", snapshotID)

	// Create volume from snapshot, with the requested size.
	vol, err := createVolume(h.provider, snapshotID, h.instance.AvailZone, computeSize(h.Config.StorageSize))
	if err != nil {
		return err
	}
	volID := vol.VolumeId
	h.Printf("new user volume id %s created", volID)

	// Tag volume so it has useful metadata.
	if err := h.tagVolume(volID); err != nil {
		return err
	}

	h.Printf("attaching new user volume %s to instance %s", volID, h.instance.InstanceId)

	// Attach volume to instance.
	vol, err = attachVolume(h.provider, vol.VolumeId, h.instance.InstanceId, userDeviceName)
	if err != nil {
		return fmt.Errorf("amazon: failed to attach new user volume %s to %s, err: %v", volID, h.instance.InstanceId, err)
	}

	h.userVolume = vol

	return nil
}
