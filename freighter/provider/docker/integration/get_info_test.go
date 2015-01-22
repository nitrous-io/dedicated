package integration

import (
	"fmt"

	"github.com/nitrous-io/dedicated/freighter/provider"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("GetInfo", func() {
	It("returns container info", func() {
		prov, info, cleanupFn, err := startContainer("nitrous/busybox:latest", "")
		defer cleanupFn()
		Expect(err).To(BeNil(), "start the container successfully")

		id := info.ID
		getInfo, err := prov.GetInfo(id)
		Expect(err).To(BeNil(), "failed to get info")
		Expect(getInfo).To(Equal(info))
	})

	It("returns an AppState that indicates whether the NITROUS_CMD process is running", func() {
		cmd := "yes"
		configJSON := fmt.Sprintf(`{ "Env": ["NITROUS_CMD=%s"] }`, cmd)
		// use busybox here because we dont want user to be set
		// su in busybox doesnt support all the options that is required ...
		prov, info, cleanupFn, err := startContainer("busybox:latest", configJSON)
		defer cleanupFn()
		Expect(err).To(BeNil(), "failed to start the container")

		getInfo, _ := prov.GetInfo(info.ID)
		Expect(getInfo.AppState).To(Equal(provider.ContainerAppRunning), "expect running app")

		err = prov.StopApp(info.ID)
		Expect(err).To(BeNil(), "failed to stop app")

		getInfo, _ = prov.GetInfo(info.ID)
		Expect(getInfo.AppState).To(Equal(provider.ContainerAppStopped), "expect stopped app")
	})
})
