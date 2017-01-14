package pylon

import (
	"sync"
	"log"
)

var (
	debugLogger Logger = log.Println
	errorLogger Logger = log.Println
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

type Logger func(...interface{})

func SetDebugLogger(logger Logger) {
	debugLogger = logger
}

func SetErrorLogger(logger Logger) {
	errorLogger = logger
}

// Health Page
const (
	pylonTemplate = `{{range .}}
			     Connections: {{.CurrConn}}
			     Strategy:    {{.Strat}}
			     {{range .Instances}}
			         Up: 	      {{.Up}}
			         Host: 	      {{.Host}}
			         Weight:      {{.Weight}}
			         Connections: {{.CurrConn}}
			     {{end}}
		         {{end}}`
	defaultHealthRoute = "/health"
)

type InstanceRender struct {
	Up       bool
	Host     string
	Weight   float32
	CurrConn int
}

type ServiceRender struct {
	CurrConn  int
	Strat     Strategy
	Instances []InstanceRender
}

func getRenders(p *Pylon) []ServiceRender{
	renders := make([]ServiceRender, len(p.Services))
	for idx, s := range p.Services {
		insts := make([]InstanceRender, len(s.Instances))
		for i, ins := range s.Instances {
			insts[i] = InstanceRender{
				Up: !s.isBlacklisted(i),
				Host: ins.Host,
				Weight: ins.Weight,
				CurrConn: len(ins.ReqCount),
			}
		}

		renders[idx] = ServiceRender{
			CurrConn: len(s.ReqCount),
			Strat: s.Strategy,
			Instances: insts,
		}
	}

	return renders
}