package context

import (
	"errors"
	"testing"

	"github.com/nitrous-io/hostbuilder/host"
)

func TestContextExecute(t *testing.T) {
	var i int
	increment := func(c *Context, h host.Host) error {
		i += 1
		return nil
	}
	steps := []Step{increment, increment}
	h, _ := host.InitHost("dummy", map[string]string{}, "")
	c := NewContext(h, steps)
	c.Execute()
	if i != len(steps) {
		t.Fatal("Failed to execute the steps")
	}
}

func TestContextExecuteError(t *testing.T) {
	var i int
	increment := func(c *Context, h host.Host) error {
		i += 1
		if i == 2 {
			return errors.New("oh it failed")
		}
		return nil
	}
	steps := []Step{increment, increment, increment}
	h, _ := host.InitHost("dummy", map[string]string{}, "")
	c := NewContext(h, steps)
	err := c.Execute()
	if err == nil || c.Error == nil {
		t.Fatal("Failed to fail fast", err)
	}
	if i != 2 {
		t.Fatal("Failed at incorrect step", i)
	}
}
