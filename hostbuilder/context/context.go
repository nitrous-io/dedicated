package context

import (
	"fmt"
	"io"

	"github.com/nitrous-io/dedicated/hostbuilder/host"
)

type stepFn func(c *Context, h host.Host) error

type Step struct {
	Description string
	Fn          stepFn
}

type Context struct {
	steps []Step
	host  host.Host
	w     io.Writer

	errStep Step  // Step that errored.
	err     error // Cache the error of Execute().
}

func (c *Context) Execute() error {
	for _, step := range c.steps {
		fmt.Fprintf(c.w, "%s...", step.Description)
		if err := step.Fn(c, c.host); err != nil {
			c.errStep, c.err = step, err
			fmt.Fprintf(c.w, "%s...Failed!", step.Description)
			return err
		}
		fmt.Fprintf(c.w, "%s...Done!", step.Description)
	}
	return nil
}

func (c *Context) Error() error {
	return fmt.Errorf("%s: %s", c.errStep.Description, c.err)
}

func NewContext(h host.Host, steps []Step, w io.Writer) *Context {
	return &Context{host: h, steps: steps, w: w}
}
