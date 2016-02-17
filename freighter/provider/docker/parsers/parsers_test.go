package parsers

import (
	"fmt"
	"testing"

	"github.com/fsouza/go-dockerclient"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func Test(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "parsers")
}

var _ = Describe("ParseRepoAndTag", func() {
	testCases := []struct {
		input, repo, tag string
	}{
		{input: "ubuntu", repo: "ubuntu", tag: ""},
		{input: "ubuntu:14.04", repo: "ubuntu", tag: "14.04"},
		{input: "nitrousio/ubuntu", repo: "nitrousio/ubuntu", tag: ""},
		{input: "nitrousio/ubuntu:14.04", repo: "nitrousio/ubuntu", tag: "14.04"},
		{input: "example.com:5000/ubuntu", repo: "example.com:5000/ubuntu", tag: ""},
		{input: "example.com:5000/ubuntu:14.04", repo: "example.com:5000/ubuntu", tag: "14.04"},
		{input: "example.com:5000/nitrousio/ubuntu", repo: "example.com:5000/nitrousio/ubuntu", tag: ""},
		{input: "example.com:5000/nitrousio/ubuntu:14.04", repo: "example.com:5000/nitrousio/ubuntu", tag: "14.04"},
	}
	for _, tc2 := range testCases {
		tc := tc2
		It(fmt.Sprintf("parses %#v to (%#v, %#v)", tc.input, tc.repo, tc.tag), func() {
			repo, tag := ParseRepoAndTag(tc.input)
			Expect(repo).To(Equal(tc.repo))
			Expect(tag).To(Equal(tc.tag))
		})
	}
})

var _ = Describe("ParseRegistry", func() {
	testCases := []struct {
		input, expected string
	}{
		{input: "ubuntu", expected: ""},
		{input: "ubuntu:14.04", expected: ""},
		{input: "nitrousio/ubuntu", expected: ""},
		{input: "nitrousio/ubuntu:latest", expected: ""},
		{input: "example.com/ubuntu", expected: "example.com"},
		{input: "example.com:5000/ubuntu", expected: "example.com:5000"},
		{input: "example.com:5000/ubuntu:latest", expected: "example.com:5000"},
		{input: "example.com:5000/nitrousio/ubuntu", expected: "example.com:5000"},
		{input: "example.com:5000/nitrousio/ubuntu:14.04", expected: "example.com:5000"},
	}
	for _, tc2 := range testCases {
		tc := tc2
		It(fmt.Sprintf("extracts registry host %#v from %#v", tc.expected, tc.input), func() {
			registry := ParseRegistry(tc.input)
			Expect(registry).To(Equal(tc.expected))
		})
	}
})

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

var _ = Describe("ParseWOutput", func() {
	Context("parses output of `w --no-header <user>` command", func() {
		testCases := []struct {
			expected         bool
			expectedErrStr   string
			threshold        int
			username, output string
		}{
			// error handling of unexpected input
			{false, "did not match expected format", 900, "nitrous", "bad format"},
			{false, "not match username action", 900, "action",
				`nitrous  pts/1                     03:11    2:25m  0.08s  0.08s -zsh`},
			{false, "unexpected idle string", 900, "nitrous",
				`nitrous  pts/1                     03:11    ????  0.08s  0.08s -zsh`},
			{false, "must be less than 3600", 9999, "nitrous",
				`nitrous  pts/1                     03:11    ????  0.08s  0.08s -zsh`},
			// no output means user not signed in and is idle
			{false, "", 900, "nitrous", ""},
			// idle for days means idle
			{false, "", 900, "nitrous",
				`nitrous  pts/1                     03:11    25days 0.08s  0.08s -zsh`},
			// idle for hours means idle
			{false, "", 900, "nitrous",
				`nitrous  pts/1                     03:11    2:25m  0.08s  0.08s -zsh`},
			// idle for 40 minutes means idle
			{false, "", 900, "nitrous",
				`nitrous  pts/1                     03:11    40:00  0.08s  0.08s -zsh`},
			// idle for 10 minutes means active
			{true, "", 900, "nitrous",
				`nitrous  pts/1                     03:11    10:00  0.08s  0.08s -zsh`},
			// check that M:SS is supported
			{true, "", 900, "nitrous",
				`nitrous  pts/1                     03:11    1:23  0.08s  0.08s -zsh`},
			// idle for 39 seconds means active
			{true, "", 900, "nitrous",
				`nitrous  pts/1                     03:11    39.12s  0.08s  0.08s -zsh`},
			// with a FROM column populated
			{true, "", 900, "nitrous",
				`nitrous  pts/0    123.234.123.234   06:49    7.00s  0.08s  0.08s -zsh`},
			// multiple lines, only one active
			{true, "", 900, "nitrous",
				`nitrous  pts/1                     03:11    2days  0.08s  0.08s -zsh
				 nitrous  pts/1                     03:11    2:49m  0.08s  0.08s -zsh
				 nitrous  pts/1                     03:11    39.12s  0.08s  0.08s -zsh`},
			// multiple lines, none active
			{false, "", 900, "nitrous",
				`nitrous  pts/1                     03:11    2days  0.08s  0.08s -zsh
				 nitrous  pts/1                     03:11    2:49m  0.08s  0.08s -zsh`},
			// contain empty line, none active
			{false, "", 900, "nitrous",
				"nitrous  pts/0                     03:06    44:25   0.18s  0.10s -zsh\n"},
		}

		for _, tc0 := range testCases {
			tc := tc0
			It(fmt.Sprintf("is %t with error %s for threshold %d, username %s, output %s", tc.expected, tc.expectedErrStr, tc.threshold, tc.username, tc.output), func() {
				active, err := ParseWOutput(tc.output, tc.username, tc.threshold)
				if tc.expectedErrStr == "" {
					Expect(err).To(BeNil())
				} else {
					Expect(err.Error()).To(ContainSubstring(tc.expectedErrStr))
				}
				Expect(active).To(Equal(tc.expected))
			})
		}
	})
})
