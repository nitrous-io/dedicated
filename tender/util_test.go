package tender

import (
	"fmt"
	"net"
	"net/http"
	"net/url"

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
			"4000": {"private:8080:4000"},
			"3000": {"public:46521:3000", "public:8080:3000", "private:1000:3000"},
			"5000": {"public:40000:5000", "private:1000:3000"},
		}

		p, _ := getPublicFreighterdPort(ports, "3000")
		Expect(p).To(Equal("46521"), "expect the first public port")

		_, err := getPublicFreighterdPort(ports, "4000")
		Expect(err).To(HaveOccurred(), "expect to return error for only private port")
	})
})

var _ = Describe("getHost", func() {
	It("returns the host's name when domain contains only host", func() {
		var h string
		h, _ = getHost("yammer.nitrousapp.com:65333", ".nitrousapp.com")
		Expect(h).To(Equal("yammer"), "Expect to get correct host's name")

		h, _ = getHost("dropbox.nitrousapp.com", ".nitrousapp.com")
		Expect(h).To(Equal("dropbox"), "Expect to get correct host's name")
	})

	It("returns the host's name when domain contains several subdomains", func() {
		var h string
		h, _ = getHost("hello.yammer.nitrousapp.com:65333", ".nitrousapp.com")
		Expect(h).To(Equal("yammer"), "Expect to get correct host's name")

		h, _ = getHost("hello.dropbox.nitrousapp.com", ".nitrousapp.com")
		Expect(h).To(Equal("dropbox"), "Expect to get correct host's name")
	})

	It("returns error when the domains do not match", func() {
		var err error
		_, err = getHost("yammer.nitrousapp.com:3000", ".nitrousdev.com")
		Expect(err).ToNot(BeNil())
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
})
