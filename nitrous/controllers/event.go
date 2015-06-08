package controllers

type EventName string

const (
	EventContainerCreated           EventName = "container.created"
	EventContainerDestroying        EventName = "container.destroying"
	EventContainerDestroyed         EventName = "container.destroyed"
	EventContainerStarting          EventName = "container.starting"
	EventContainerStarted           EventName = "container.started"
	EventContainerStopping          EventName = "container.stopping"
	EventContainerStopped           EventName = "container.stopped"
	EventContainerRestarting        EventName = "container.restarting"
	EventContainerRestarted         EventName = "container.restarted"
	EventContainerReconfiguring     EventName = "container.reconfiguring"
	EventContainerReconfigured      EventName = "container.reconfigured"
	EventContainerReconfigureFailed EventName = "container.reconfigure_failed"
	EventHostCreating               EventName = "host.creating"
	EventHostCreated                EventName = "host.created"
	EventHostCreateFailed           EventName = "host.create_failed"
	EventHostUpdated                EventName = "host.updated"
	EventHostDestroyed              EventName = "host.destroyed"
	EventHostStarted                EventName = "host.started"
	EventHostStopped                EventName = "host.stopped"
	EventHostRestarted              EventName = "host.restarted"
	EventHostStarting               EventName = "host.starting"
	EventHostStopping               EventName = "host.stopping"
	EventHostRestarting             EventName = "host.restarting"
	EventHostSoftwareUpdating       EventName = "host.software_updating"
	EventHostSoftwareUpdated        EventName = "host.software_updated"
	EventHostSoftwareUpdateFailed   EventName = "host.software_update_failed"
	EventHostRespeccing             EventName = "host.respeccing"
	EventHostRespecced              EventName = "host.respecced"
	EventHostRespecFailed           EventName = "host.respec_failed"
	EventHostsRefreshed             EventName = "hosts.refreshed"
	EventImageCreated               EventName = "image.created"
	EventImageUpdated               EventName = "image.updated"
	EventImageBuilding              EventName = "image.building"
	EventImageBuilt                 EventName = "image.built"
	EventImageBuildFailed           EventName = "image.build_failed"
	EventImagePulling               EventName = "image.pulling"
	EventImagePulled                EventName = "image.pulled"
	EventImagePullFailed            EventName = "image.pull_failed"
	EventImagePushing               EventName = "image.pushing"
	EventImagePushed                EventName = "image.pushed"
	EventImagePushFailed            EventName = "image.push_failed"
	EventImageRemoved               EventName = "image.removed"
	EventImageTagged                EventName = "image.tagged"
	EventRegistryAdded              EventName = "registry.added"
	EventRegistryRemoved            EventName = "registry.removed"
	EventVolumeCreated              EventName = "volume.created"
	EventVolumeDestroyed            EventName = "volume.destroyed"
	EventInvoiceUpdated             EventName = "invoice.updated"
	EventCollaboratorAdded          EventName = "collaborator.added"
	EventCollaboratorRemoved        EventName = "collaborator.removed"
)

type Event struct {
	Name EventName   `json:"name"`
	Data interface{} `json:"data"`
}

func NewEvent(name EventName, data interface{}) *Event {
	return &Event{Name: name, Data: data}
}
