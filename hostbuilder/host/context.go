package host

import (
	"fmt"
	"io"
	"io/ioutil"
	"time"

	"golang.org/x/net/context"
)

type key int

const (
	hostKey   key = 0
	writerKey key = 1
)

type HostContext struct {
	context.Context
}

type Step struct {
	Description string
	Func        func(context.Context) error
	Timeout     time.Duration
}

// Run executes a step's Func within the given Timeout.
func (ctx *HostContext) Run(step Step) error {
	w, ok := WriterFromContext(ctx)
	if !ok {
		w = ioutil.Discard
	}
	fmt.Fprintf(w, "%s...", step.Description)

	newCtx, cancel := context.WithTimeout(ctx, step.Timeout)
	defer cancel()

	ch := make(chan error, 1)
	go func() {
		select {
		case <-newCtx.Done():
			return
		default:
			ch <- step.Func(ctx)
		}
	}()

	select {
	case err := <-ch:
		if err != nil {
			fmt.Fprintf(w, "%s... Failed!", step.Description)
			return err
		}
		fmt.Fprintf(w, "%s... Done!", step.Description)
		return nil
	case <-newCtx.Done():
		fmt.Fprintf(w, "%s... Failed!", step.Description)
		return newCtx.Err()
	}
}

func NewContext(ctx context.Context, h Host, w io.Writer) *HostContext {
	childCtx := context.WithValue(ctx, hostKey, h)
	childCtx = context.WithValue(childCtx, writerKey, w)
	return &HostContext{childCtx}
}

func FromContext(ctx context.Context) (Host, bool) {
	h, ok := ctx.Value(hostKey).(Host)
	return h, ok
}

func WriterFromContext(ctx context.Context) (io.Writer, bool) {
	w, ok := ctx.Value(writerKey).(io.Writer)
	return w, ok
}
