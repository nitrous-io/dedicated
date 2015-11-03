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
	Logger
	Executor
	Host   Host
	Writer io.Writer
}

type Step struct {
	Description string
	Func        func(*HostContext) error
	Timeout     time.Duration
}

type StepWithRecovery struct {
	Step
	RecoveryFunc func(*HostContext) error
}

// Run executes a step's Func within the given Timeout.
func (ctx *HostContext) Run(step Step) error {
	w := ctx.Writer
	if w == nil {
		w = ioutil.Discard
	}
	printing := step.Description != ""
	if printing {
		fmt.Fprintf(w, "%s...", step.Description)
	}

	var (
		ch                      = make(chan error, 1)
		stepCtx context.Context = ctx
	)

	if step.Timeout != 0 {
		var cancel context.CancelFunc
		stepCtx, cancel = context.WithTimeout(ctx, step.Timeout)
		defer cancel()
	}

	go func() { ch <- step.Func(ctx) }()

	select {
	case err := <-ch:
		if err != nil {
			if printing {
				fmt.Fprintf(w, "%s... Failed!", step.Description)
			}
			return err
		}
		if printing {
			fmt.Fprintf(w, "%s... Done!", step.Description)
		}
		return nil
	case <-stepCtx.Done():
		if printing {
			fmt.Fprintf(w, "%s... Failed!", step.Description)
		}
		return stepCtx.Err()
	}
}

// NewContext returns the a context which we can use to configure timeout for
// any operations. This does not set Executor of the context, the user has to
// set it explicitly whenever the executor is available.
func NewContext(ctx context.Context, h Host, l Logger, w io.Writer) *HostContext {
	return &HostContext{
		Context: ctx,
		Host:    h,
		Logger:  l,
		Writer:  w,
	}
}
