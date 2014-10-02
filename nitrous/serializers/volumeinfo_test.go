package serializers_test

import (
	"encoding/json"

	"github.com/nitrous-io/dedicated/freighter/freighterclient"
	"github.com/nitrous-io/dedicated/nitrous/serializers"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("volume info serializer", func() {
	Describe("SerializeVolumeInfo", func() {
		It("serializes to JSON", func() {
			v := serializers.SerializeVolumeInfo(
				&freighterclient.Volume{Size: 123456})
			b, err := json.Marshal(v)
			Expect(err).To(BeNil())
			Expect(b).To(MatchJSON(`{
				"size": 123456
			}`))
		})
	})
})
