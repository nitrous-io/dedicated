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
	"time"

	"github.com/Sirupsen/logrus"
	"github.com/mitchellh/goamz/aws"
	"github.com/mitchellh/goamz/ec2"
	"github.com/nitrous-io/dedicated/event"
	"github.com/nitrous-io/dedicated/hostbuilder/host"
	"github.com/nitrous-io/dedicated/hostbuilder/state"
	mh "github.com/nitrous-io/dedicated/pkg/metricshelper"
	"github.com/nitrous-io/dedicated/pkg/queue"
	"github.com/nitrous-io/dedicated/pkg/shellquote"
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

	GeneralVolumeType  = "gp2"
	IOPSVolumeType     = "io1"
	MagneticVolumeType = "standard"

	// See http://docs.aws.amazon.com/AWSEC2/latest/UserGuide/device_naming.html
	// for device naming on Linux instances.
	rootDeviceName = "/dev/sda1"
	// userDeviceName is the name of the user device/volume that we pass to EC2,
	// e.g. in block device mapping.
	userDeviceName = "/dev/sdu"
	// userDeviceOSName is the name of the user device/volume within the OS.
	userDeviceOSName = "/dev/xvdu"
	// userMountPoint is the mount path of userDeviceName
	userMountPoint = "/mnt/userdata"

	updateTimeout            = 30 * time.Second
	resizeVolumeTimeout      = 30 * time.Minute
	startTimeout             = 5 * time.Minute
	stopTimeout              = 10 * time.Minute
	allocateElasticIPTimeout = 3 * time.Minute
)

