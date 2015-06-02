package serializers_test

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/lib/pq"
	"github.com/nitrous-io/dedicated/nitrous/models/host"
	"github.com/nitrous-io/dedicated/nitrous/models/user"
	"github.com/nitrous-io/dedicated/nitrous/serializers"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("host serializer", func() {
	var (
		terminatedAt = time.Date(2013, time.November, 3, 0, 0, 0, 0, time.UTC)
		createdAt    = time.Date(2013, time.November, 1, 0, 0, 0, 0, time.UTC)
		updatedAt    = time.Date(2013, time.November, 2, 0, 0, 0, 0, time.UTC)
		lastPingedAt = time.Date(2013, time.November, 2, 1, 0, 0, 0, time.UTC)

		model *host.Host
	)
	BeforeEach(func() {
		model = &host.Host{
			ID:                    10,
			UserID:                11,
			RegionID:              12,
			PlanID:                13,
			ProviderTemplateID:    14, // Not serialized.
			Slug:                  "wild-rocket-23",
			Name:                  sql.NullString{"wildrocket.xxx", true},
			State:                 "running",
			IPAddress:             sql.NullString{"1.2.3.4", true},
			FreighterdPort:        sql.NullInt64{65000, true},
			FreighterdVersion:     "1.0.69-bed0c-88",
			DockerVersion:         "1.4.1-nitrous",
			ProviderIdentifier:    sql.NullString{"digitalocean", true},
			MaxContainers:         sql.NullInt64{3, true},
			Size:                  sql.NullString{"micro", true},
			TerminatedAt:          &terminatedAt,
			CreatedAt:             &createdAt,
			UpdatedAt:             &updatedAt,
			LastPingedAt:          pq.NullTime{lastPingedAt, true},
			LiveDockerVersion:     "1.2-docker",
			LiveFreighterdVersion: "2.3-freighterd",
			UpdateAvailable:       host.UpdateAvailableHard,
			PendingRespec:         true,
		}
	})

	Describe("SerializeHost", func() {
		It("serializes to JSON", func() {
			b, err := json.Marshal(serializers.SerializeHost(model))
			Expect(err).To(BeNil())
			Expect(b).To(MatchJSON(fmt.Sprintf(`{
				"id": 10,
				"user_id": 11,
				"region_id": 12,
				"plan_id": 13,
				"slug": "wild-rocket-23",
				"name": "wildrocket.xxx",
				"ip_address": "1.2.3.4",
				"static_ip": false,
				"freighterd_port": 65000,
				"freighterd_version": "1.0.69-bed0c-88",
				"docker_version": "1.4.1-nitrous",
				"provider_identifier": "digitalocean",
				"hostname": "%s",
				"proxy_hostname": "%s",
				"size": "micro",
				"state": "running",
				"terminated_at": "2013-11-03T00:00:00Z",
				"created_at": "2013-11-01T00:00:00Z",
				"updated_at": "2013-11-02T00:00:00Z",
				"last_pinged_at": "2013-11-02T01:00:00Z",
				"live_docker_version": "1.2-docker",
				"live_freighterd_version": "2.3-freighterd",
				"update_available": "hard",
				"max_containers": 3,
				"pending_respec": true
			}`, model.Hostname(), model.ProxyHostname())))
		})

		It("serializes AssignedAt in the created_at field if present", func() {
			assignedAt := time.Date(2013, time.November, 4, 0, 0, 0, 0, time.UTC)
			model.AssignedAt = &assignedAt

			b, err := json.Marshal(serializers.SerializeHost(model))
			Expect(err).To(BeNil())
			Expect(b).To(MatchJSON(fmt.Sprintf(`{
				"id": 10,
				"user_id": 11,
				"region_id": 12,
				"plan_id": 13,
				"slug": "wild-rocket-23",
				"name": "wildrocket.xxx",
				"ip_address": "1.2.3.4",
				"static_ip": false,
				"freighterd_port": 65000,
				"freighterd_version": "1.0.69-bed0c-88",
				"docker_version": "1.4.1-nitrous",
				"provider_identifier": "digitalocean",
				"hostname": "%s",
				"proxy_hostname": "%s",
				"size": "micro",
				"state": "running",
				"terminated_at": "2013-11-03T00:00:00Z",
				"created_at": "2013-11-04T00:00:00Z",
				"updated_at": "2013-11-02T00:00:00Z",
				"last_pinged_at": "2013-11-02T01:00:00Z",
				"live_docker_version": "1.2-docker",
				"live_freighterd_version": "2.3-freighterd",
				"update_available": "hard",
				"max_containers": 3,
				"pending_respec": true
			}`, model.Hostname(), model.ProxyHostname())))
		})

		It("serializes LastRunningAt in the last_running_at if present", func() {
			lastRunning := pq.NullTime{time.Date(2013, time.November, 8, 0, 0, 0, 0, time.UTC), true}
			model.LastRunningAt = lastRunning

			b, err := json.Marshal(serializers.SerializeHost(model))
			Expect(err).To(BeNil())
			Expect(b).To(MatchJSON(fmt.Sprintf(`{
				"id": 10,
				"user_id": 11,
				"region_id": 12,
				"plan_id": 13,
				"slug": "wild-rocket-23",
				"name": "wildrocket.xxx",
				"ip_address": "1.2.3.4",
				"static_ip": false,
				"freighterd_port": 65000,
				"freighterd_version": "1.0.69-bed0c-88",
				"docker_version": "1.4.1-nitrous",
				"provider_identifier": "digitalocean",
				"hostname": "%s",
				"proxy_hostname": "%s",
				"size": "micro",
				"state": "running",
				"terminated_at": "2013-11-03T00:00:00Z",
				"created_at": "2013-11-01T00:00:00Z",
				"updated_at": "2013-11-02T00:00:00Z",
				"last_running_at": "2013-11-08T00:00:00Z",
				"last_pinged_at": "2013-11-02T01:00:00Z",
				"live_docker_version": "1.2-docker",
				"live_freighterd_version": "2.3-freighterd",
				"update_available": "hard",
				"max_containers": 3,
				"pending_respec": true
			}`, model.Hostname(), model.ProxyHostname())))
		})

		It("omits last_pinged_at if LastPingedAt is absent", func() {
			model.LastPingedAt.Valid = false
			b, err := json.Marshal(serializers.SerializeHost(model))
			Expect(err).To(BeNil())
			Expect(b).To(MatchJSON(fmt.Sprintf(`{
				"id": 10,
				"user_id": 11,
				"region_id": 12,
				"plan_id": 13,
				"slug": "wild-rocket-23",
				"name": "wildrocket.xxx",
				"ip_address": "1.2.3.4",
				"static_ip": false,
				"freighterd_port": 65000,
				"freighterd_version": "1.0.69-bed0c-88",
				"docker_version": "1.4.1-nitrous",
				"provider_identifier": "digitalocean",
				"hostname": "%s",
				"proxy_hostname": "%s",
				"size": "micro",
				"state": "running",
				"terminated_at": "2013-11-03T00:00:00Z",
				"created_at": "2013-11-01T00:00:00Z",
				"updated_at": "2013-11-02T00:00:00Z",
				"live_docker_version": "1.2-docker",
				"live_freighterd_version": "2.3-freighterd",
				"update_available": "hard",
				"max_containers": 3,
				"pending_respec": true
			}`, model.Hostname(), model.ProxyHostname())))
		})

		It("serializes to null JSON when host is nil", func() {
			b, err := json.Marshal(serializers.SerializeHost(nil))
			Expect(err).To(BeNil())
			Expect(b).To(MatchJSON(`null`))
		})
	})

	Describe("SerializeHostWithUser", func() {
		It("serializes just the host to JSON if no user is provided", func() {
			lastRunning := pq.NullTime{time.Date(2013, time.November, 8, 0, 0, 0, 0, time.UTC), true}
			model.LastRunningAt = lastRunning

			b, err := json.Marshal(serializers.SerializeHostWithUser(model, nil))
			Expect(err).To(BeNil())
			Expect(b).To(MatchJSON(fmt.Sprintf(`{
				"id": 10,
				"user_id": 11,
				"region_id": 12,
				"plan_id": 13,
				"slug": "wild-rocket-23",
				"name": "wildrocket.xxx",
				"ip_address": "1.2.3.4",
				"static_ip": false,
				"freighterd_port": 65000,
				"freighterd_version": "1.0.69-bed0c-88",
				"docker_version": "1.4.1-nitrous",
				"provider_identifier": "digitalocean",
				"hostname": "%s",
				"proxy_hostname": "%s",
				"size": "micro",
				"state": "running",
				"terminated_at": "2013-11-03T00:00:00Z",
				"created_at": "2013-11-01T00:00:00Z",
				"updated_at": "2013-11-02T00:00:00Z",
				"last_running_at": "2013-11-08T00:00:00Z",
				"last_pinged_at": "2013-11-02T01:00:00Z",
				"live_docker_version": "1.2-docker",
				"live_freighterd_version": "2.3-freighterd",
				"update_available": "hard",
				"max_containers": 3,
				"pending_respec": true
			}`, model.Hostname(), model.ProxyHostname())))
		})

		It("sets TimeRemaining with time remaining relative to the current time", func() {
			model.LastRunningAt = pq.NullTime{time.Now(), true}

			u := &user.User{
				MaxRunningTime: sql.NullInt64{3600, true},
			}
			ser := serializers.SerializeHostWithUser(model, u)
			Expect(*ser.TimeRemaining).To(BeNumerically("~", int64(3599), 1.00))
		})
	})

	Describe("SerializeHosts", func() {
		It("serializes to JSON", func() {
			lastRunning := pq.NullTime{time.Date(2013, time.November, 8, 0, 0, 0, 0, time.UTC), true}
			model.LastRunningAt = lastRunning

			hosts := serializers.SerializeHosts([]host.Host{*model})
			b, err := json.Marshal(hosts)
			Expect(err).To(BeNil())
			Expect(b).To(MatchJSON(fmt.Sprintf(`[{
				"id": 10,
				"user_id": 11,
				"region_id": 12,
				"plan_id": 13,
				"slug": "wild-rocket-23",
				"name": "wildrocket.xxx",
				"ip_address": "1.2.3.4",
				"static_ip": false,
				"freighterd_port": 65000,
				"freighterd_version": "1.0.69-bed0c-88",
				"docker_version": "1.4.1-nitrous",
				"provider_identifier": "digitalocean",
				"hostname": "%s",
				"proxy_hostname": "%s",
				"size": "micro",
				"state": "running",
				"terminated_at": "2013-11-03T00:00:00Z",
				"created_at": "2013-11-01T00:00:00Z",
				"updated_at": "2013-11-02T00:00:00Z",
				"last_running_at": "2013-11-08T00:00:00Z",
				"last_pinged_at": "2013-11-02T01:00:00Z",
				"live_docker_version": "1.2-docker",
				"live_freighterd_version": "2.3-freighterd",
				"update_available": "hard",
				"max_containers": 3,
				"pending_respec": true
			}]`, model.Hostname(), model.ProxyHostname())))
		})
	})

	Describe("SerializeHostsWithUser", func() {
		It("serializes just the host to JSON if no user is provided", func() {
			lastRunning := pq.NullTime{time.Date(2013, time.November, 8, 0, 0, 0, 0, time.UTC), true}
			model.LastRunningAt = lastRunning

			hosts := serializers.SerializeHostsWithUser([]host.Host{*model}, nil)
			b, err := json.Marshal(hosts)
			Expect(err).To(BeNil())
			Expect(b).To(MatchJSON(fmt.Sprintf(`[{
				"id": 10,
				"user_id": 11,
				"region_id": 12,
				"plan_id": 13,
				"slug": "wild-rocket-23",
				"name": "wildrocket.xxx",
				"ip_address": "1.2.3.4",
				"static_ip": false,
				"freighterd_port": 65000,
				"freighterd_version": "1.0.69-bed0c-88",
				"docker_version": "1.4.1-nitrous",
				"provider_identifier": "digitalocean",
				"hostname": "%s",
				"proxy_hostname": "%s",
				"size": "micro",
				"state": "running",
				"terminated_at": "2013-11-03T00:00:00Z",
				"created_at": "2013-11-01T00:00:00Z",
				"updated_at": "2013-11-02T00:00:00Z",
				"last_running_at": "2013-11-08T00:00:00Z",
				"last_pinged_at": "2013-11-02T01:00:00Z",
				"live_docker_version": "1.2-docker",
				"live_freighterd_version": "2.3-freighterd",
				"update_available": "hard",
				"max_containers": 3,
				"pending_respec": true
			}]`, model.Hostname(), model.ProxyHostname())))
		})

		It("sets TimeRemaining with time remaining relative to the current time", func() {
			model.LastRunningAt = pq.NullTime{time.Now(), true}

			u := &user.User{
				MaxRunningTime: sql.NullInt64{3600, true},
			}
			ser := serializers.SerializeHostsWithUser([]host.Host{*model}, u)
			Expect(*ser[0].TimeRemaining).To(BeNumerically("~", int64(3599), 1.00))
		})
	})
})
