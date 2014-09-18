package serializers

import "github.com/nitrous-io/freighter/freighterclient"

type VolumeInfo struct {
	Size uint64 `json:"size"`
}

func SerializeVolumeInfo(v *freighterclient.Volume) *VolumeInfo {
	if v == nil {
		return nil
	}
	return &VolumeInfo{
		Size: v.Size,
	}
}
