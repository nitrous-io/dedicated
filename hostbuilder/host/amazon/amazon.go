package amazon

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Sirupsen/logrus"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/nitrous-io/dedicated/event"
	"github.com/nitrous-io/dedicated/hostbuilder/host"
	"github.com/nitrous-io/dedicated/hostbuilder/state"
	mh "github.com/nitrous-io/dedicated/pkg/metricshelper"
	"github.com/nitrous-io/dedicated/pkg/queue"
	"golang.org/x/net/context"
)

const (
	// represents 1gb of storage
	storageUnit = 1
	// minimum storage size in GB. This is the size of standard root disk used in AMI
	minStorageSize = 4
	// maximum storage size in GB. This is also the maximum size of a single
	// EBS volume
	// http://docs.aws.amazon.com/AWSEC2/latest/APIReference/API_EbsBlockDevice.html
	maxStorageSize = 1024

	updateTimeout            = 30 * time.Second
	resizeRootVolumeTimeout  = 30 * time.Minute
	allocateElasticIPTimeout = 3 * time.Minute
)

var (
	GeneralVolumeType    = aws.String("gp2")
	rootDeviceName       = aws.String("/dev/sda1")
	DefaultCreateTimeout = 10 * time.Minute

	ErrCreateTimeout    = errors.New("timed out creating host")
	ErrInstanceNotFound = errors.New("instance not found")

	KnownRegions = []string{
		"ap-southeast-1",
		"eu-west-1",
		"us-east-1",
		"us-west-2",
	}
)

type Host struct {
	Config    *HostConfig
	notifyURL string // to be set in builder's config
	uuid      string // request UUID set by user
	env       string // env tag to be set for each instance

	// ec2 properties
	credentials *credentials.Credentials
	provider    *ec2.EC2
	instance    *ec2.Instance

	// the following settings are only available during the creation phase
	keypair       *keypair      // temporary keypair of the instance
	createTimeout time.Duration // limit of instance's creation time

	// For respec.
	rootVolume *ec2.Volume
	newVolume  *ec2.Volume
	snapshot   *ec2.Snapshot
	newSize    int64 // New storage size in GB.
}

type HostConfig struct {
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

func (h *Host) CreateWithQueue(qp queue.Publisher, queueInfo *host.QueueInfo) (*host.Info, error) {
	var (
		ctx    context.Context
		cancel context.CancelFunc
	)
	ctx, cancel = context.WithTimeout(context.Background(), h.createTimeout)
	defer cancel()

	defer func() {
		if h.keypair != nil {
			host.PublishMessage(qp, event.HostCreating, h.uuid, queueInfo, "Removing key...", queue.Create)
			h.Printf("keyID=%s Removing key...", h.keypair.name)
			if err := h.keypair.Delete(); err != nil {
				host.PublishMessage(qp, event.HostCreating, h.uuid, queueInfo, "Removing key...Failed!", queue.Create)
				h.Printf("ERROR: Failed to remove key %s", h.keypair.name)
			} else {
				host.PublishMessage(qp, event.HostCreating, h.uuid, queueInfo, "Removing key...Done!", queue.Create)
			}
		}
	}()

	hctx := host.NewContext(ctx, h, h, nil)
	steps := []host.Step{
		{
			Description: "Creating key",
			Func:        createKeypair,
		},
		{
			Description: "Checking image's availability",
			Func:        inspectSourceAMI,
		},
		{
			Description: "Creating workspace",
			Func:        createInstance,
			Timeout:     5 * time.Minute,
		},
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
			host.PublishMessage(qp, event.HostCreateFailed, h.uuid, queueInfo, errorMsg, queue.Create)
			inf, _ := h.Info()
			if err == context.DeadlineExceeded {
				return inf, ErrCreateTimeout
			}

			return inf, err
		}

		host.PublishMessage(qp, event.HostCreating, h.uuid, queueInfo, step.Description, queue.Create)
	}
	info, err := h.Info()
	host.PublishHost(qp, event.HostCreated, h.uuid, queueInfo, info, queue.Create, "")
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
				return nil, ErrCreateTimeout
			}
			return nil, err
		}
	}

	return h.Info()
}

