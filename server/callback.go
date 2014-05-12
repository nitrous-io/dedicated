package server

type Callback interface {
	Invoke(to string, payload string)
}
