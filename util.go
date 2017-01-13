package pylon

import (
	"sync"
)

type SharedInt struct {
	*sync.RWMutex
	value int
}

func NewSharedInt(v int) SharedInt {
	return SharedInt{
		&sync.RWMutex{},
		v,
	}
}

func (c *SharedInt) Get() int {
	c.RLock()
	defer c.RUnlock()

	return c.value
}

func (c *SharedInt) Set(v int) int {
	c.Lock()
	defer c.Unlock()

	prev := c.value
	c.value = v

	return prev
}