// Respec changes the specs of an existing host its new properties.
// The following changes are supported:
//   1. Changing storage size of root volume.
//   2. TODO Changing instance type.
//   3. Allocating/de-allocating an Elastic IP.
func (h *Host) Respec() (*host.Info, error) {
	label := h.MetricsLabel("respec")
	defer mh.UpdateSince(label, time.Now())

	if err := h.reloadInstance(h.Config.ID); err != nil {
		return nil, mh.Error(err, label)
	}

	// Allocate Elastic IP.
	// If instance already has an Elastic IP allocated, we skip this step.
	addresses, err := getAddresses(h.provider, h.instance.InstanceID)
	if err != nil {
		return nil, mh.Error(err, label)
	}
	if len(addresses) == 0 {
		ctx, cancel := context.WithTimeout(context.Background(), allocateElasticIPTimeout)
		defer cancel()

		hctx := host.NewContext(ctx, h, h, nil)
		err := hctx.Run(host.Step{
			Description: "Allocating static IP",
			Func:        allocateElasticIP,
		})
		if err != nil {
			h.Printf("ERROR: failed to allocate Elastic IP to instance %s: %v", *h.instance.InstanceID, err)
			return nil, mh.Error(err, label)
		}
	}

	// If StorageSize is 0, no change required.
	if h.Config.StorageSize == 0 {
		return h.Info()
	}

	vid := rootVolumeID(h.instance)
	if vid == nil {
		return nil, mh.Error(fmt.Errorf("amazon: cannot find root volume for instance %s", *h.instance.InstanceID), label)
	}
	rootVolume, err := getVolume(h.provider, vid)
	if err != nil {
		return nil, mh.Error(err, label)
	}
	configuredSize := computeSize(h.Config.StorageSize)
	currentSize := *rootVolume.Size
	if currentSize < configuredSize {
		h.Printf("respec: volume resize required (%d to %d)", currentSize, configuredSize)
		if err := h.resizeRootVolume(rootVolume, configuredSize); err != nil {
			return nil, mh.Error(fmt.Errorf("amazon: failed to resize root volume, err: %v", err), label)
		}
	}

	return h.Info()
}

type resizeStep struct {
	host.Step
	restartOnError bool
}

