package state

//go:generate stringer -type=State
// The above annotation tells `go generate` to generate a String() method for
// type State. Everytime you update this file, please run:
//   1. `go get golang.org/x/tools/cmd/stringer` (first time only)
//   2. `go generate ./...`
type State int

const (
	Unknown State = iota
	Running
	Stopped
	Starting
	Stopping
	Terminated
	Detached
)
