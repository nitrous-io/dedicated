package docker

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("provider/docker (config)", func() {
	configJSON := `{
  "Config": {
    "Env": ["LANG=C"],
    "ExposedPorts": {"8080/tcp": {}}
  }
}`
	hostConfigJSON := `{
  "HostConfig": {
    "PortBindings": {"3000/tcp": [{"HostPort":"1234", "HostIp": "127.0.0.1"}]}
  }
}`

	Describe("parseConfigParam", func() {
		It("handles empty string", func() {
			c, h, err := parseConfigParam("")
			Expect(c).To(BeNil())
			Expect(h).To(BeNil())
			Expect(err).To(BeNil())
		})

		It("parses Config params", func() {
			c, h, err := parseConfigParam(configJSON)
			Expect(err).To(BeNil())
			Expect(h).To(BeNil())
			Expect(c.Env).To(Equal([]string{"LANG=C"}))
			_, ok := c.ExposedPorts["8080/tcp"]
			Expect(ok).To(BeTrue())
		})

		It("parses HostConfig params", func() {
			c, h, err := parseConfigParam(hostConfigJSON)
			Expect(err).To(BeNil())
			Expect(c).To(BeNil())
			pb, ok := h.PortBindings["3000/tcp"]
			Expect(ok).To(BeTrue())
			Expect(pb).To(HaveLen(1))
			Expect(pb[0].HostIp).To(Equal("127.0.0.1"))
			Expect(pb[0].HostPort).To(Equal("1234"))
		})

		It("returns error", func() {
			c, h, err := parseConfigParam("not json")
			Expect(c).To(BeNil())
			Expect(h).To(BeNil())
			Expect(err.Error()).To(ContainSubstring("invalid character"))
		})
	})
})
