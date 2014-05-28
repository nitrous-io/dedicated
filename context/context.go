package context

import "github.com/nitrous-io/hostbuilder/host"

type Step func(c *Context, h host.Host) error

type Context struct {
	steps []Step
	host  host.Host

	Error error // cache the error of execute
}

func (c *Context) Execute() error {
	for _, step := range c.steps {
		if err := step(c, c.host); err != nil {
			c.Error = err
			return err
		}
	}
	return nil
}

func NewContext(h host.Host, steps []Step) *Context {
	return &Context{host: h, steps: steps}
}
