package controllers

type EventName string

const (
	EventContainerCreated      EventName = "container.created"
	EventContainerDestroyed    EventName = "container.destroyed"
	EventContainerStarted      EventName = "container.started"
	EventContainerStopped      EventName = "container.stopped"
	EventContainerRestarted    EventName = "container.restarted"
	EventHostCreated           EventName = "host.created"
	EventHostUpdated           EventName = "host.updated"
	EventHostDestroyed         EventName = "host.destroyed"
	EventHostStarted           EventName = "host.started"
	EventHostStopped           EventName = "host.stopped"
	EventHostRestarted         EventName = "host.restarted"
	EventImageCreated          EventName = "image.created"
	EventImageUpdated          EventName = "image.updated"
	EventImageBuilding         EventName = "image.building"
	EventImageBuilt            EventName = "image.built"
	EventImageBuildFailed      EventName = "image.build_failed"
	EventImagePulling          EventName = "image.pulling"
	EventImagePulled           EventName = "image.pulled"
	EventImagePullFailed       EventName = "image.pull_failed"
	EventImageTagged           EventName = "image.tagged"
	EventImagePushed           EventName = "image.pushed"
	EventImageRemoved          EventName = "image.removed"
	EventRegistryAuthenticated EventName = "registry.authenticated"
	EventVolumeCreated         EventName = "volume.created"
	EventVolumeDestroyed       EventName = "volume.destroyed"
)

type Event struct {
	Name EventName   `json:"name"`
	Data interface{} `json:"data"`
}

func NewEvent(name EventName, data interface{}) *Event {
	return &Event{Name: name, Data: data}
}
