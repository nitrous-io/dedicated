package serializers_test

import (
	"database/sql"
	"encoding/json"
	"time"

	"github.com/nitrous-io/dedicated/nitrous/models/publickey"
	"github.com/nitrous-io/dedicated/nitrous/serializers"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("public key serializer", func() {
	var (
		createdAt = time.Date(2013, time.November, 1, 0, 0, 0, 0, time.UTC)
		updatedAt = time.Date(2013, time.November, 2, 0, 0, 0, 0, time.UTC)

		model = &publickey.PublicKey{
			ID:          10,
			UserID:      11,
			ContainerID: sql.NullInt64{12, true},
			Fingerprint: "bc:68:22:0a:6b:31:18:f7:52:33:fe:af:d9:ed:9f:81",
			Key:         "ssh-rsa AAAAB3NzaC",
			CreatedAt:   &createdAt,
			UpdatedAt:   &updatedAt,
		}
	)

	Describe("SerializePublicKey", func() {
		It("serializes to JSON", func() {
			k := serializers.SerializePublicKey(model)
			b, err := json.Marshal(k)
			Expect(err).To(BeNil())
			Expect(b).To(MatchJSON(`{
				"id": 10,
				"user_id": 11,
				"container_id": 12,
				"fingerprint": "bc:68:22:0a:6b:31:18:f7:52:33:fe:af:d9:ed:9f:81",
				"key": "ssh-rsa AAAAB3NzaC",
				"created_at": "2013-11-01T00:00:00Z",
				"updated_at": "2013-11-02T00:00:00Z"
			}`))
		})
	})

	Describe("SerializePublicKeys", func() {
		It("serializes to JSON", func() {
			k := serializers.SerializePublicKeys([]*publickey.PublicKey{model})
			b, err := json.Marshal(k)
			Expect(err).To(BeNil())
			Expect(b).To(MatchJSON(`[{
				"id": 10,
				"user_id": 11,
				"container_id": 12,
				"fingerprint": "bc:68:22:0a:6b:31:18:f7:52:33:fe:af:d9:ed:9f:81",
				"key": "ssh-rsa AAAAB3NzaC",
				"created_at": "2013-11-01T00:00:00Z",
				"updated_at": "2013-11-02T00:00:00Z"
			}]`))
		})
	})
})
