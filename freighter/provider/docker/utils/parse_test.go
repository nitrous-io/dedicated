package utils

import (
	"testing"

	"github.com/fsouza/go-dockerclient"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func Test(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "provider/docker/utils")
}

var _ = Describe("ParsePortBindings", func() {
	It("parses port bindings in docker-compliant format", func() {
		testCases := []struct {
			specs         []string
			gateway       string
			expected      map[docker.Port][]docker.PortBinding
			expectedError error
		}{
			{
				specs:   []string{"public:3000:3000", "2222:22"},
				gateway: "127.0.0.1",
				expected: map[docker.Port][]docker.PortBinding{
					"3000/tcp": {{"0.0.0.0", "3000"}},
					"22/tcp":   {{"127.0.0.1", "2222"}},
				},
			},
			{
				specs:   []string{"private::3000", "private:2222:22"},
				gateway: "127.0.0.1",
				expected: map[docker.Port][]docker.PortBinding{
					"3000/tcp": {{"127.0.0.1", ""}},
					"22/tcp":   {{"127.0.0.1", "2222"}},
				},
			},
		}

		for _, tc := range testCases {
			results, err := ParsePortBindings(tc.specs, tc.gateway)
			Expect(err).To(BeNil(), "unexpected error when parsing %s %s", tc.specs, tc.gateway)
			Expect(results).To(Equal(tc.expected), "unexpected bindings")
		}
	})
})

var _ = Describe("parsePortSpec", func() {
	It("parses port specs", func() {
		testCases := []struct {
			spec, gateway string
			expected      string
			expectedError error
		}{
			{"public:3000:3000", "127.0.0.1", "0.0.0.0:3000:3000", nil},
			{"private:3000:3000", "127.0.0.1", "127.0.0.1:3000:3000", nil},
			{"public::3000", "127.0.0.1", "0.0.0.0::3000", nil},
			{"private::3000", "127.0.0.1", "127.0.0.1::3000", nil},
			{"10.0.42.1::3000", "127.0.0.1", "10.0.42.1::3000", nil},
			{"3000:3000", "127.0.0.1", "127.0.0.1:3000:3000", nil},
			{"3000", "127.0.0.1", "", ErrInvalidPortSpec},
		}

		for _, tc := range testCases {
			p, err := parsePortSpec(tc.spec, tc.gateway)
			if tc.expectedError != nil {
				Expect(err).To(Equal(tc.expectedError), "unexpected error when parsing %s %s", tc.spec, tc.gateway)
			} else {
				Expect(err).To(BeNil(), "unexpected error when parsing %s %s", tc.spec, tc.gateway)
			}
			Expect(p).To(Equal(tc.expected), "unexpected result when parsing %s %s", tc.spec, tc.gateway)
		}
	})
})
