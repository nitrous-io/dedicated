package images

import (
	"bufio"
	"bytes"
	"encoding/json"
	"strings"

	"github.com/nitrous-io/nitrous/controllers"
	"github.com/nitrous-io/nitrous/pkg/pubsub"
)

// ProgressEvent represents the progress data that is emitted from freighter.
type ProgressEvent struct {
	ID       int64  `json:"id"`
	Progress string `json:"progress"`
}

// progressBroadcaster acts like an io.Writer but writes the data into pubsub
// pipe instead
type progressPublisher struct {
	namespace string
	topic     pubsub.Topic
	imageID   int64
	events    *pubsub.Pubsub

	// handle fragments that is not newline terminated
	buffer string
}

func (b *progressPublisher) Write(p []byte) (int, error) {
	scanner := bufio.NewScanner(bytes.NewBuffer(p))
	for scanner.Scan() {
		line := scanner.Text()
		var tmp map[string]interface{}
		// if we cant unmarshal the json output, this is a fragment message.
		// We should cache it or append to the previous cache to get a full msg
		if err := json.Unmarshal([]byte(line), &tmp); err != nil {
			line = b.buffer + line
			// if the line still not formed a correct json, cached it as a
			// buffer
			if err := json.Unmarshal([]byte(line), &tmp); err != nil {
				b.buffer = line
				line = ""
			} else {
				// we have a valid line now. zero the buffer and continue
				b.buffer = ""
			}
		}

		if line != "" {
			e := controllers.NewEvent(b.namespace, &ProgressEvent{
				ID:       b.imageID,
				Progress: strings.TrimSpace(line),
			})
			b.events.Publish(e, b.topic)
		}
	}
	if err := scanner.Err(); err != nil {
		return 0, err
	}
	return len(p), nil
}
