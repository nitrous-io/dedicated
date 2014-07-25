package volumes

type Volume struct {
	Slug string `json:"slug"`
	Size uint64 `json:"size"` // ignored apart from volumes:inspect
}
