package context

import (
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/nitrous-io/dedicated/hostbuilder/host"
	"golang.org/x/net/context"
)

type key int

const (
	hostKey   key = 0
	writerKey key = 1
)

var (
	ErrStepTimeout = errors.New("context step timed out")

	errHostMissing   = errors.New("context does not contain a Host")
	errWriterMissing = errors.New("context does not contain a Writer")
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
	_, w, err := FromContext(ctx)
	if err != nil {
		return err
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
		if newCtx.Err() == context.DeadlineExceeded {
			return ErrStepTimeout
		}
		return newCtx.Err()
	}
}

func NewContext(ctx context.Context, h host.Host, w io.Writer) *HostContext {
	childCtx := context.WithValue(ctx, hostKey, h)
	childCtx = context.WithValue(childCtx, writerKey, w)
	return &HostContext{childCtx}
}

func FromContext(ctx context.Context) (host.Host, io.Writer, error) {
	h, ok := ctx.Value(hostKey).(host.Host)
	if !ok {
		return nil, nil, errHostMissing
	}
	w, ok := ctx.Value(writerKey).(io.Writer)
	if !ok {
		return nil, nil, errWriterMissing
	}
	return h, w, nil
}
