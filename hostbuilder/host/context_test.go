package host_test

import (
	"io/ioutil"
	"time"

	"github.com/nitrous-io/dedicated/hostbuilder/host"
	"github.com/nitrous-io/dedicated/hostbuilder/host/dummy"
	"golang.org/x/net/context"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("context", func() {
	Describe("Run", func() {

		It("runs the given step within its timeout", func() {
			ctx := context.Background()
			h := &dummy.Host{ID: "123"}
			hctx := host.NewContext(ctx, h, ioutil.Discard)

			var counter int
			err := hctx.Run(host.Step{
				Func: func(*host.HostContext) error {
					counter++
					return nil
				},
				Timeout: 10 * time.Millisecond,
			})

			Expect(counter).To(Equal(1))
			Expect(err).To(BeNil())
		})

		It("runs the given step if timeout is not specified", func() {
			ctx := context.Background()
			h := &dummy.Host{ID: "123"}
			hctx := host.NewContext(ctx, h, ioutil.Discard)

			var counter int
			err := hctx.Run(host.Step{
				Func: func(*host.HostContext) error {
					counter++
					return nil
				},
			})

			Expect(counter).To(Equal(1))
			Expect(err).To(BeNil())
		})

		It("returns an error when the step times out before completion", func() {
			ctx := context.Background()
			h := &dummy.Host{ID: "123"}
			hctx := host.NewContext(ctx, h, ioutil.Discard)

			var counter int
			err := hctx.Run(host.Step{
				Func: func(*host.HostContext) error {
					time.Sleep(100 * time.Millisecond)
					counter++
					return nil
				},
				Timeout: 5 * time.Millisecond,
			})

			Expect(counter).To(Equal(0))
			Expect(err).To(MatchError(context.DeadlineExceeded))
		})

		It("times out when the parent context times out", func() {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
			defer cancel()
			h := &dummy.Host{ID: "123"}
			hctx := host.NewContext(ctx, h, ioutil.Discard)

			var counter int
			err := hctx.Run(host.Step{
				Func: func(*host.HostContext) error {
					time.Sleep(100 * time.Millisecond)
					counter++
					return nil
				},
				Timeout: 5 * time.Minute, // Much more than parent context timeout.
			})

			Expect(counter).To(Equal(0))
			Expect(err).To(MatchError(context.DeadlineExceeded))
		})
	})
})
