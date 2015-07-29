package amazon

import (
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Sirupsen/logrus"
	"github.com/mitchellh/goamz/aws"
	"github.com/mitchellh/goamz/ec2"
	"github.com/nitrous-io/dedicated/event"
	"github.com/nitrous-io/dedicated/hostbuilder/host"
	"github.com/nitrous-io/dedicated/hostbuilder/state"
	mh "github.com/nitrous-io/dedicated/pkg/metricshelper"
	"github.com/nitrous-io/dedicated/pkg/queue"
	"golang.org/x/net/context"
)

const (
	FFlagEBSHotswapping = 1 // Feature flag for EBS hotswapping

	// represents 1gb of storage
	storageUnit = 1
	// minimum storage size in GB. This is the size of standard root disk used in AMI
	minStorageSize = 4
	// maximum storage size in GB. This is also the maximum size of a single
	// EBS volume
	// http://docs.aws.amazon.com/AWSEC2/latest/APIReference/API_EbsBlockDevice.html
	maxStorageSize = 1024

	GeneralVolumeType  = "gp2"
	IOPSVolumeType     = "io1"
	MagneticVolumeType = "standard"

	rootDeviceName   = "/dev/sda1"
	userDeviceName   = "/dev/sdu"
	userDeviceOSName = "/dev/xvdu"

	updateTimeout            = 30 * time.Second
	resizeVolumeTimeout      = 30 * time.Minute
	stopTimeout              = 10 * time.Minute
	allocateElasticIPTimeout = 3 * time.Minute
)

var (
	DefaultCreateTimeout = 10 * time.Minute

	ErrCreateTimeout    = errors.New("timed out creating host")
	ErrTimeout          = errors.New("operation timed out")
	ErrInstanceNotFound = errors.New("instance not found")

	KnownRegions = map[string]aws.Region{
		// aws.APNortheast.Name:  aws.APNortheast,
		aws.APSoutheast.Name: aws.APSoutheast,
		// aws.APSoutheast2.Name: aws.APSoutheast2,
		aws.EUWest.Name: aws.EUWest,
		// aws.EUCentral.Name:    aws.EUCentral,
		aws.USEast.Name: aws.USEast,
		// aws.USWest.Name:       aws.USWest,
		aws.USWest2.Name: aws.USWest2,
		// aws.SAEast.Name:       aws.SAEast,
	}
)

// driverConfig stores driver configuration, such as Amazon EC2 access/secret
// keys, timeouts.
type driverConfig struct {
	AccessKey     string `json:"access_key"`
	SecretKey     string `json:"secret_key"`
	HostVersion   string `json:"host_version"` // Created instances will be tagged with "dedicated:version=#{HostVersion}"
	Env           string `json:"env"`          // Created instances will be tagged with "dedicated=#{Env}".
	CreateTimeout string `json:"create_timeout"`
	NotifyURL     string `json:"notify_url"`
	Regions       []struct {
		ID      string `json:"id"`
		Keypair struct {
			Name           string `json:"name"`
			PrivateKeyPath string `json:"private_key_path"`
		} `json:"keypair"`
		InstancePool []struct {
			Type string `json:"type"`
			Max  int    `json:"max"`
		} `json:"instance_pool"`
	} `json:"regions"`

	// TODO Super hacky - refactor this when we refactor hostbuilder drivers.
	IgnoreRegion string `json:"ignore_region"`

	// cachedCreateTimeout is used to cache a parsed CreateTimeout.
	cachedCreateTimeout time.Duration
}

func (c *driverConfig) createTimeout() time.Duration {
	if c.cachedCreateTimeout != 0 {
		return c.cachedCreateTimeout
	}

	t := DefaultCreateTimeout
	d, err := time.ParseDuration(c.CreateTimeout)
	if err == nil {
		t = d
	}
	c.cachedCreateTimeout = t

	return t
}

type Host struct {
	driverConfig

	Config *hostConfig

	requestID string

	// ec2 properties
	auth     aws.Auth // this is used when we want to query all known regions
	provider *ec2.EC2
	instance *ec2.Instance

	// Caches keypair so that we don't have to parse config and read the private
	// key file multiple times.
	keypair *keypair

	// For respec.
	userVolume *ec2.Volume
	newVolume  *ec2.Volume
	snapshot   *ec2.Snapshot
	newSize    int64 // New storage size in GB.
}

type hostConfig struct {
	ID              string `json:"id"`
	HostID          int64  `json:"host_id"` // database id of the host
	Name            string `json:"name"`
	SourceAMI       string `json:"source_ami"`
	InstanceType    string `json:"instance_type"`
	RegionID        string `json:"region_id"`
	SecurityGroupID string `json:"security_group_id"`
	IPAddress       string `json:"ip_address"`

	// DisableEIP disables the allocation of Elastic IP for the current host
	// When this happens, the returned IP will be the host's public IP.
	DisableEIP bool `json:"disable_eip"`

	// VPC specific settings
	SubnetID string `json:"subnet_id"`

	// StorageSize is used to calculate total storage of the root device.
	// TotalStorage = StorageSize * storageUnit
	StorageSize int `json:"storage_size"`

	// freighter specific config
	PublicKeys        []string        `json:"public_keys"`
	TLS               *host.TLSConfig `json:"tls"`
	Secret            string          `json:"secret"`
	AptUsername       string          `json:"apt_username"` // host id on nitrous.io
	AptPassword       string          `json:"apt_password"` // host secret
	AptLocation       string          `json:"apt_location"` // URI of the repository
	AptDist           string          `json:"apt_dist"`     // distribution of the repo
	FreighterdVersion string          `json:"freighterd_version"`
}