func (h *Host) resizeRootVolume(rootVolume *ec2.Volume, newSize int64) (err error) {
	var (
		ctx    context.Context
		cancel context.CancelFunc
	)
	ctx, cancel = context.WithTimeout(context.Background(), resizeRootVolumeTimeout)
	defer cancel()

	h.rootVolume = rootVolume
	h.newSize = newSize

	hctx := host.NewContext(ctx, h, h, nil)
	steps := []resizeStep{
		{
			Step: host.Step{
				Description: "Stopping workspace",
				Func: func(ctx *host.HostContext) error {
					h := ctx.Host.(*Host)
					return h.stopInstance()
				},
			},
		},
		{
			Step: host.Step{
				Description: "Taking snapshot of disk",
				Func:        snapshotRootVolume,
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
				Func:        swapRootVolume,
			},
		},
		{
			Step: host.Step{
				Description: "Starting workspace",
				Func: func(ctx *host.HostContext) error {
					h := ctx.Host.(*Host)
					_, err := h.Start()
					return err
				},
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
			h.Printf("ERROR: failed in resize root volume step %q: %v", step.Description, err)

			// Start the instance back up if necessary.
			// Note that the time to start the instance is excluded from the
			// context timeout.
			if step.restartOnError {
				if _, e := h.Start(); e != nil {
					h.Printf("failed to start instance %s back up", *h.instance.InstanceID)
				}
			}

			if err == context.DeadlineExceeded {
				return ErrCreateTimeout
			}
			return err
		}
	}

	return nil
}

func snapshotRootVolume(ctx *host.HostContext) error {
	h := ctx.Host.(*Host)
	label := h.MetricsLabel("snapshot_root_volume")
	defer mh.UpdateSince(label, time.Now())

	rootVolume := h.rootVolume
	newSize := h.newSize

	h.Printf("Creating snapshot of root volume %s of instance %s", *rootVolume.VolumeID, *h.instance.InstanceID)

	desc := fmt.Sprintf("Created by hostbuilder for respec of %s from %sGB to %dGB",
		*h.instance.InstanceID, rootVolume.Size, newSize)
	snapshot, err := createSnapshot(h.provider, rootVolume.VolumeID, aws.String(desc))
	if err != nil {
		return fmt.Errorf("amazon: failed to create snapshot of root volume %+v, err: %v", rootVolume, err)
	}

	h.snapshot = snapshot

	if _, err := h.provider.CreateTags(
		&ec2.CreateTagsInput{
			Resources: []*string{snapshot.SnapshotID},
			Tags: []*ec2.Tag{
				{Key: aws.String("InstanceID"), Value: h.instance.InstanceID},
				{Key: aws.String("dedicated"), Value: &h.env},
			},
		}); err != nil {
		h.Printf("ERROR: failed to tag snapshot %v, err: %v", *snapshot.SnapshotID, err)
	}

	h.Printf("Created snapshot %s of volume %s", *snapshot.SnapshotID, rootVolume.VolumeID)

	return nil
}

func createNewVolume(ctx *host.HostContext) error {
	h := ctx.Host.(*Host)
	label := h.MetricsLabel("create_new_volume")
	defer mh.UpdateSince(label, time.Now())

	rootVolume := h.rootVolume
	snapshot := h.snapshot
	newSize := h.newSize

	h.Printf("Creating volume from snapshot %s", *snapshot.SnapshotID)
	newVolume, err := createVolume(h.provider, snapshot.SnapshotID, rootVolume.AvailabilityZone, newSize)
	if err != nil {
		return fmt.Errorf("amazon: failed to create new volume from snapshot %s, err: %v", *snapshot.SnapshotID, err)
	}
	h.Printf("Created new volume %s from snapshot %s", *newVolume.VolumeID, *snapshot.SnapshotID)
	h.newVolume = newVolume
	return nil
}

func swapRootVolume(ctx *host.HostContext) (err error) {
	h := ctx.Host.(*Host)
	label := h.MetricsLabel("swap_root_volume")
	defer mh.UpdateSince(label, time.Now())

	rootVolume := h.rootVolume
	newVolume := h.newVolume

	h.Printf("Detaching root volume %s of instance %s", *rootVolume.VolumeID, *h.instance.InstanceID)
	rootVolume, err = detachVolume(h.provider, rootVolume.VolumeID, h.instance.InstanceID)
	if err != nil {
		return fmt.Errorf("amazon: failed to detach root volume, err: %v", err)
	}
	h.Printf("Detached root volume %s from %s", *rootVolume.VolumeID, *h.instance.InstanceID)

	h.Printf("Attaching new volume %s to instance %s", *newVolume.VolumeID, *h.instance.InstanceID)
	newVolume, err = attachVolume(h.provider, newVolume.VolumeID, h.instance.InstanceID, rootDeviceName)
	if err != nil {
		return fmt.Errorf("amazon: failed to attach new volume %+v to %s, err: %v", newVolume, *h.instance.InstanceID, err)
	}
	h.Printf("Attached new volume %s to instance %s", *newVolume.VolumeID, *h.instance.InstanceID)
	return nil
}

func deleteOldVolume(ctx *host.HostContext) error {
	h := ctx.Host.(*Host)
	label := h.MetricsLabel("delete_old_volume")
	defer mh.UpdateSince(label, time.Now())

	rootVolume := h.rootVolume
	rootVolumeID := *rootVolume.VolumeID

	h.Printf("Deleting old root volume %s of instance %s", rootVolumeID, h.instance.InstanceID)
	if _, err := h.provider.DeleteVolume(&ec2.DeleteVolumeInput{
		VolumeID: rootVolume.VolumeID,
	}); err != nil {
		h.Printf("ERROR: failed to delete old root volume %s for instance %s after respec, err: %v", rootVolumeID, h.instance.InstanceID, err)
	}
	h.Printf("Deleted old root volume %s of instance %s", rootVolumeID, h.instance.InstanceID)
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

	// if eip is disabled for this host, we dont need to release it
	if !h.Config.DisableEIP {
		if ip := h.Config.IPAddress; ip != "" {
			// release elastic IP stored in the client. We cant use
			// h.instance.PublicIPAddress because it's not guaranteed to be
			// the correct IP when the instance is stopped
			if err := releaseAddress(h.provider, aws.String(ip), isVPC(h.instance)); err != nil {
				err = fmt.Errorf("amazon: failed to release ElasticIP %s for %s, err %v",
					h.Config.IPAddress, h.instance.InstanceID, err)
				return mh.Error(err, label)
			}
		}
	}

	// terminate the instance
	if _, err := h.provider.TerminateInstances(
		&ec2.TerminateInstancesInput{
			InstanceIDs: []*string{
				h.instance.InstanceID,
			},
		}); err != nil {
		err = fmt.Errorf("amazon: failed to terminate instance %s, err %v", h.instance.InstanceID, err)
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
	if h.instance == nil || h.instance.InstanceID == nil {
		return nil, fmt.Errorf("Invalid ec2 instance %+v", h.instance)
	}
	return info(h.DriverName(), h.provider.Config.Region, h.instance), nil
}

func (h *Host) Stop() (*host.Info, error) {
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

func (h *Host) stopInstance() error {
	if _, err := h.provider.StopInstances(&ec2.StopInstancesInput{
		InstanceIDs: []*string{
			h.instance.InstanceID,
		},
	}); err != nil {
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

	host.PublishMessage(qp, event.HostStopping, h.uuid, queueInfo, "Stopping host", queue.Stop)

	if err := h.reloadInstance(h.Config.ID); err != nil {
		if info, hostInfoErr := h.Info(); hostInfoErr == nil {
			host.PublishHost(qp, event.HostStopFailed, h.uuid, queueInfo, info, queue.Stop, err.Error())
		} else {
			logrus.Debugf("[ERROR] uuid=%s amazon: failed to send a message due to failure on getting host info: %v", h.uuid, hostInfoErr)
		}
		return nil, mh.Error(err, label)
	}

	if err := h.stopInstance(); err != nil {
		if info, hostInfoErr := h.Info(); hostInfoErr == nil {
			host.PublishHost(qp, event.HostStopFailed, h.uuid, queueInfo, info, queue.Stop, err.Error())
		} else {
			logrus.Debugf("[ERROR] uuid=%s amazon: failed to send a message due to failure on getting host info: %v", h.uuid, hostInfoErr)
		}
		return nil, mh.Error(err, label)
	}

	info, err := h.Info()
	if err == nil {
		host.PublishHost(qp, event.HostStopped, h.uuid, queueInfo, info, queue.Stop, "")
	} else {
		logrus.Debugf("[ERROR] uuid=%s amazon: failed to send a message due to failure on getting host info: %v", h.uuid, err)
	}
	return info, err
}

// Start starts the instance and allocate the desired IP address to it
func (h *Host) Start() (*host.Info, error) {
	label := h.MetricsLabel("start")
	defer mh.UpdateSince(label, time.Now())

	if err := h.reloadInstance(h.Config.ID); err != nil {
		return nil, mh.Error(err, label)
	}

	if _, err := h.provider.StartInstances(
		&ec2.StartInstancesInput{
			InstanceIDs: []*string{h.instance.InstanceID},
		}); err != nil {
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
		if _, err := h.provider.AssociateAddress(&ec2.AssociateAddressInput{
			InstanceID:         h.instance.InstanceID,
			PublicIP:           aws.String(h.Config.IPAddress),
			AllowReassociation: new(bool),
		}); err != nil {
			return nil, mh.Error(err, label)
		}

		// reload the instance to get a new public IP address
		if err := h.reloadInstance(*h.instance.InstanceID); err != nil {
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
		wg sync.WaitGroup

		mu      = &sync.Mutex{} // mutex for lists below
		infos   = []*host.Info{}
		errmsgs []string
	)

	for _, region := range KnownRegions {
		conn := ec2.New(&aws.Config{Region: region, Credentials: h.credentials})
		wg.Add(1)
		go func(conn *ec2.EC2, region string) {
			defer wg.Done()
			regionInfos := []*host.Info{}
			firstTime := true
			var nextToken *string

			for firstTime || nextToken != nil {
				instancesResp, err := conn.DescribeInstances(&ec2.DescribeInstancesInput{
					MaxResults: aws.Long(100),
					NextToken:  nextToken,
				})
				if err != nil {
					errmsg := fmt.Sprintf("failed to get instances in region %s, err %v", region, err)
					h.Printf(errmsg)
					// if failed to retrieve instances for this region, saves
					// the error messages and return
					mu.Lock()
					errmsgs = append(errmsgs, errmsg)
					mu.Unlock()
					return
				}

				nextToken = instancesResp.NextToken
				firstTime = false

				for _, reservation := range instancesResp.Reservations {
					for _, instance := range reservation.Instances {
						checkInstance := false
						// Check the instance environment
						for _, tag := range instance.Tags {
							if *tag.Key == "dedicated" && *tag.Value == h.env {
								checkInstance = true
								break
							}
						}
						if !checkInstance {
							continue
						}
						info := info(h.DriverName(), region, instance)
						regionInfos = append(regionInfos, info)
					}
				}
			}

			// append all regionnal hosts to the global list of hosts
			mu.Lock()
			infos = append(infos, regionInfos...)
			mu.Unlock()
		}(conn, region)
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

// Printf logs the data together with the host uuid and droplet id if they are
// avalaible
func (h *Host) Printf(format string, v ...interface{}) {
	parts := []string{}
	if h.uuid != "" {
		parts = append(parts, fmt.Sprintf("uuid=%s", h.uuid))
	}
	if h.instance != nil {
		parts = append(parts, fmt.Sprintf("instance=%s", *h.instance.InstanceID))
	}
	logrus.Infof(fmt.Sprintf("amazon: %s %s", strings.Join(parts, " "), format), v...)
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
	env := h.env
	if env == "" {
		env = "env"
	}
	return strings.Replace(h.Config.Name, "{{env}}", env, -1)
}

// SSHConfig returns the host's SSH confiration. This configuration is only
// valid during creation phase.
func (h *Host) SSHConfig() *host.SSHConfig {
	return &host.SSHConfig{
		IPAddress:  *h.instance.PublicIPAddress,
		Port:       host.SSHPort,
		Username:   "ubuntu",
		PrivateKey: h.keypair.privateKey,
	}
}

// reloadInstance reload the current instance associated with this Host to the
// instance specified by `id` params. It can be used to refresh the current
// instance, or load a new one
func (h *Host) reloadInstance(id string) error {
	return getInstance(h.provider, id, h)
}

func (h *Host) generateNotifyURL() string {
	return strings.Replace(h.notifyURL, "{{slug}}", strconv.FormatInt(h.Config.HostID, 10), -1)
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
		if h.keypair != nil {
			h.Printf("keyID=%s Leaving instance for inspection: %s", h.keypair.name, h.instance.InstanceID)
		} else {
			h.Printf("keyID=unknown Leaving instance for inspection: %s", h.instance.InstanceID)
		}
	}
}

func NewHost(defaultConfig map[string]string, config string) (host.Host, error) {
	hostConfig := &HostConfig{}
	if err := json.Unmarshal([]byte(config), hostConfig); err != nil {
		logrus.Errorf("uuid=%s amazon: invalid config %s", defaultConfig["uuid"], config)
		return nil, err
	}

	// get create timeout
	createTimeout := DefaultCreateTimeout
	d, err := time.ParseDuration(defaultConfig["create_timeout"])
	if err == nil {
		createTimeout = d
	}

	host := &Host{
		Config:        hostConfig,
		createTimeout: createTimeout,
		uuid:          defaultConfig["uuid"], // this is injected by the server per request
		notifyURL:     defaultConfig["notify_url"],
		env:           defaultConfig["env"],
		credentials: credentials.NewStaticCredentials(
			defaultConfig["access_key"],
			defaultConfig["secret_key"],
			"",
		),
	}

	// workaround for actions that requires multiple regions i.e. Host#List
	// This is set by the caller and should not be serialized as a
	// configuration value
	if _, ok := defaultConfig["ignore_region"]; !ok {
		host.provider = ec2.New(&aws.Config{
			Region:      hostConfig.RegionID,
			Credentials: host.credentials,
		})
	}
	return host, nil
}

func init() {
	host.Hosts["amazon"] = NewHost
}

// createKeypair create a temporary keypair using Amazon API, store the
// temporary private key so we can connect to the host during the creation
// phase. The keypair should be deleted after provisioning finishes.
func createKeypair(ctx *host.HostContext) error {
	h := ctx.Host.(*Host)
	label := h.MetricsLabel("create_key")
	defer mh.UpdateSince(label, time.Now())

	// generate key pair
	k, err := generateTempKeypair(h.provider)
	if err != nil {
		return mh.Error(err, label)
	}
	h.keypair = k
	return nil
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

// createInstance runs the instance with specified config and a temporary
// keypair and wait for it to be ready.
func createInstance(ctx *host.HostContext) error {
	h := ctx.Host.(*Host)
	label := h.MetricsLabel("create_host")
	defer func(start time.Time) {
		// we are interested in the final context of `h` so label is generated
		// inside defer block
		mh.UpdateSince(label, start)
	}(time.Now())

	storageSize := computeSize(h.Config.StorageSize)

	h.Printf("amazon: creating instance in region %s, type: %s, size: %d, name: %s",
		h.Config.RegionID, h.Config.InstanceType, storageSize, h.Name())

	// If we have a non-default VPC / Subnet specified, we can flag
	// AssociatePublicIpAddress to get a Public IP assigned. By default these are not provided.
	// You cannot specify both SubnetId and the NetworkInterface.0.* parameters though, otherwise
	// you get: Network interfaces and an instance-level subnet ID may not be specified on the same request
	// You also need to attach Security Groups to the NetworkInterface instead of the instance,
	// to avoid: Network interfaces and an instance-level security groups may not be specified on
	// the same request
	resp, err := h.provider.RunInstances(&ec2.RunInstancesInput{
		KeyName:      &h.keypair.name,
		ImageID:      &h.Config.SourceAMI, // required
		MaxCount:     aws.Long(1),         // required
		MinCount:     aws.Long(1),         // required
		InstanceType: &h.Config.InstanceType,
		BlockDeviceMappings: []*ec2.BlockDeviceMapping{
			{
				DeviceName: rootDeviceName,
				EBS: &ec2.EBSBlockDevice{
					DeleteOnTermination: aws.Boolean(true),
					VolumeType:          GeneralVolumeType,
					VolumeSize:          &storageSize,
				},
			},
		},
		NetworkInterfaces: []*ec2.InstanceNetworkInterfaceSpecification{
			{
				DeviceIndex:              aws.Long(0),
				AssociatePublicIPAddress: aws.Boolean(true),
				DeleteOnTermination:      aws.Boolean(true),
				SubnetID:                 aws.String(h.Config.SubnetID),
				Groups:                   []*string{&h.Config.SecurityGroupID},
			},
		},
	})
	if err != nil || len(resp.Instances) == 0 {
		return mh.Error(fmt.Errorf("amazon: failed to create new instance %v", err), label)
	}
	h.instance = resp.Instances[0]

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

	if *h.instance.PublicIPAddress == "" {
		return fmt.Errorf("failed to allocate public IP for instance %s", h.instance.InstanceID)
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

	_, err := assignAddress(h.provider, h.instance.InstanceID, isVPC(h.instance))
	if err != nil {
		return mh.Error(err, label)
	}

	// reload the instance to get a new public IP address
	if err := h.reloadInstance(*h.instance.InstanceID); err != nil {
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
	if instance.State == nil {
		return state.Unknown
	}
	switch *instance.State.Name {
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
	return i != nil && i.SubnetID != nil && *i.SubnetID != ""
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

// rootVolumeID returns the volume ID of root device. It returns nil if there's
// no root device associated with the instance.
func rootVolumeID(instance *ec2.Instance) *string {
	for _, dev := range instance.BlockDeviceMappings {
		if dev.DeviceName != nil && *dev.DeviceName == *rootDeviceName {
			if dev.EBS == nil {
				return nil
			}
			return dev.EBS.VolumeID
		}
	}
	return nil
}

// info formats data of an ec2 instance and returns it
func info(provider, region string, i *ec2.Instance) *host.Info {
	if i == nil || i.InstanceID == nil {
		return nil
	}
	info := &host.Info{
		ID:             *i.InstanceID,
		Provider:       provider,
		Region:         region,
		FreighterdPort: host.DefaultPort,
		State:          getState(i),
	}
	if i.PublicIPAddress != nil {
		info.IPAddress = *i.PublicIPAddress
	}
	// Get EC2 instance name
	for _, tag := range i.Tags {
		if *tag.Key == "Name" {
			info.Name = *tag.Value
			break
		}
	}
	return info
}
