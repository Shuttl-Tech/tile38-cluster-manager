package core

import (
	"context"
	"sync"
)

func NewThreadContext(ctx context.Context) *Context {
	return &Context{
		wg:      new(sync.WaitGroup),
		Context: ctx,
	}
}

// Context is used to synchronize how and when various event
// loops shut down.
type Context struct {
	wg *sync.WaitGroup
	context.Context
}

// Begin marks the beginning of an async event loop
func (c *Context) Begin() {
	c.wg.Add(1)
}

// End marks the end of an async event loop
func (c *Context) End() {
	c.wg.Done()
}

// Wait waits for all active async goroutines to exit
func (c *Context) Wait() {
	c.wg.Wait()
}
