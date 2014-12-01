package timeutils_test

import (
	"testing"
	"time"

	"github.com/nitrous-io/dedicated/nitrous/pkg/timeutils"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func Test(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "timeutils")
}

var _ = Describe("timeutils", func() {
	Describe("FirstOfNextMonth()", func() {
		It("returns the correct date", func() {
			testCases := []struct {
				curTime time.Time
				nxtTime time.Time
			}{
				{
					time.Date(2014, 11, 15, 0, 0, 0, 0, time.UTC),
					time.Date(2014, 12, 1, 0, 0, 0, 0, time.UTC),
				},
				{
					time.Date(2014, 12, 15, 0, 0, 0, 0, time.UTC),
					time.Date(2015, 1, 1, 0, 0, 0, 0, time.UTC),
				},
				{
					time.Date(2014, 12, 1, 0, 0, 0, 0, time.UTC),
					time.Date(2015, 1, 1, 0, 0, 0, 0, time.UTC),
				},
			}

			for _, tc := range testCases {
				Expect(timeutils.FirstOfNextMonth(tc.curTime)).To(Equal(tc.nxtTime))
			}
		})
	})
})
