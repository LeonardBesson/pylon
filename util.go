package pylon

import (
	"sync"
	"log"
	"fmt"
	"io"
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

const (
	LOG_DEBUG 	   int8 = 1 << 0
	LOG_ERROR 	   int8 = 1 << 1
	LOG_INFO 	   int8 = 1 << 2
	LOG_VERBOSE 	   int8 = 1 << 3
	LOG_NONE 	   int8 = 0
	LOG_EXCEPT_VERBOSE int8 = LOG_DEBUG | LOG_ERROR | LOG_INFO
	LOG_ALL 	   int8 = LOG_DEBUG | LOG_ERROR | LOG_INFO | LOG_VERBOSE
)

var (
	debugLogger   Logger = debugFunc
	errorLogger   Logger = errorFunc
	infoLogger    Logger = infoFunc
	verboseLogger Logger = verboseFunc
	logMask       int8   = LOG_EXCEPT_VERBOSE
)

type Logger func(...interface{})

func SetDebugLogger(l Logger) {
	debugLogger = l
}

func SetErrorLogger(l Logger) {
	errorLogger = l
}

func SetInfoLogger(l Logger) {
	infoLogger = l
}

func SetVerboseLogger(l Logger) {
	verboseLogger = l
}

func SetLogWriter(w io.Writer) {
	log.SetOutput(w)
}

func SetLogLevels(mask int8) {
	logMask = mask
}

func logDebug(a ...interface{}) {
	if logMask & LOG_DEBUG != 0 {
		debugLogger(a)
	}
}

func logError(a ...interface{}) {
	if logMask & LOG_ERROR != 0 {
		errorLogger(a)
	}
}

func logInfo(a ...interface{}) {
	if logMask & LOG_INFO != 0 {
		infoLogger(a)
	}
}

func logVerbose(a ...interface{}) {
	if logMask & LOG_VERBOSE != 0 {
		verboseLogger(a)
	}
}

func debugFunc(a ...interface{}) {
	log.Println("- DEBUG -", a)
}

func infoFunc(a ...interface{}) {
	log.Println("- INFO  -", a)
}

func errorFunc(a ...interface{}) {
	log.Println("- ERROR -", a)
}

func verboseFunc(a ...interface{}) {
	log.Println("- VERBO -", a)
}

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
	Name      string
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
	ErrServiceNoRoute = NewError(ErrServiceNoRouteCode, "Service has no route")
	ErrInvalidRouteType = NewError(ErrInvalidRouteTypeCode, "Route has invalid type")
	ErrServiceNoInstance = NewError(ErrServiceNoInstanceCode, "Service has no instances")
	ErrAllInstancesDown = NewError(ErrAllInstancesDownCode, "All instances are dead")
	ErrFailedRoundRobin = NewError(ErrFailedRoundRobinCode, "No instance can be round robin picked")
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