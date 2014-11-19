package server

// mixpanel event names
const (
	UserSignedUp = "User Signed Up"
	UserSignedIn = "User Signed In"

	HostCreated   = "Host Created"
	HostDestroyed = "Host Destroyed"

	ContainerCreated      = "Container Created"
	ContainerAccessed     = "Container Token Requested"
	ContainerReconfigured = "Container Reconfigured"
	ContainerKeyAdded     = "Container Key Added"
	ContainerDestroyed    = "Container Destroyed"
)
