// for internal functions that we dont want to expose to other packages
package images

import (
	"github.com/nitrous-io/nitrous/controllers"
	"github.com/nitrous-io/nitrous/pkg/pubsub"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("progressPublisher", func() {
	It("publishes the progress data", func() {
		events := pubsub.New(128)
		ch := events.Subscribe("user-1")

		pp := &progressPublisher{
			eventName: controllers.EventImagePulling,
			topic:     pubsub.Topic("user-1"),
			imageID:   123,
			events:    events,
		}

		pp.Write([]byte(`{"status":"pulling"}`))

		e := (<-ch).(*controllers.Event)
		Expect(e.Name).To(Equal(controllers.EventImagePulling))
		data := e.Data.(*ProgressEvent)
		Expect(data.Progress).To(Equal(`{"status":"pulling"}`))
	})

	It("handles streaming messages which is not newline terminated properly", func() {
		events := pubsub.New(128)
		ch := events.Subscribe("user-1")

		pp := &progressPublisher{
			eventName: controllers.EventImagePulling,
			topic:     pubsub.Topic("user-1"),
			imageID:   123,
			events:    events,
		}

		pp.Write([]byte(`
		{"status":"pulling"}
		{"status":`))
		pp.Write([]byte(`"this is a long message about `))
		pp.Write([]byte(`big image `))
		pp.Write([]byte(`pulling..."}
		{"status":"pulled"}`))

		e1 := (<-ch).(*controllers.Event)
		e2 := (<-ch).(*controllers.Event)
		e3 := (<-ch).(*controllers.Event)
		Expect(e1.Data.(*ProgressEvent).Progress).To(Equal(`{"status":"pulling"}`))
		Expect(e2.Data.(*ProgressEvent).Progress).To(Equal(`{"status":"this is a long message about big image pulling..."}`))
		Expect(e3.Data.(*ProgressEvent).Progress).To(Equal(`{"status":"pulled"}`))
	})
})