func (c *hostConfig) securityGroups() []ec2.SecurityGroup {
	return []ec2.SecurityGroup{
		{Id: c.SecurityGroupID},
	}
}

func (h *Host) CreateWithQueue(qp queue.Publisher, queueInfo *host.QueueInfo) (*host.Info, error) {
	var (
		ctx    context.Context
		cancel context.CancelFunc
	)
	ctx, cancel = context.WithTimeout(context.Background(), h.createTimeout())
	defer cancel()

	hctx := host.NewContext(ctx, h, h, nil)
	defer func() {
		if hctx.Executor != nil {
			hctx.Executor.Close()
		}
	}()

	steps := []host.Step{
		{
			Description: "Checking image's availability",
			Func:        inspectSourceAMI,
		},
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

// Update sets properties of an existing host
func (h *Host) Update(w io.Writer) (*host.Info, error) {
	label := h.MetricsLabel("update")
	defer mh.UpdateSince(label, time.Now())

	if err := h.reloadInstance(h.Config.ID); err != nil {
		return nil, mh.Error(err, label)
	}

	ctx, cancel := context.WithTimeout(context.Background(), updateTimeout)
	defer cancel()

	hctx := host.NewContext(ctx, h, h, w)
	steps := []host.Step{
		{
			Description: "Setting workspace's name",
			Func:        associateInstanceTags,
		},
	}
	for _, step := range steps {
		if err := hctx.Run(step); err != nil {
			h.Printf("ERROR: Failed in update host step: %v", err)
			h.logFailed()
			if err == context.DeadlineExceeded {
				return nil, ErrTimeout
			}
			return nil, err
		}
	}

	return h.Info()
}

// Respec changes the specs of an existing host its new properties.
// The following changes are supported:
//   1. Changing storage size of user volume.
//   2. Changing instance type (only between allowable instance types, e.g. we
//      don't support changing from a VPC-only to non-VPC instance).
//   3. Allocating/de-allocating an Elastic IP.
func (h *Host) Respec() (*host.Info, error) {
	label := h.MetricsLabel("respec")
	defer mh.UpdateSince(label, time.Now())

	if err := h.reloadInstance(h.Config.ID); err != nil {
		return nil, mh.Error(err, label)
	}

	// Allocate Elastic IP.
	addresses, err := getAddresses(h.provider, h.instance.InstanceId)
	if err != nil {
		return nil, mh.Error(err, label)
	}
	// If instance already has an Elastic IP allocated, we skip this step.
	if len(addresses) == 0 {
		ctx, cancel := context.WithTimeout(context.Background(), allocateElasticIPTimeout)
		defer cancel()

		hctx := host.NewContext(ctx, h, h, nil)
		err := hctx.Run(host.Step{
			Description: "Allocating static IP",
			Func:        allocateElasticIP,
		})
		if err != nil {
			h.Printf("ERROR: failed to allocate Elastic IP to instance %s: %v", h.instance.InstanceId, err)
			return nil, mh.Error(err, label)
		}
	}

	var changeInstanceType, changeStorageSize bool

	if h.Config.InstanceType != "" && h.Config.InstanceType != h.instance.InstanceType {
		changeInstanceType = true
	}

	var (
		userVolume     *ec2.Volume
		configuredSize int64
	)

	// If StorageSize is 0, no change required.
	if h.Config.StorageSize != 0 {
		var err error
		userVolume, err = h.getUserVolume()
		if err != nil {
			return nil, mh.Error(err, label)
		}

		currentSize, err := strconv.ParseInt(userVolume.Size, 10, 64)
		if err != nil {
			return nil, mh.Error(fmt.Errorf("amazon: cannot parse size from volume: %+v, err: %v", userVolume, err), label)
		}

		configuredSize = computeSize(h.Config.StorageSize)
		if currentSize < configuredSize {
			changeStorageSize = true
			h.Printf("respec: volume resize required (%d to %d)", currentSize, configuredSize)
		}
	}

	// If no further changes required, we're done.
	if !changeInstanceType && !changeStorageSize {
		return h.Info()
	}

	// Otherwise, we stop the instance since any further changes now require
	// it to be stopped.
	if err := h.stopInstance(); err != nil {
		return nil, mh.Error(fmt.Errorf("amazon: failed to stop instance for respeccing, err: %v", err), label)
	}

	if changeInstanceType {
		h.Printf("Changing instance type of instance %s from %s to %s", h.instance.InstanceId, h.instance.InstanceType, h.Config.InstanceType)
		if err := modifyInstanceType(h.provider, h, h.Config.InstanceType); err != nil {
			return nil, mh.Error(fmt.Errorf("amazon: failed to change instance type, err: %v"), label)
		}
	}

	if changeStorageSize {
		if err := h.resizeUserVolume(userVolume, configuredSize); err != nil {
			return nil, mh.Error(fmt.Errorf("amazon: failed to resize user volume, err: %v", err), label)
		}
	}

	return h.Start()
}

func (h *Host) getUserVolume() (*ec2.Volume, error) {
	userDevice := userDevice(h.instance)
	if userDevice == nil {
		return nil, fmt.Errorf("amazon: cannot find user device for instance %s", h.instance.InstanceId)
	}

	resp, err := h.provider.Volumes([]string{userDevice.VolumeId}, ec2.NewFilter())
	if err != nil {
		return nil, fmt.Errorf("amazon: failed to get user volume info, err: %v", err)
	}
	if len(resp.Volumes) != 1 {
		return nil, fmt.Errorf("amazon: expected to get 1 volume but got %+v", resp.Volumes)
	}
	userVolume := &resp.Volumes[0]
	if userVolume.VolumeId != userDevice.VolumeId {
		// Sanity check - because using the wrong volume is bad m'kay.
		return nil, fmt.Errorf("amazon: expected volume %s but got unexpected volume: %+v", userDevice.VolumeId, userVolume)
	}

	return userVolume, nil
}

type resizeStep struct {
	host.Step
	restartOnError bool
}

func (h *Host) resizeUserVolume(userVolume *ec2.Volume, newSize int64) (err error) {
	var (
		ctx    context.Context
		cancel context.CancelFunc
	)
	ctx, cancel = context.WithTimeout(context.Background(), resizeVolumeTimeout)
	defer cancel()

	h.userVolume = userVolume
	h.newSize = newSize

	hctx := host.NewContext(ctx, h, h, nil)
	steps := []resizeStep{
		{
			Step: host.Step{
				Description: "Taking snapshot of disk",
				Func:        snapshotUserVolume,
			},
			restartOnError: true,
		},
		{
			Step: host.Step{
				Description: "Creating larger disk",
				Func:        createNewVolume,
			},
			restartOnError: true,
		},
		{
			Step: host.Step{
				Description: "Switching to larger disk",
				Func:        swapUserVolume,
			},
		},
		{
			Step: host.Step{
				Description: "Cleaning up",
				Func:        deleteOldVolume,
			},
		},
	}
	for _, step := range steps {
		if err := hctx.Run(step.Step); err != nil {
			h.Printf("ERROR: failed in resize user volume step %q: %v", step.Description, err)

			// Start the instance back up if necessary.
			// Note that the time to start the instance is excluded from the
			// context timeout.
			if step.restartOnError {
				if _, e := h.Start(); e != nil {
					h.Printf("failed to start instance %s back up", h.instance.InstanceId)
				}
			}

			if err == context.DeadlineExceeded {
				return ErrTimeout
			}
			return err
		}
	}

	return nil
}

func snapshotUserVolume(ctx *host.HostContext) error {
	h := ctx.Host.(*Host)
	label := h.MetricsLabel("snapshot_user_volume")
	defer mh.UpdateSince(label, time.Now())

	userVolume := h.userVolume
	newSize := h.newSize

	h.Printf("Creating snapshot of user volume %s of instance %s", userVolume.VolumeId, h.instance.InstanceId)

	desc := fmt.Sprintf("Created by hostbuilder for respec of %s from %sGB to %dGB",
		h.instance.InstanceId, userVolume.Size, newSize)
	snapshot, err := createSnapshot(h.provider, userVolume.VolumeId, desc)
	if err != nil {
		return fmt.Errorf("amazon: failed to create snapshot of user volume %+v, err: %v", userVolume, err)
	}

	h.snapshot = snapshot

	tags := []ec2.Tag{
		{Key: "InstanceID", Value: h.instance.InstanceId},
		{Key: "dedicated", Value: h.driverConfig.Env},
	}
	if _, err := h.provider.CreateTags([]string{snapshot.Id}, tags); err != nil {
		h.Printf("ERROR: failed to tag snapshot %v, err: %v", snapshot.Id, err)
	}

	h.Printf("Created snapshot %s of volume %s", snapshot.Id, userVolume.VolumeId)

	return nil
}

func createNewVolume(ctx *host.HostContext) error {
	h := ctx.Host.(*Host)
	label := h.MetricsLabel("create_new_volume")
	defer mh.UpdateSince(label, time.Now())

	userVolume := h.userVolume
	snapshot := h.snapshot
	newSize := h.newSize

	h.Printf("Creating volume from snapshot %s", snapshot.Id)
	newVolume, err := createVolume(h.provider, snapshot.Id, userVolume.AvailZone, newSize)
	if err != nil {
		return fmt.Errorf("amazon: failed to create new volume from snapshot %s, err: %v", snapshot.Id, err)
	}
	h.Printf("Created new volume %s from snapshot %s", newVolume.VolumeId, snapshot.Id)

	h.newVolume = newVolume

	return nil
}

func swapUserVolume(ctx *host.HostContext) error {
	h := ctx.Host.(*Host)
	label := h.MetricsLabel("swap_user_volume")
	defer mh.UpdateSince(label, time.Now())

	userVolume := h.userVolume
	newVolume := h.newVolume

	h.Printf("Detaching user volume %s of instance %s", userVolume.VolumeId, h.instance.InstanceId)
	if vol, err := detachVolume(h.provider, userVolume.VolumeId, h.instance.InstanceId); err != nil {
		return fmt.Errorf("amazon: failed to detach user volume, err: %v", err)
	} else {
		userVolume = vol
	}
	h.Printf("Detached user volume %s from %s", userVolume.VolumeId, h.instance.InstanceId)

	h.Printf("Attaching new volume %s to instance %s", newVolume.VolumeId, h.instance.InstanceId)
	if vol, err := attachVolume(h.provider, newVolume.VolumeId, h.instance.InstanceId, userDeviceName); err != nil {
		return fmt.Errorf("amazon: failed to attach new volume %+v to %s, err: %v", newVolume, h.instance.InstanceId, err)
	} else {
		newVolume = vol
	}
	h.Printf("Attached new volume %s to instance %s", newVolume.VolumeId, h.instance.InstanceId)

	return nil
}

func deleteOldVolume(ctx *host.HostContext) error {
	h := ctx.Host.(*Host)
	label := h.MetricsLabel("delete_old_volume")
	defer mh.UpdateSince(label, time.Now())

	userVolume := h.userVolume

	h.Printf("Deleting old user volume %s of instance %s", userVolume.VolumeId, h.instance.InstanceId)
	if _, err := h.provider.DeleteVolume(userVolume.VolumeId); err != nil {
		h.Printf("ERROR: failed to delete old user volume %s for instance %s after respec, err: %v", userVolume.VolumeId, h.instance.InstanceId, err)
	}
	h.Printf("Deleted old user volume %s of instance %s", userVolume.VolumeId, h.instance.InstanceId)

	return nil
}

// Destroy releases the instance Elastic IP and terminate it
func (h *Host) Destroy() error {
	label := h.MetricsLabel("destroy")
	defer mh.UpdateSince(label, time.Now())

	if err := h.reloadInstance(h.Config.ID); err != nil {
		// if the instance does not exist, then consider that it is sucessfully destroyed
		if err == ErrInstanceNotFound {
			return nil
		}
		return mh.Error(err, label)
	}

	if err := h.releaseEIP(); err != nil {
		return mh.Error(err, label)
	}

	// terminate the instance
	if _, err := h.provider.TerminateInstances([]string{h.instance.InstanceId}); err != nil {
		err = fmt.Errorf("amazon: failed to terminate instance %s, err %v", h.instance.InstanceId, err)
		return mh.Error(err, label)
	}

	state := StateChangeConf{
		Pending: []string{"pending", "running", "shutting-down", "stopped", "stopping"},
		Refresh: instanceStateRefreshFunc(h.provider, h.instance),
		Target:  "terminated",
	}
	h.Printf("waiting for state to become %s", state.Target)
	updatedInstance, err := WaitForState(&state)
	if err != nil {
		return mh.Error(err, label)
	}
	h.instance = updatedInstance.(*ec2.Instance)

	return nil
}

func (h *Host) Info() (*host.Info, error) {
	if h.instance == nil {
		return nil, errors.New("Invalid ec2 instance")
	}

	info := &host.Info{
		Provider:       h.DriverName(),
		ID:             h.instance.InstanceId,
		IPAddress:      h.instance.PublicIpAddress,
		FreighterdPort: host.DefaultPort,
		Region:         h.provider.Region.Name,
		State:          getState(h.instance),
	}

	userVolume, err := h.getUserVolume()
	if err == nil {
		info.VolumeID = userVolume.VolumeId
	}

	return info, nil
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

// unmountUserVolume logs in to instance via SSH to:
// 1. Stop freighter.
// 2. Stop docker.
// 3. Unmount user volume.
// Then, we detach the user EBS volume from its instance.
func unmountUserVolume(ctx *host.HostContext) error {
	h := ctx.Host.(*Host)
	conn := ctx.Executor

	label := h.MetricsLabel("unmount_user_volume")
	defer mh.UpdateSince(label, time.Now())

	if out, err := conn.CombinedOutput(`sudo service freighterd stop`); err != nil {
		return fmt.Errorf("failed to stop freighterd, err: %v, output: %s", err, out)
	}

	if out, err := conn.CombinedOutput(`sudo service docker stop`); err != nil {
		return fmt.Errorf("failed to stop docker, err: %v, output: %s", err, out)
	}

	if out, err := conn.CombinedOutput(fmt.Sprintf(`sudo umount %s`, userDeviceOSName)); err != nil {
		return fmt.Errorf("failed to unmount %s, err: %v, output: %s", userDeviceOSName, err, out)
	}

	if _, err := detachVolume(h.provider, h.userVolume.VolumeId, h.instance.InstanceId); err != nil {
		return err
	}

	return nil
}

func (h *Host) stopInstance() error {
	if _, err := h.provider.StopInstances(h.instance.InstanceId); err != nil {
		return err
	}

	state := StateChangeConf{
		Pending: []string{"pending", "running", "stopping"},
		Refresh: instanceStateRefreshFunc(h.provider, h.instance),
		Target:  "stopped",
	}
	h.Printf("waiting for state to become %s", state.Target)
	updatedInstance, err := WaitForState(&state)
	if err != nil {
		return err
	}
	h.instance = updatedInstance.(*ec2.Instance)

	return nil
}

func (h *Host) StopWithQueue(qp queue.Publisher, queueInfo *host.QueueInfo) (*host.Info, error) {
	label := h.MetricsLabel("stop")
	defer mh.UpdateSince(label, time.Now())

	host.PublishMessage(qp, event.HostStopping, h.requestID, queueInfo, "Stopping host", queue.Stop)

	if err := h.reloadInstance(h.Config.ID); err != nil {
		if info, e := h.Info(); e == nil {
			host.PublishHost(qp, event.HostStopFailed, h.requestID, queueInfo, info, queue.Stop, err.Error())
		} else {
			logrus.Debugf("[ERROR] uuid=%s amazon: failed to send a message due to failure on getting host info: %v", h.requestID, e)
		}
		return nil, mh.Error(err, label)
	}

	// TODO Feature flag required: for old hosts, we stop them the old way.

	var (
		ctx    context.Context
		cancel context.CancelFunc
	)
	ctx, cancel = context.WithTimeout(context.Background(), stopTimeout)
	defer cancel()

	userVolume, err := h.getUserVolume()
	if err != nil {
		return nil, mh.Error(err, label)
	}
	h.userVolume = userVolume

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
			Timeout:     5 * time.Minute,
		},
		{
			Description: "Detaching user disk",
			Func:        unmountUserVolume,
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

			if info, e := h.Info(); e == nil {
				host.PublishHost(qp, event.HostStopFailed, h.requestID, queueInfo, info, queue.Stop, err.Error())
			} else {
				logrus.Debugf("[ERROR] uuid=%s amazon: failed to send a message due to failure on getting host info: %v", h.requestID, e)
			}

			if err == context.DeadlineExceeded {
				return nil, ErrTimeout
			}
			return nil, mh.Error(err, label)
		}
	}

	instanceID := h.instance.InstanceId
	instanceType := h.instance.InstanceType
	logrus.Infof("amazon: successfully detached user volume %s from instance %s", userVolume.VolumeId, instanceID)

	poolFull, err := h.isPoolFull(instanceType)
	if err != nil {
		if info, e := h.Info(); e == nil {
			host.PublishHost(qp, event.HostStopFailed, h.requestID, queueInfo, info, queue.Stop, err.Error())
		} else {
			logrus.Debugf("[ERROR] uuid=%s amazon: failed to send a message due to failure on getting host info: %v", h.requestID, e)
		}
		return nil, mh.Error(err, label)
	}

	// If instance pool is full, we terminate the instance.
	// Otherwise, we release the (still running) instance to the pool.
	if poolFull {
		logrus.Infof("amazon: pool for %s is full, terminating instance", instanceType)
		if _, err := h.provider.TerminateInstances([]string{instanceID}); err != nil {
			err = fmt.Errorf("amazon: failed to terminate instance %s, err %v", instanceID, err)
			if info, e := h.Info(); e == nil {
				host.PublishHost(qp, event.HostStopFailed, h.requestID, queueInfo, info, queue.Stop, err.Error())
			} else {
				logrus.Debugf("[ERROR] uuid=%s amazon: failed to send a message due to failure on getting host info: %v", h.requestID, e)
			}
			return nil, mh.Error(err, label)
		}
	} else {
		logrus.Infof("amazon: releasing instance of stopped workspace to pool for %s", instanceType)
		if err := h.releaseInstance(); err != nil {
			err = fmt.Errorf("amazon: failed to release instance %s to pool, err %v", instanceID, err)
			if info, e := h.Info(); e == nil {
				host.PublishHost(qp, event.HostStopFailed, h.requestID, queueInfo, info, queue.Stop, err.Error())
			} else {
				logrus.Debugf("[ERROR] uuid=%s amazon: failed to send a message due to failure on getting host info: %v", h.requestID, e)
			}
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

// Start starts the instance and allocate the desired IP address to it
func (h *Host) Start() (*host.Info, error) {
	label := h.MetricsLabel("start")
	defer mh.UpdateSince(label, time.Now())

	if err := h.reloadInstance(h.Config.ID); err != nil {
		return nil, mh.Error(err, label)
	}

	if _, err := h.provider.StartInstances(h.instance.InstanceId); err != nil {
		return nil, mh.Error(err, label)
	}

	state := StateChangeConf{
		Pending: []string{"pending", "stopped"},
		Refresh: instanceStateRefreshFunc(h.provider, h.instance),
		Target:  "running",
	}
	h.Printf("waiting for state to become %s", state.Target)
	updatedInstance, err := WaitForState(&state)
	if err != nil {
		return nil, mh.Error(err, label)
	}
	h.instance = updatedInstance.(*ec2.Instance)

	// if this is a VPC instance, we dont have to reassociate ElasticIP because
	// it will be persisted after stop by default.
	// http://docs.aws.amazon.com/AWSEC2/latest/UserGuide/elastic-ip-addresses-eip.html#VPC_EIP_EC2_Differences
	if !isVPC(h.instance) && !h.Config.DisableEIP {
		if _, err := h.provider.AssociateAddress(&ec2.AssociateAddress{
			InstanceId:         h.instance.InstanceId,
			PublicIp:           h.Config.IPAddress,
			AllowReassociation: false,
		}); err != nil {
			return nil, mh.Error(err, label)
		}

		// reload the instance to get a new public IP address
		if err := h.reloadInstance(h.instance.InstanceId); err != nil {
			return nil, mh.Error(err, label)
		}
	}

	return h.Info()
}

// Restart performs a hard-reset of the instance. This should not change the
// public IP address of the instance. This does not use RebootAPI, but instead
// stop/starting the instance to make sure that it has a clean state.
func (h *Host) Restart() (*host.Info, error) {
	label := h.MetricsLabel("restart")
	defer mh.UpdateSince(label, time.Now())
	if _, err := h.Stop(); err != nil {
		return nil, err
	}
	return h.Start()
}

// List returns all instances in the current region. To perform list of
// multiple regions, use one List() request for each region.
func (h *Host) List() ([]*host.Info, error) {
	label := h.MetricsLabel("list")
	defer mh.UpdateSince(label, time.Now())

	var (
		infos   = []*host.Info{}
		errmsgs []string
		mutex   = &sync.Mutex{} // protects infos and errmsgs
		wg      sync.WaitGroup
	)

	for _, region := range KnownRegions {
		wg.Add(1)
		go func(region aws.Region) {
			defer wg.Done()

			// per-region librato metrics
			// h.MetricsLabel cannot be used since it assumes region is populated
			// and List() call is region-agnostic
			label := fmt.Sprintf("%s.amazon.%s", "list", region.Name)
			defer mh.UpdateSince(label, time.Now())

			regionInfos := []*host.Info{}
			conn := ec2.New(h.auth, region)
			params := make(map[string]string)
			params["MaxResults"] = "200"

			firstTime := true
			nextToken := ""

			for firstTime || nextToken != "" {
				if nextToken != "" {
					params["NextToken"] = nextToken
				}

				instancesResp, err := conn.InstancesWithExtraParams(nil, nil, params)

				if err != nil {
					errmsg := fmt.Sprintf("failed to get instances in region %s, err %+v", region.Name, err)
					h.Printf(errmsg)
					mutex.Lock()
					errmsgs = append(errmsgs, errmsg)
					mutex.Unlock()
					break
				}

				nextToken = instancesResp.NextToken
				firstTime = false

				for _, reservation := range instancesResp.Reservations {
					for _, instance := range reservation.Instances {
						checkInstance := false
						// Tags is array of Tag. There is no way to check tag by key
						for _, tag := range instance.Tags {
							if tag.Key == "dedicated" && tag.Value == h.driverConfig.Env {
								checkInstance = true
								break
							}
						}

						if !checkInstance {
							continue
						}

						info := &host.Info{
							Provider:       h.DriverName(),
							ID:             instance.InstanceId,
							IPAddress:      instance.PublicIpAddress,
							FreighterdPort: host.DefaultPort,
							State:          getState(&instance),
							Region:         region.Name,
							LaunchTime:     &instance.LaunchTime,
						}

						// Get EC2 instance name
						for _, tag := range instance.Tags {
							if tag.Key == "Name" {
								info.Name = tag.Value
								break
							}
						}

						regionInfos = append(regionInfos, info)
					}
				}
			}
			mutex.Lock()
			infos = append(infos, regionInfos...)
			mutex.Unlock()
		}(region)
	}
	wg.Wait()

	var err error
	if len(errmsgs) > 0 {
		err = fmt.Errorf("Errors getting instances: %s", strings.Join(errmsgs, "; "))
	}
	return infos, err
}

func (h *Host) DriverName() string {
	return "amazon"
}

// Printf logs a formatted string together with the request uuid and instance
// id, if available.
func (h *Host) Printf(format string, v ...interface{}) {
	fields := logrus.Fields{}
	if h.requestID != "" {
		fields["uuid"] = h.requestID
	}
	if h.instance != nil {
		fields["instance"] = h.instance.InstanceId
	}
	logrus.WithFields(fields).Infof(fmt.Sprintf("amazon: %s", format), v...)
}

// MetricsLabel returns the label with the current host's region if possible
func (h *Host) MetricsLabel(prefix string) string {
	// region should be set for every requests because ec2 API requires it to
	// find the instance id properly
	if h.Config.RegionID != "" {
		return fmt.Sprintf("%s.amazon.%s", prefix, h.Config.RegionID)
	}
	return fmt.Sprintf("%s.amazon", prefix)
}

// Settings returns the dynamic settings of the host
func (h *Host) Settings() *host.Settings {
	return host.NewSettings(
		h.Config.TLS,
		&host.AptConfig{
			AptUsername: h.Config.AptUsername,
			AptPassword: h.Config.AptPassword,
			AptLocation: h.Config.AptLocation,
			AptDist:     h.Config.AptDist,
		},
		"/home/ubuntu",
		h.generateNotifyURL(),
		h.Config.Secret,
		h.Config.FreighterdVersion,
		h.Config.PublicKeys,
	)
}

// Name returns the name of the host, injecting env if available
func (h *Host) Name() string {
	env := h.driverConfig.Env
	if env == "" {
		env = "env"
	}
	return strings.Replace(h.Config.Name, "{{env}}", env, -1)
}

// SSHConfig returns the host's SSH confiration.
func (h *Host) SSHConfig() *host.SSHConfig {
	kp, err := h.regionKeypair()
	if err != nil {
		h.Printf("error getting region keypair, err: %v", err)
	}

	return &host.SSHConfig{
		IPAddress:  h.instance.PublicIpAddress,
		Port:       host.SSHPort,
		Username:   "ubuntu",
		PrivateKey: kp.privateKey,
	}
}

func (h *Host) SetRequestID(id string) {
	h.requestID = id
}

// reloadInstance reloads the current instance associated with this Host to the
// instance specified by `id` param. It can be used to refresh the current
// instance, or to load a new one.
func (h *Host) reloadInstance(id string) error {
	return getInstance(h.provider, id, h)
}

func (h *Host) releaseEIP() error {
	if h.Config.DisableEIP {
		return nil
	}

	if ip := h.Config.IPAddress; ip != "" {
		// Release IP address given by the client. We can't use
		// h.instance.PublicIPAddress because it's not guaranteed to be the
		// correct IP when the instance is stopped.
		logrus.Infof("amazon: releasing IP address %s for instance %s", ip, h.instance.InstanceId)
		if err := releaseAddress(h.provider, ip, isVPC(h.instance)); err != nil {
			return fmt.Errorf("amazon: failed to release ElasticIP %s for %s, err %v",
				h.Config.IPAddress, h.instance.InstanceId, err)
		}
	}

	return nil
}

func (h *Host) generateNotifyURL() string {
	return strings.Replace(h.driverConfig.NotifyURL, "{{slug}}", strconv.FormatInt(h.Config.HostID, 10), -1)
}

func (h *Host) regionKeypair() (*keypair, error) {
	if h.keypair != nil {
		return h.keypair, nil
	}

	for _, r := range h.driverConfig.Regions {
		if r.ID != h.Config.RegionID {
			continue
		}

		path := r.Keypair.PrivateKeyPath
		data, err := ioutil.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("error reading private key at %q, err: %v", path, err)
		}
		block, _ := pem.Decode(data)
		pk, err := x509.ParsePKCS1PrivateKey(block.Bytes)
		if err != nil {
			return nil, fmt.Errorf("failed to parse a valid RSA private key from %q, err: %v", path, err)
		}

		h.keypair = &keypair{name: r.Keypair.Name, privateKey: pk}

		return h.keypair, nil
	}

	return nil, fmt.Errorf("private key not configured for region %q", h.Config.RegionID)
}

// progressLog writes the message to the given writer and logs the message,
// prefixing host information.  It is used for user-facing messages.  Use
// `h.Printf` for internal logging.
func (h *Host) progressLog(w io.Writer, format string, v ...interface{}) {
	fmt.Fprintf(w, format, v...)
	h.Printf(format, v...)
}

func (h *Host) logFailed() {
	if h.instance != nil {
		h.Printf("Leaving instance for inspection: %s", h.instance.InstanceId)
	}
}

func (h *Host) supportsEBSHotswapping() bool {
	for _, tag := range h.instance.Tags {
		if tag.Key == "dedicated:version" {
			version, _ := strconv.ParseInt(tag.Value, 10, 64) // On any error, consider host as version 0.
			return version >= FFlagEBSHotswapping
		}
	}

	return false
}

func New(driverOptions string, hostOptions string) (host.Host, error) {
	driverConfig := driverConfig{}
	if err := json.Unmarshal([]byte(driverOptions), &driverConfig); err != nil {
		logrus.Errorf("amazon: invalid driver config: %s", driverOptions)
		return nil, err
	}

	hostConfig := &hostConfig{}
	if err := json.Unmarshal([]byte(hostOptions), hostConfig); err != nil {
		logrus.Errorf("amazon: invalid host config: %s", hostOptions)
		return nil, err
	}

	auth, err := aws.GetAuth(driverConfig.AccessKey, driverConfig.SecretKey)
	if err != nil {
		logrus.Errorf("amazon: auth failed %v", err)
		return nil, err
	}

	host := &Host{
		driverConfig: driverConfig,
		Config:       hostConfig,
		auth:         auth,
	}

	// workaround for actions that requires multiple regions i.e. Host#List
	// This is set by the caller and should not be serialized as a
	// configuration value
	if driverConfig.IgnoreRegion != "true" {
		region, ok := aws.Regions[hostConfig.RegionID]
		if !ok {
			return nil, fmt.Errorf("amazon: invalid region %q", hostConfig.RegionID)
		}
		host.provider = ec2.New(auth, region)
	}
	return host, nil
}

func init() {
	host.Register("amazon", &host.RegisteredDriver{
		New: New,
	})
}

// inspectSourceAMI checks that the requested AMI exists and can be used with
// the current settings.
func inspectSourceAMI(ctx *host.HostContext) error {
	h := ctx.Host.(*Host)
	label := h.MetricsLabel("inspect_source_ami")
	defer mh.UpdateSince(label, time.Now())

	h.Printf("inspecting source ami %q", h.Config.SourceAMI)
	if _, err := getImage(h.provider, h.Config.SourceAMI); err != nil {
		return mh.Error(err, label)
	}
	h.Printf("inspecting source ami %q... Done", h.Config.SourceAMI)
	return nil
}

// createInstance launches a new instance with host's Config and waits for it to
// enter the "running" state.
func createInstance(ctx *host.HostContext) error {
	h := ctx.Host.(*Host)
	label := h.MetricsLabel("create_host")
	defer mh.UpdateSince(label, time.Now())

	storageSize := computeSize(h.Config.StorageSize)

	h.Printf("amazon: creating instance in region %s, type: %s, size: %d, name: %s",
		h.Config.RegionID, h.Config.InstanceType, storageSize, h.Name())

	kp, err := h.regionKeypair()
	if err != nil {
		return mh.Error(fmt.Errorf("amazon: failed to get region keypair %v", err), label)
	}

	opts := &ec2.RunInstances{
		KeyName:        kp.name,
		ImageId:        h.Config.SourceAMI,
		InstanceType:   h.Config.InstanceType,
		SecurityGroups: h.Config.securityGroups(),
		MinCount:       0,
		MaxCount:       0,
		BlockDevices: []ec2.BlockDeviceMapping{
			{
				// The root volume is defined in the AMI's block devices as a
				// snapshot, so we do not need to specify its size.
				DeviceName:          rootDeviceName,
				VolumeType:          GeneralVolumeType,
				DeleteOnTermination: true,
			},
			{
				// The user volume is also defined in the AMI's block devices
				// and we override its size to what was requested.
				DeviceName:          userDeviceName,
				VolumeSize:          storageSize,
				VolumeType:          GeneralVolumeType,
				DeleteOnTermination: false,
			},
		},
		AssociatePublicIpAddress: true,
	}
	// assigned subnet ID for VPC instance
	if h.Config.SubnetID != "" {
		opts.SubnetId = h.Config.SubnetID
	}
	resp, err := h.provider.RunInstances(opts)
	if err != nil || len(resp.Instances) == 0 {
		return mh.Error(fmt.Errorf("amazon: failed to create new instance %v", err), label)
	}
	h.instance = &resp.Instances[0]

	// wait for the instance to be ready
	state := &StateChangeConf{
		Pending: []string{"pending"},
		Target:  "running",
		Refresh: instanceStateRefreshFunc(h.provider, h.instance),
	}
	h.Printf("waiting for state to become %s", state.Target)
	updatedInstance, err := WaitForState(state)
	if err != nil {
		return mh.Error(fmt.Errorf("amazon: failed to change state %v", err), label)
	}
	h.instance = updatedInstance.(*ec2.Instance)

	if h.instance.PublicIpAddress == "" {
		return fmt.Errorf("failed to allocate public IP for instance %s", h.instance.InstanceId)
	}

	return nil
}

// allocateElasticIP creates a new ElasticIP and associate it with the
// instance. This is done so the public IP of the instance will not be changed
// when it's stopped/restarted
func allocateElasticIP(ctx *host.HostContext) error {
	h := ctx.Host.(*Host)
	// if eip is disabled on this host, we dont want to allocate it
	if h.Config.DisableEIP {
		return nil
	}
	label := h.MetricsLabel("allocate_ip")
	defer mh.UpdateSince(label, time.Now())

	_, err := assignAddress(h.provider, h.instance.InstanceId, isVPC(h.instance))
	if err != nil {
		return mh.Error(err, label)
	}

	// reload the instance to get a new public IP address
	if err := h.reloadInstance(h.instance.InstanceId); err != nil {
		return mh.Error(err, label)
	}

	return nil
}

// associateInstanceTags sets the instance's tags which include Name and
// dedicated environment.
// If this fails, we still consider the operation to be successful
func associateInstanceTags(ctx *host.HostContext) error {
	h := ctx.Host.(*Host)
	label := h.MetricsLabel("set_tags")
	defer mh.UpdateSince(label, time.Now())

	setTags(h.provider, h)
	return nil
}

func getState(instance *ec2.Instance) state.State {
	switch instance.State.Name {
	case "pending":
		return state.Starting
	case "running":
		return state.Running
	case "stopping":
		return state.Stopping
	case "stopped":
		return state.Stopped
	case "terminated":
		return state.Terminated
	}
	return state.Unknown
}

// isVPC checks if an instance is a EC2-VPC instance
func isVPC(i *ec2.Instance) bool {
	return i != nil && i.SubnetId != ""
}

func computeSize(size int) int64 {
	storageSize := int64(size * storageUnit)
	if storageSize > maxStorageSize {
		storageSize = maxStorageSize
	} else if storageSize < minStorageSize {
		storageSize = minStorageSize
	}
	return storageSize
}

func userDevice(instance *ec2.Instance) *ec2.BlockDevice {
	for _, dev := range instance.BlockDevices {
		if dev.DeviceName == userDeviceName {
			return &dev
		}
	}
	return nil
}
