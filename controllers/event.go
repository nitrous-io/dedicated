package controllers

type Event struct {
	Name string      `json:"name"`
	Data interface{} `json:"data"`
}

func NewEvent(name string, data interface{}) *Event {
	return &Event{Name: name, Data: data}
}
