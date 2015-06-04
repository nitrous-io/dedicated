// taken from packer
package amazon

import (
	"errors"
	"fmt"
	"time"

	"github.com/Sirupsen/logrus"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/service/ec2"
)

var (
	ErrResourceNotFound = errors.New("resource not found")
	defaultSleep        = 2 * time.Second
)

// StateRefreshFunc is a function type used for StateChangeConf that is
// responsible for refreshing the item being watched for a state change.
//
// It returns three results. `result` is any object that will be returned
// as the final object after waiting for state change. This allows you to
// return the final updated object, for example an EC2 instance after refreshing
// it.
//
// `state` is the latest state of that object. And `err` is any error that
// may have happened while refreshing the state.
type StateRefreshFunc func() (result interface{}, state *string, err error)

// StateChangeConf is the configuration struct used for `WaitForState`.
type StateChangeConf struct {
	Pending []string
	Target  string
	Refresh StateRefreshFunc
}

// instanceStateRefreshFunc returns a StateRefreshFunc that is used to watch
// an EC2 instance.
func instanceStateRefreshFunc(conn *ec2.EC2, i *ec2.Instance) StateRefreshFunc {
	return func() (interface{}, *string, error) {
		resp, err := conn.DescribeInstances(&ec2.DescribeInstancesInput{
			InstanceIDs: []*string{i.InstanceID},
		})
		if err != nil {
			if ec2err, ok := err.(awserr.Error); ok && (ec2err.Code() == "InvalidInstanceID" || ec2err.Code() == "InvalidInstanceID.NotFound") {
				// Set this to nil as if we didn't find anything.
				resp = nil
			} else {
				logrus.Errorf("Error on InstanceStateRefresh: %s", err)
				return nil, nil, err
			}
		}

		if resp == nil || len(resp.Reservations) == 0 || len(resp.Reservations[0].Instances) == 0 {
			// In cases when we are waiting for a newly created instance, AWS
			// sometimes has consistency issues and doesn't see our instance
			// yet. We just return an empty state instead of an error.
			return nil, nil, nil
		}

		i = resp.Reservations[0].Instances[0]
		// if we dont have any state, retry until we get one
		if i.State == nil {
			return nil, nil, nil
		}
		return i, i.State.Name, nil
	}
}

func volumeAttachmentStateRefreshFunc(provider volumeProvider, volumeID, instanceID *string) StateRefreshFunc {
	return func() (result interface{}, state *string, err error) {
		resp, err := provider.DescribeVolumes(&ec2.DescribeVolumesInput{
			VolumeIDs: []*string{volumeID},
		})
		if err != nil {
			return nil, nil, fmt.Errorf("volumeAttachmentStateRefreshFunc: error getting volume info: %v", err)
		}
		if len(resp.Volumes) != 1 {
			return nil, nil, fmt.Errorf("volumeAttachmentStateRefreshFunc: expected to get one volume but got %+v: %v", resp, err)
		}

		v := resp.Volumes[0]
		for _, att := range v.Attachments {
			if att.InstanceID != nil && *att.InstanceID == *instanceID {
				return v, att.State, nil
			}
		}
		return nil, nil, fmt.Errorf("volumeAttachmentStateRefreshFunc: volume %+v is not attached to %s", v, *instanceID)
	}
}

func volumeStateRefreshFunc(provider volumeProvider, volumeID *string) StateRefreshFunc {
	return func() (result interface{}, state *string, err error) {
		resp, err := provider.DescribeVolumes(&ec2.DescribeVolumesInput{
			VolumeIDs: []*string{volumeID},
		})
		if err != nil {
			return nil, nil, fmt.Errorf("volumeStateRefreshFunc: error getting volume info: %v", err)
		}
		if len(resp.Volumes) != 1 {
			return nil, nil, fmt.Errorf("volumeStateRefreshFunc: expected to get one volume but got %+v: %v", resp, err)
		}

		v := resp.Volumes[0]
		return v, v.State, nil
	}
}

func snapshotStateRefreshFunc(provider volumeProvider, snapshotID *string) StateRefreshFunc {
	return func() (result interface{}, state *string, err error) {
		resp, err := provider.DescribeSnapshots(&ec2.DescribeSnapshotsInput{
			SnapshotIDs: []*string{snapshotID},
		})
		if err != nil {
			return nil, nil, fmt.Errorf("snapshotStateRefreshFunc: error getting snapshot info: %v", err)
		}
		if len(resp.Snapshots) != 1 {
			return nil, nil, fmt.Errorf("snapshotStateRefreshFunc: expected to get one snapshot but got %+v: %v", resp, err)
		}
		s := resp.Snapshots[0]
		return s, s.State, nil
	}
}

// WaitForState watches an object and waits for it to achieve a certain
// state.
func WaitForState(conf *StateChangeConf) (i interface{}, err error) {
	var notfoundTick int
	for {
		var currentState *string
		i, currentState, err = conf.Refresh()
		if err != nil {
			return i, err
		}

		if i == nil || currentState == nil {
			// If we didn't find the resource or its state, check if we have been
			// not finding it for awhile, and if so, report an error.
			notfoundTick += 1
			if notfoundTick > 20 {
				return nil, ErrResourceNotFound
			}
		} else {
			// Reset the counter for when a resource isn't found
			notfoundTick = 0
			state := *currentState
			if state == conf.Target {
				return i, err
			}
			var found bool
			for _, allowed := range conf.Pending {
				if state == allowed {
					found = true
					break
				}
			}
			if !found {
				return nil, fmt.Errorf("state %q, wanted %q", state, conf.Target)
			}
		}

		time.Sleep(defaultSleep)
	}
}