var (
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

type Host struct {
	driverConfig

	Config *hostConfig

	requestID string

	// ec2 properties
	auth     aws.Auth // this is used when we want to query all known regions
	provider *ec2.EC2
	instance *ec2.Instance
	ami      *ec2.Image

	// Caches keypair so that we don't have to parse config and read the private
	// key file multiple times.
	keypair *keypair

	// For respec.
	rootVolume *ec2.Volume
	userVolume *ec2.Volume
	newVolume  *ec2.Volume
	snapshot   *ec2.Snapshot
	newSize    int64 // New storage size in GB.
}

func init() {
	host.Register("amazon", &host.RegisteredDriver{
		New: New,
	})
}

func New(driverOptions string, hostOptions string) (host.Host, error) {
	driverCfg := driverConfig{}
	if err := json.Unmarshal([]byte(driverOptions), &driverCfg); err != nil {
		logrus.Errorf("amazon: invalid driver config, err: %v", err)
		return nil, err
	}

	hostCfg := &hostConfig{}
	if err := json.Unmarshal([]byte(hostOptions), hostCfg); err != nil {
		logrus.Errorf("amazon: invalid host config, err: %v", err)
		return nil, err
	}

	auth, err := aws.GetAuth(driverCfg.AccessKey, driverCfg.SecretKey)
	if err != nil {
		logrus.Errorf("amazon: auth failed %v", err)
		return nil, err
	}

	host := &Host{
		driverConfig: driverCfg,
		Config:       hostCfg,
		auth:         auth,
	}

	// workaround for actions that requires multiple regions i.e. Host#List
	// This is set by the caller and should not be serialized as a
	// configuration value
	if driverCfg.IgnoreRegion != "true" {
		region, ok := aws.Regions[hostCfg.RegionID]
		if !ok {
			return nil, fmt.Errorf("amazon: invalid region %q", hostCfg.RegionID)
		}
		host.provider = ec2.New(auth, region)
	}
	return host, nil
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

// startInstance starts the underlying EC2 instance and reassociates it with the
// configured IP address (Config.IPAddress), if necessary.
func (h *Host) startInstance() error {
	if _, err := h.provider.StartInstances(h.instance.InstanceId); err != nil {
		return err
	}

	state := StateChangeConf{
		Pending: []string{"pending", "stopped"},
		Refresh: instanceStateRefreshFunc(h.provider, h.instance),
		Target:  "running",
	}
	h.Printf("waiting for state to become %s", state.Target)
	updatedInstance, err := WaitForState(&state)
	if err != nil {
		return err
	}
	h.instance = updatedInstance.(*ec2.Instance)

	// Reassociate Elastic IP if:
	// 1. configured (i.e. Config.DisableEIP is false), and
	// 2. underlying EC2 instance is not in a VPC.
	// When a VPC instance is stopped, its Elastic EIP remains associated, so
	// we don't have to reassociate. See http://docs.aws.amazon.com/AWSEC2/latest/UserGuide/elastic-ip-addresses-eip.html#VPC_EIP_EC2_Differences.
	if !isVPC(h.instance) && !h.Config.DisableEIP {
		if _, err := h.provider.AssociateAddress(&ec2.AssociateAddress{
			InstanceId:         h.instance.InstanceId,
			PublicIp:           h.Config.IPAddress,
			AllowReassociation: false,
		}); err != nil {
			return err
		}

		// reload the instance to get a new public IP address
		if err := h.reloadInstance(h.instance.InstanceId); err != nil {
			return err
		}
	}

	return nil
}

// terminateInstance releases the EIP associated to the host's instance (if any)
// and terminates the instance.
func (h *Host) terminateInstance() error {
	if err := h.releaseEIP(); err != nil {
		return err
	}

	if _, err := h.provider.TerminateInstances([]string{h.instance.InstanceId}); err != nil {
		return fmt.Errorf("amazon: failed to terminate instance %s, err %v", h.instance.InstanceId, err)
	}

	state := StateChangeConf{
		Pending: []string{"pending", "running", "shutting-down", "stopped", "stopping"},
		Refresh: instanceStateRefreshFunc(h.provider, h.instance),
		Target:  "terminated",
	}
	h.Printf("waiting for state to become %s", state.Target)
	updatedInstance, err := WaitForState(&state)
	if err != nil {
		return err
	}
	h.instance = updatedInstance.(*ec2.Instance)

	return nil
}

func (h *Host) releaseEIP() error {
	if h.Config.DisableEIP {
		return nil
	}

	if ip := h.Config.IPAddress; ip != "" {
		// Release IP address given by the client. We can't use
		// h.instance.PublicIPAddress because it's not guaranteed to be the
		// correct IP when the instance is stopped.
		h.Printf("releasing IP address %s for instance %s", ip, h.instance.InstanceId)
		if err := releaseAddress(h.provider, ip, isVPC(h.instance)); err != nil {
			return fmt.Errorf("amazon: failed to release ElasticIP %s for %s, err %v",
				ip, h.instance.InstanceId, err)
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

func (h *Host) publishHostError(qp queue.Publisher, queueInfo *host.QueueInfo, ev event.Event, action queue.Event, err error) {
	if info, e := h.Info(); e == nil {
		host.PublishHost(qp, ev, h.requestID, queueInfo, info, action, err.Error())
	} else {
		logrus.Debugf("[ERROR] uuid=%s amazon: failed to send a message due to failure on getting host info: %v", h.requestID, e)
	}
}

func (h *Host) getVolumeByDeviceName(deviceName string) (*ec2.Volume, error) {
	device := blockDevice(h.instance, deviceName)
	if device == nil {
		return nil, fmt.Errorf("amazon: cannot find volume for device %s of instance %s", deviceName, h.instance.InstanceId)
	}

	vol, err := h.getVolume(device.VolumeId)
	if err != nil {
		return nil, err
	}

	if vol.VolumeId != device.VolumeId {
		// Sanity check - because using the wrong volume is bad m'kay.
		return nil, fmt.Errorf("amazon: expected volume %s but got unexpected volume: %+v", device.VolumeId, vol)
	}

	for _, att := range vol.Attachments {
		if att.InstanceId == h.instance.InstanceId {
			return vol, nil
		}
	}

	return vol, fmt.Errorf("expected volume %s to be attached to instance %s but it is not", device.VolumeId, h.instance.InstanceId)
}

func (h *Host) getVolume(volumeID string) (*ec2.Volume, error) {
	resp, err := h.provider.Volumes([]string{volumeID}, ec2.NewFilter())
	if err != nil {
		return nil, fmt.Errorf("failed to get volume info of volume ID %s, err: %v", volumeID, err)
	}
	if len(resp.Volumes) != 1 {
		return nil, fmt.Errorf("expected to get 1 volume but got %+v", resp.Volumes)
	}

	return &resp.Volumes[0], nil
}

// tagVolume uses the Name tag on ec2 to tag volumes with convention:
// <type>-<env>-<host id>
// Where 'type' is
// userdata - a hotswap-enabled volume to be mounted as user data
// userroot - pre-hotswap volume that serves both as instance root and user data
// n/a      - hotswap-enabled root volumes will not be tagged
func (h *Host) tagVolume(volumeID, namePrefix string) error {
	name := fmt.Sprintf("%s-%s-%d", namePrefix, h.driverConfig.Env, h.Config.HostID)

	tags := []ec2.Tag{
		{Key: "dedicated", Value: h.driverConfig.Env},
		{Key: "Name", Value: name},
	}

	h.Printf("tagging volume %s with tags %#v", volumeID, tags)
	if _, err := h.provider.CreateTags([]string{volumeID}, tags); err != nil {
		return fmt.Errorf("amazon: failed to tag volume %s with tags %#v, err: %v", volumeID, tags, err)
	}

	return nil
}

// inspectSourceAMI gets the requested AMI's info and saves it.
func inspectSourceAMI(ctx *host.HostContext) error {
	h := ctx.Host.(*Host)
	label := h.MetricsLabel("inspect_source_ami")
	defer mh.UpdateSince(label, time.Now())

	h.Printf("inspecting source ami %q", h.Config.SourceAMI)
	image, err := getImage(h.provider, h.Config.SourceAMI)
	if err != nil {
		return mh.Error(err, label)
	}
	h.Printf("source ami: %#v", image)

	h.ami = image

	return nil
}

// createInstance launches a new instance with host's Config and waits for it to
// enter the "running" state.
func createInstance(ctx *host.HostContext) error {
	h := ctx.Host.(*Host)
	label := h.MetricsLabel("create_host")
	defer mh.UpdateSince(label, time.Now())

	storageSize := computeSize(h.Config.StorageSize)

	h.Printf("amazon: creating instance in region %s, type: %s, size: %d, ami: %s, name: %s",
		h.Config.RegionID, h.Config.InstanceType, storageSize, h.Config.SourceAMI, h.Name())

	kp, err := h.regionKeypair()
	if err != nil {
		return mh.Error(fmt.Errorf("amazon: failed to get region keypair %v", err), label)
	}

	var bdm []ec2.BlockDeviceMapping

	if containsUserDevice(h.ami.BlockDevices) {
		bdm = []ec2.BlockDeviceMapping{
			{
				// The root volume is defined in the AMI's block devices as a
				// snapshot, so we do not need to specify its size.
				DeviceName:          rootDeviceName,
				VolumeType:          GeneralVolumeType,
				DeleteOnTermination: true,
			},
			{
				// The user volume is also in the AMI's block devices
				// and we override its size to what was requested.
				DeviceName:          userDeviceName,
				VolumeSize:          storageSize,
				VolumeType:          GeneralVolumeType,
				DeleteOnTermination: false,
			},
		}
	} else {
		bdm = []ec2.BlockDeviceMapping{
			{
				DeviceName:          rootDeviceName,
				VolumeSize:          storageSize,
				VolumeType:          GeneralVolumeType,
				DeleteOnTermination: true,
			},
		}
	}

	opts := &ec2.RunInstances{
		KeyName:                  kp.name,
		ImageId:                  h.Config.SourceAMI,
		InstanceType:             h.Config.InstanceType,
		SecurityGroups:           h.Config.securityGroups(),
		MinCount:                 0,
		MaxCount:                 0,
		BlockDevices:             bdm,
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

// associateVolumeTags tags the volume where user data is stored.
// For non-hotswap instances, this is the root volume.
// For hotswap instances, this is the separate "user volume".
func associateVolumeTags(ctx *host.HostContext) error {
	h := ctx.Host.(*Host)
	label := h.MetricsLabel("tag_user_volume")
	defer mh.UpdateSince(label, time.Now())

	userDevice := blockDevice(h.instance, userDeviceName)
	if userDevice == nil {
		// no user device = pre-hotswap.  Tag root volume
		rootDevice := blockDevice(h.instance, rootDeviceName)
		if err := h.tagVolume(rootDevice.VolumeId, "userroot"); err != nil {
			return err
		}
	} else {
		// we only name root volumes for non-hotswap instances, i.e. only
		// volumes which contain user data should have a Name tag
		if err := h.tagVolume(userDevice.VolumeId, "userdata"); err != nil {
			return err
		}
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

func attachElasticIP(ctx *host.HostContext) error {
	h := ctx.Host.(*Host)
	if h.Config.DisableEIP {
		return nil
	}
	label := h.MetricsLabel("attach_ip")
	defer mh.UpdateSince(label, time.Now())

	if ip := h.Config.IPAddress; ip != "" {
		h.Printf("attaching IP address %s to instance %s", ip, h.instance.InstanceId)
		if _, err := associateAddress(h.provider, ip, h.instance.InstanceId, isVPC(h.instance)); err != nil {
			if err != errAddressNotAllocated {
				return fmt.Errorf("amazon: failed to attach IP addresss %s to %s, err: %v",
					ip, h.instance.InstanceId, err)
			}

			h.Printf("IP address %s not already allocated, will allocate a new IP", ip)

			// If address is not allocated, we allocate and assign a new one.
			if err := allocateElasticIP(ctx); err != nil {
				return err
			}
		}
	}

	// Reload instance info so it's updated with the configured IP address.
	if err := h.reloadInstance(h.instance.InstanceId); err != nil {
		return mh.Error(err, label)
	}

	return nil
}

func detachElasticIP(ctx *host.HostContext) error {
	h := ctx.Host.(*Host)
	if h.Config.DisableEIP {
		return nil
	}
	label := h.MetricsLabel("detach_ip")
	defer mh.UpdateSince(label, time.Now())

	if ip := h.Config.IPAddress; ip != "" {
		h.Printf("detaching IP address %s from instance %s", ip, h.instance.InstanceId)
		if err := disassociateAddress(h.provider, ip, isVPC(h.instance)); err != nil {
			return fmt.Errorf("amazon: failed to detach IP address %s from %s, err: %v",
				ip, h.instance.InstanceId, err)
		}
	}

	return nil
}

// associateInstanceTags sets the instance's tags which include Name and
// dedicated environment.
func associateInstanceTags(ctx *host.HostContext) error {
	h := ctx.Host.(*Host)
	label := h.MetricsLabel("set_tags")
	defer mh.UpdateSince(label, time.Now())

	return setTags(h.provider, h)
}

// mountUserVolume logs in to instance via SSH to:
// 1. Mount user volume.
// 2. Start docker.
// 3. Start freighterd.
func mountUserVolume(ctx *host.HostContext) error {
	h := ctx.Host.(*Host)
	conn := ctx.Executor

	label := h.MetricsLabel("mount_user_volume")
	defer mh.UpdateSince(label, time.Now())

	// Mount user device, assuming that /etc/fstab already knows the mountpoint.
	if out, err := conn.CombinedOutput(fmt.Sprintf(`sudo mount %s`, userDeviceOSName)); err != nil {
		return fmt.Errorf("failed to mount %s, err: %v, output: %s", userDeviceOSName, err, out)
	}

	// Explicitly resize mounted file system to the size of the volume.
	// This is necessary because we detach and attach EBS volumes of varying
	// sizes, and we want the OS to recognize the new volume's size.
	if out, err := conn.CombinedOutput(fmt.Sprintf(`sudo resize2fs %s`, userDeviceOSName)); err != nil {
		return fmt.Errorf("failed to mount %s, err: %v, output: %s", userDeviceOSName, err, out)
	}

	// Store information about the host on the userVolume
	// In that way we are able to attach host information into system logs.
	// metadata files are stored in the userVolume (/mnt/userdata/etc/metadata)
	ts := time.Now().Format("20060102150405")
	metadataCmd := fmt.Sprintf(`sudo mkdir -p %s/etc/metadata && echo -e %s | sudo tee %s/etc/metadata/metadata{,.%s}`,
		userMountPoint, getMetadata(h), userMountPoint, ts)
	if out, err := conn.CombinedOutput(metadataCmd); err != nil {
		h.Printf("failed to write host information, err: %v, output: %s", err, out)
	}

	if out, err := conn.CombinedOutput(`sudo service docker start`); err != nil {
		return fmt.Errorf("failed to start docker, err: %v, output: %s", err, out)
	}

	if out, err := conn.CombinedOutput(`sudo service freighterd start`); err != nil {
		return fmt.Errorf("failed to start freighterd, err: %v, output: %s", err, out)
	}

	return nil
}

// unmountUserVolume logs in to instance via SSH to:
// 1. Stop freighterd.
// 2. Stop docker.
// 3. Unmount user volume.
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

	// Try to unmount volume. We retry this several times in case there are
	// processes that have open files on the user volume.
	tries := 3
	delay := 1 * time.Second
	unmountOK := false
	for i := 0; i < tries; i++ {
		out, err := conn.CombinedOutput(fmt.Sprintf(`sudo umount %s`, userDeviceOSName))
		if err != nil {
			h.Printf("failed to unmount %s - attempt %d, err: %v, output: %s",
				userDeviceOSName, i+1, err, out)
			time.Sleep(delay)
			continue
		}

		unmountOK = true
		break
	}

	if !unmountOK {
		return fmt.Errorf("could not unmount %s", userDeviceOSName)
	}

	return nil
}

func detachUserVolume(ctx *host.HostContext) error {
	h := ctx.Host.(*Host)

	userVolume, err := h.getVolumeByDeviceName(userDeviceName)
	if err != nil {
		return err
	}

	h.Printf("Detaching user volume %s of instance %s", userVolume.VolumeId, h.instance.InstanceId)
	if _, err := detachVolume(h.provider, userVolume.VolumeId, h.instance.InstanceId); err != nil {
		return err
	}
	h.Printf("Detached user volume %s from %s", userVolume.VolumeId, h.instance.InstanceId)

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

func blockDevice(instance *ec2.Instance, deviceName string) *ec2.BlockDevice {
	for _, dev := range instance.BlockDevices {
		if dev.DeviceName == deviceName {
			return &dev
		}
	}
	return nil
}

func containsUserDevice(bdm []ec2.BlockDeviceMapping) bool {
	for _, m := range bdm {
		if m.DeviceName == userDeviceName {
			return true
		}
	}

	return false
}

// getMetadata returns a quoted string that contains the host metadata as key=value pairs.
// The output is safe to be used in a shell script
func getMetadata(h *Host) string {
	if h == nil {
		return ""
	}

	metadata := []string{
		fmt.Sprintf("instance_id=%s", h.instance.InstanceId),
		fmt.Sprintf("host_id=%d", h.Config.HostID),
		fmt.Sprintf("environment=%s", h.Env),
		fmt.Sprintf("region=%s", h.Config.RegionID), // ex: ap-southeast-1
		fmt.Sprintf("volume_id=%s", h.Config.VolumeID),
		fmt.Sprintf("ip=%s", h.Config.IPAddress),
		fmt.Sprintf("timestamp=%d", time.Now().Unix()),
	}

	return shellquote.QuoteCmd(strings.Join(metadata, "\n"))
}
