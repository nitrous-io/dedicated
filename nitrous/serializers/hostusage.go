package serializers

import (
	"time"

	"github.com/nitrous-io/dedicated/nitrous/models/hosttransition"
)

type HostUsage struct {
	Minutes int64     `json:"minutes"`
	Start   time.Time `json:"start_at"`
	End     time.Time `json:"end_at"`
}

func SerializeHostUsage(h *hosttransition.HostUsage) *HostUsage {
	if h == nil {
		return nil
	}
	return &HostUsage{
		Minutes: h.Minutes,
		Start:   h.Start,
		End:     h.End,
	}
}

func SerializeHostUsages(usages []*hosttransition.HostUsage) []*HostUsage {
	items := make([]*HostUsage, len(usages))
	for i, item := range usages {
		items[i] = SerializeHostUsage(item)
	}
	return items
}
