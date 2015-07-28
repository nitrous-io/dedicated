package serializers_test

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"sort"
	"time"

	"github.com/lib/pq/hstore"
	"github.com/nitrous-io/dedicated/freighter/freighterclient"
	"github.com/nitrous-io/dedicated/nitrous/models/container"
	"github.com/nitrous-io/dedicated/nitrous/serializers"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("container serializer", func() {
	var (
		touchedAt    = time.Date(2013, time.November, 2, 12, 0, 0, 0, time.UTC)
		terminatedAt = time.Date(2013, time.November, 3, 0, 0, 0, 0, time.UTC)
		createdAt    = time.Date(2013, time.November, 1, 0, 0, 0, 0, time.UTC)
		updatedAt    = time.Date(2013, time.November, 2, 0, 0, 0, 0, time.UTC)
		startedAt    = time.Date(2014, time.August, 13, 0, 0, 0, 0, time.UTC)

		model = &container.Container{
			ID:         10,
			CID:        "deadbeef",
			UserID:     11,
			HostID:     12,
			Image:      "busybox",
			Slug:       "railsbox-10",
			Name:       sql.NullString{"Rails dev box", true},
			IPAddress:  "1.2.3.4",
			GatewayIP:  "1.2.3.0",
			State:      container.StateStopped,
			User:       "guest",
			Cmd:        []string{"/sbin/init"},
			Entrypoint: []string{"/bin/blah"},
			ExitCode:   0,
			Metadata: hstore.Hstore{
				Map: map[string]sql.NullString{
					"Key":  {"Value", true},
					"Some": {"Thing", true},
				},
			},
			StartedAt:    &startedAt,
			FinishedAt:   nil,        // will be omitted
			TouchedAt:    &touchedAt, // Not serialized.
			TerminatedAt: &terminatedAt,
			CreatedAt:    &createdAt,
			UpdatedAt:    &updatedAt,
		}
	)

	Describe("SerializeContainer", func() {
		It("serializes to JSON", func() {
			c := serializers.SerializeContainer(model)
			c.Config = &serializers.ContainerConfig{
				Cmd:                []string{"/bin/yes"},
				Env:                []string{"NITROUS_IMAGE_VERSION=1.2.3", "PAGER=less"},
				ConfigPortBindings: []string{"public::5050", "private::3000"},
				Ports:              []string{"public:4040:5050", "private:4000:3000"},
				Mounts:             []string{"db"},
				Privileged:         true,
				PublishAll:         true,
			}
			b, err := json.Marshal(c)
			Expect(err).To(BeNil())
			Expect(b).To(MatchJSON(fmt.Sprintf(`{
				"id": 10,
				"cid": "deadbeef",
				"user_id": 11,
				"host_id": 12,
				"image": "busybox",
				"slug": "railsbox-10",
				"name": "Rails dev box",
				"ip_address": "1.2.3.4",
				"gateway_ip": "1.2.3.0",
				"state": "%s",
				"user": "guest",
				"cmd": ["/sbin/init"],
				"entrypoint": ["/bin/blah"],
				"exit_code": 0,
				"preview_hostname": "%s",
				"metadata": {
					"Key": "Value",
					"Some": "Thing"
				},
				"user_data": null,
				"started_at": "2014-08-13T00:00:00Z",
				"finished_at": null,
				"terminated_at": "2013-11-03T00:00:00Z",
				"created_at": "2013-11-01T00:00:00Z",
				"updated_at": "2013-11-02T00:00:00Z",
				"config": {
					"config_port_bindings": ["public::5050", "private::3000"],
					"port": ["public:4040:5050", "private:4000:3000"],
					"env": ["NITROUS_IMAGE_VERSION=1.2.3", "PAGER=less"],
					"cmd": ["/bin/yes"],
					"volume": ["db"],
					"privileged": true,
					"publish_all": true
				}
			}`, model.ResolvedState(), model.PreviewHostname())))
		})

	})
})

var _ = Describe("container config serializer", func() {
	It("serializes to JSON", func() {
		priv, pub := true, true
		fconfig := &freighterclient.ContainerConfig{
			ConfigPortBindings: []string{"public::5050", "private::3000"},
			PortBindings:       []string{"public:4040:5050", "private:4000:3000"},
			Env:                []string{"NITROUS_IMAGE_VERSION=1.2.3", "PAGER=less"},
			Cmd:                []string{"/bin/yes"},
			Mounts:             []string{"db"},
			Privileged:         &priv,
			PublishAll:         &pub,
		}
		c := serializers.SerializeContainerConfig(fconfig)
		j, err := json.Marshal(c)
		Expect(err).To(BeNil())
		Expect(j).To(MatchJSON(fmt.Sprintf(`{
			"config_port_bindings": ["public::5050", "private::3000"],
			"port": ["public:4040:5050", "private:4000:3000"],
			"env": ["NITROUS_IMAGE_VERSION=1.2.3", "PAGER=less"],
			"cmd": ["/bin/yes"],
			"volume": ["db"],
			"privileged": true,
			"publish_all": true
		}`)))
	})

	It("filters out special nitrous mounts", func() {
		priv, pub := true, true
		fconfig := &freighterclient.ContainerConfig{
			ConfigPortBindings: []string{"public::5050", "private::3000"},
			PortBindings:       []string{"public:4040:5050", "private:4000:3000"},
			Env:                []string{"NITROUS_IMAGE_VERSION=1.2.3", "PAGER=less"},
			Cmd:                []string{"/bin/yes"},
			Mounts: []string{
				"db",
				"/nitrous-oxide",
				"/magic/bla/slug:/.nitrous-container-slug:ro",
				"/magic/bla/ssh:/nitrous/ssh:ro",
			},
			Privileged: &priv,
			PublishAll: &pub,
		}
		c := serializers.SerializeContainerConfig(fconfig)
		j, err := json.Marshal(c)
		Expect(err).To(BeNil())
		Expect(j).To(MatchJSON(fmt.Sprintf(`{
			"config_port_bindings": ["public::5050", "private::3000"],
			"port": ["public:4040:5050", "private:4000:3000"],
			"env": ["NITROUS_IMAGE_VERSION=1.2.3", "PAGER=less"],
			"cmd": ["/bin/yes"],
			"volume": ["db", "/nitrous-oxide"],
			"privileged": true,
			"publish_all": true
		}`)))
	})
})

var _ = Describe("BySlug", func() {
	It("sorts containers by slug", func() {
		containers := []*serializers.Container{
			{Slug: "Z"},
			{Slug: "AX"},
			{Slug: "AB"},
		}

		sort.Sort(serializers.BySlug(containers))

		Expect(containers).To(Equal([]*serializers.Container{
			{Slug: "AB"},
			{Slug: "AX"},
			{Slug: "Z"},
		}))
	})
})
