package tender

import (
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strconv"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("parsePort", func() {
	It("returns port of the request", func() {
		testCases := []struct {
			url  string
			host string
			port int
		}{
			{"http://google.com", "google.com", 80},
			{"https://google.com", "google.com", 443},
			{"http://google.com:8000", "google.com:8000", 8000},
			{"/", "google.com", 80},
		}

		for _, tc := range testCases {
			url, err := url.Parse(tc.url)
			Expect(err).To(BeNil(), "test case should be a valid url")
			p := parsePort(&http.Request{Host: tc.host, URL: url})
			Expect(p).To(Equal(tc.port), "Expect correct port to be returned")
		}
	})
})

var _ = Describe("hostToIP", func() {
	It("changes the request's host and port to the specified IP:Port", func() {
		url, _ := url.Parse("http://google.com:8000")
		r := &http.Request{URL: url, Host: "google.com:8000"}
		hostToIP(r, net.ParseIP("10.0.0.2"), 65333)
		Expect(r.URL.Host).To(Equal("10.0.0.2:65333"), "Expect the host to be changed to ip:port")
	})
})

var _ = Describe("getPublicFreighterdPort", func() {
	It("gets the first public port mapped to the wanted port", func() {
		ports := map[string][]string{
			"4000": []string{},
			"3000": []string{"46521", "8080", "1000"},
			"5000": []string{"40000"},
		}

		p, _ := getPublicFreighterdPort(ports, "3000")
		Expect(p).To(Equal("46521"), "expect the first public port")

		_, err := getPublicFreighterdPort(ports, "4000")
		Expect(err).To(HaveOccurred(), "expect to return error for only private port")
	})
})

var _ = Describe("getHostname", func() {
	It("returns the right-most label before the given base domain", func() {
		testCases := []struct {
			domain     string
			baseDomain string
			expected   string
		}{
			{"yammer.nitrousapp.com:65333", ".nitrousapp.com", "yammer"},
			{"dropbox.nitrousapp.com", ".nitrousapp.com", "dropbox"},
			{"hello.yammer.nitrousapp.com:65333", ".nitrousapp.com", "yammer"},
			{"hello.dropbox.nitrousapp.com", ".nitrousapp.com", "dropbox"},
			{"umm.hello.yammer.nitrousapp.com:65333", ".nitrousapp.com", "yammer"},
			{"trailingdot.nitrousapp.com.", ".nitrousapp.com", "trailingdot"},
		}

		for _, tc := range testCases {
			h, err := getHostname(tc.domain, tc.baseDomain)
			Expect(err).To(BeNil())
			Expect(h).To(Equal(tc.expected), "Expected %s, %s to return %s but got %s",
				tc.domain, tc.baseDomain, tc.expected, h)
		}
	})

	It("returns error when the domain is shorter than base domain", func() {
		_, err := getHostname("nitrousapp.com", ".nitrousapp.com")
		Expect(err).To(Equal(ErrDomainMismatched))
	})

	It("returns error when the domains do not match", func() {
		_, err := getHostname("yammer.nitrousapp.com:3000", ".nitrousdev.com")
		Expect(err).To(Equal(ErrDomainMismatched))
	})
})

var _ = Describe("PortsRange", func() {
	It("returns start/end ports", func() {
		testCases := []struct {
			start, end         string
			startPort, endPort int64
			hasErr             bool
		}{
			{"", "", DefaultStartPort, DefaultEndPort, false},
			{"", "8000", DefaultStartPort, 8000, false},
			{"2000", "", 2000, DefaultEndPort, false},
			{"asdf", "", 0, 0, true},
		}

		for _, tc := range testCases {
			sp, ep, err := PortsRange(tc.start, tc.end)
			if err != nil && !tc.hasErr {
				Fail(fmt.Sprintf("(%#v): unexpected error %s", tc, err))
			}
			if tc.hasErr && err == nil {
				Fail(fmt.Sprintf("(%#v): expect err but got nothing", tc))
			}
			if tc.startPort != sp || tc.endPort != ep {
				Fail(fmt.Sprintf("Expect(%d:%d) but got (%d:%d)", tc.startPort, tc.endPort, sp, ep))
			}
		}
	})

	It("disallows ports < MinPort", func() {
		sp, ep, err := PortsRange(strconv.Itoa(int(MinPort-2)), strconv.Itoa(int(MinPort-1)))
		if err != nil {
			Fail(fmt.Sprintf("unexpected error %s", err))
		}
		Expect(sp).To(Equal(MinPort))
		Expect(ep).To(Equal(MinPort))
	})
})
