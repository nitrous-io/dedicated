package controllers

import (
	"bufio"
	"bytes"
	"encoding/json"
	"strings"

	"github.com/nitrous-io/dedicated/nitrous/pkg/pubsub"
)

// ProgressEvent represents the progress data that is emitted.
type ProgressEvent struct {
	ID         int64  `json:"id"`
	Slug       string `json:"slug"`
	Progress   string `json:"progress"`
	Repository string `json:"repository"`
}

// ProgressBroadcaster acts like an io.Writer but writes the data into pubsub
// pipe instead
type ProgressPublisher struct {
	EventName  EventName
	Topic      pubsub.Topic
	ID         int64
	Slug       string
	Repository string
	Events     *pubsub.Pubsub

	// handle fragments that is not newline terminated
	buffer string
}

func (b *ProgressPublisher) Write(p []byte) (int, error) {
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
			e := NewEvent(b.EventName, &ProgressEvent{
				ID:         b.ID,
				Slug:       b.Slug,
				Repository: b.Repository,
				Progress:   strings.TrimSpace(line),
			})
			b.Events.Publish(e, b.Topic)
		}
	}
	if err := scanner.Err(); err != nil {
		return 0, err
	}
	return len(p), nil
}
