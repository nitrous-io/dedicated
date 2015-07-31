package server

// mixpanel event names
const (
	EventContainerCommitted    = "Container Committed"
	EventContainerCreated      = "Container Created"
	EventContainerKeyAdded     = "Container Key Added"
	EventContainerKeyRemoved   = "Container Key Removed"
	EventContainerReconfigured = "Container Reconfigured"
	EventContainerRestarted    = "Container Restarted"
	EventContainerStarted      = "Container Started"
	EventContainerStopped      = "Container Stopped"
	EventContainerTerminated   = "Container Terminated"

	EventHostCreated         = "Host Created"
	EventHostStarted         = "Host Started"
	EventHostStopped         = "Host Stopped"
	EventHostRestarted       = "Host Restarted"
	EventHostTerminated      = "Host Terminated"
	EventHostSoftwareUpdated = "Host Software Updated"
	EventHostRespecced       = "Host Respecced"

	EventImageBuilt   = "Image Built"
	EventImagePulled  = "Image Pulled"
	EventImagePushed  = "Image Pushed"
	EventImageRemoved = "Image Removed"
	EventImageTagged  = "Image Tagged"

	EventRegistryAdded   = "Registry Added"
	EventRegistryRemoved = "Registry Removed"

	EventUserUpdatedBillingProfile = "User Updated Billing Profile"
	EventUserUpdatedProfile        = "User Updated Profile"

	EventVolumeCreated = "Volume Created"
	EventVolumeRemoved = "Volume Removed"

	EventContainerKeyListed        = "Container Key Listed"
	EventContainerKeyAddedToGitHub = "Container Key Added To GitHub"

	EventContainerCollabAdded   = "Container Collaborator Added"
	EventContainerCollabRemoved = "Container Collaborator Removed"

	EventBillingProfileActivated = "Billing Profile Activated"

	EventInvoiceManuallyPaid = "Invoice Manually Paid"

	EventUserDeleted = "User Deleted"
)
