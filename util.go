package pylon

import (
	"sync"
	"log"
	"fmt"
)

// Util types
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

// Logging
var (
	debugLogger Logger = log.Println
	errorLogger Logger = log.Println
)

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
			     Name:	  {{.Name}}
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
	Name 	  string
	CurrConn  int
	Strat     Strategy
	Instances []InstanceRender
}

func getRenders(p *Pylon) []ServiceRender {
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
			Name: s.Name,
			CurrConn: len(s.ReqCount),
			Strat: s.Strategy,
			Instances: insts,
		}
	}

	return renders
}

// Errors
const (
	ErrServiceNoRouteCode = iota + 30
	ErrInvalidRouteTypeCode
	ErrRouteNoRouteCode
	ErrServiceNoInstanceCode
	ErrAllInstancesDownCode
	ErrInvalidStrategyCode
	ErrFailedRoundRobinCode
	ErrInvalidRouteRegexCode
	ErrInvalidHostCode
)

var (
	ErrServiceNoRoute    = NewError(ErrServiceNoRouteCode,    "Service has no route")
	ErrInvalidRouteType  = NewError(ErrInvalidRouteTypeCode,  "Route has no correct type")
	ErrServiceNoInstance = NewError(ErrServiceNoInstanceCode, "Service has no instances")
	ErrAllInstancesDown  = NewError(ErrAllInstancesDownCode,  "All instances are dead")
	ErrFailedRoundRobin  = NewError(ErrFailedRoundRobinCode,  "No instance can be round robin picked")
)

type Error struct {
	Code    int
	Message string
}

func (e *Error) Error() string {
	return fmt.Sprintf("%s - [Error %d]", e.Message, e.Code)
}

func NewError(code int, message string) *Error {
	return &Error{
		code,
		message,
	}
}