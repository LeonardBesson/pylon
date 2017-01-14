/*
A simple reverse proxy and load balancer

supported balancing strategies:
	- Round Robin
	- Random
	- Least Connected

Example of a json config:

{
  "servers": [
    {
      "name": "server1",
      "port": 7777,
      "services": [
        {
          "route_prefix": "/microservice/",
          "instances": [
            {
              "host": "127.0.0.1:1111",
              "weight": 3
            },
            {
              "host": "127.0.0.1:2222"
            },
            {
              "host": "127.0.0.1:3333"
            }
          ],
          "balancing_strategy": "round_robin",
          "max_connections": 300,
          "health_check": {
            "enabled": true,
            "interval": 30
          }
        }
      ]
    }
  ]
}
*/
package pylon

import (
	"net/http"
	"regexp"
	"fmt"
	"strconv"
	"sync"
	"math/rand"
	"net"
	"time"
	"strings"
	"html/template"
)

var (
	dialer = &net.Dialer{Timeout: dialerTimeout}
	proxyPool *ProxyPool
)

type RouteType int8

const (
	maxFloat32 = float32(^uint(0))
	blacklisted = true

	Regex RouteType = iota
	Prefix RouteType = iota

	defaultMaxCon = 100000
	defaultHealthCheckInterval = 20
	defaultProxyPoolCapacity = defaultMaxCon

	flushInterval = time.Second * 1
	dialerTimeout = time.Second * 3
)

type Pylon struct {
	Services []*MicroService
}

type Route interface {
	Type() RouteType
	Data() interface{}
}

type MicroService struct {
	Route       Route
	Instances   []*Instance
	Strategy    Strategy
	LastUsedIdx SharedInt
	BlackList   map[int]bool
	ReqCount    chan int
	// Caching the weight sum for faster retrieval
	WeightSum   float32
	Mutex       *sync.RWMutex
	HealthCheck HealthCheck
}

type RegexRoute struct {
	Regex *regexp.Regexp
}

type PrefixRoute struct {
	Prefix string
}

func (r RegexRoute) Type() RouteType {
	return Regex
}

func (r RegexRoute) Data() interface{} {
	return r.Regex
}

func (p PrefixRoute) Type() RouteType {
	return Prefix
}

func (p PrefixRoute) Data() interface{} {
	return p.Prefix
}

func ListenAndServe(p string) error {
	jsonParser := JSONConfigParser{}
	c, err := jsonParser.ParseFromPath(p)
	if err != nil {
		return err
	}

	return ListenAndServeConfig(c)
}

func ListenAndServeConfig(c *Config) error {
	wg := sync.WaitGroup{}

	// Initializing the pool before hand in case one server
	// Gets a request as soon as it's served
	poolSize := 0
	for _, s := range c.Servers {
		for _, ser := range s.Services {
			if ser.MaxCon == 0 {
				poolSize += defaultProxyPoolCapacity
			} else {
				poolSize += ser.MaxCon
			}
		}
	}
	fmt.Println("Pool size is", poolSize)
	proxyPool = NewProxyPool(poolSize)

	wg.Add(len(c.Servers))
	for _, s := range c.Servers {
		p, err := NewPylon(&s)
		if err != nil {
			return err
		}
		healthRoute := defaultHealthRoute
		if s.HealthRoute != "" {
			healthRoute = s.HealthRoute
		}
		go func() {
			defer wg.Done()
			serve(p, s.Port, healthRoute)
		}()
	}
	wg.Wait()
	return nil
}

func NewPylon(s *Server) (*Pylon, error) {
	p := &Pylon{}
	for _, ser := range s.Services {
		m, err := NewMicroService(&ser)
		if err != nil {
			return nil, err
		}
		p.Services = append(p.Services, m)
	}

	return p, nil
}

func NewMicroService(s *Service) (*MicroService, error) {
	m := &MicroService{}
	if s.Pattern != "" {
		reg, err := regexp.Compile(s.Pattern)
		if err != nil {
			return nil, err
		}
		m.Route = RegexRoute{
			Regex: reg,
		}
	} else if s.Prefix != "" {
		m.Route = PrefixRoute{
			Prefix: s.Prefix,
		}
	} else {
		return nil, ErrServiceNoRoute
	}

	maxCon := defaultMaxCon
	if s.MaxCon > 0 {
		maxCon = s.MaxCon
	}

	var weightSum float32 = 0.0
	for _, inst := range s.Instances {
		var weight float32 = 1
		if inst.Weight > 0 {
			weight = inst.Weight
		}
		weightSum += weight

		newInst := &Instance{
			inst.Host,
			weight,
			make(chan int, maxCon),
			NewSharedInt(0),
		}
		m.Instances = append(m.Instances, newInst)
	}
	m.Strategy = s.Strategy
	m.Mutex = &sync.RWMutex{}
	m.BlackList = make(map[int]bool, len(s.Instances))
	m.LastUsedIdx = NewSharedInt(0)
	m.ReqCount = make(chan int, maxCon)
	m.WeightSum = weightSum
	m.HealthCheck = s.HealthCheck
	if m.HealthCheck.Interval == 0 {
		m.HealthCheck.Interval = defaultHealthCheckInterval
	}

	return m, nil
}

func serve(p *Pylon, port int, healthRoute string) {
	mux := http.NewServeMux()
	mux.Handle("/", NewPylonHandler(p))
	mux.Handle(healthRoute, NewPylonHealthHandler(p))
	fmt.Println("Serving on " + strconv.Itoa(port))
	server := &http.Server{
		Addr:           ":" + strconv.Itoa(port),
		Handler:        mux,
		ReadTimeout:    20 * time.Second,
		WriteTimeout:   20 * time.Second,
		MaxHeaderBytes: 1 << 20,
	}

	for _, s := range p.Services {
		// Do an initial health check
		go handleHealthCheck(s)

		if s.HealthCheck.Enabled {
			go startPeriodicHealthCheck(s, time.Second * time.Duration(s.HealthCheck.Interval))
		}
	}

	server.ListenAndServe()
}

func startPeriodicHealthCheck(m *MicroService, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for t := range ticker.C {
		fmt.Println("Checking health of Service:", m.Route, " ---tick:", t)
		handleHealthCheck(m)
	}
}

func handleHealthCheck(m *MicroService) bool {
	change := false
	//var weightSum float32 = 0.0
	for i, inst := range m.Instances {
		_, err := dialer.Dial("tcp", inst.Host)
		if err != nil {
			if !m.isBlacklisted(i) {
				m.blackList(i, true)
				change = true
			}
		} else {
			if m.isBlacklisted(i) {
				m.blackList(i, false)
				change = true
			}
			//weightSum += inst.Weight
		}
	}
	//m.Mutex.Lock()
	//m.WeightSum = weightSum
	//m.Mutex.Unlock()
	return change
}

func NewPylonHandler(p *Pylon) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		route, err := getRoute(r.URL.Path, p)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		m := p.microFromRoute(route)
		if m == nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		inst, _, err := m.getLoadBalancedInstance()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		fmt.Println("Instance is " + inst.Host)

		m.ReqCount <- 1
		inst.ReqCount <- 1

		fmt.Println("Serving new request, count: " + strconv.Itoa(len(m.ReqCount)))
		proxy := proxyPool.Get()
		SetUpProxy(proxy, m, inst.Host)
		proxy.ServeHTTP(w, r)
		proxyPool.Put(proxy)

		<-inst.ReqCount
		<-m.ReqCount
		fmt.Println("Request served, count: " + strconv.Itoa(len(m.ReqCount)))
	}
}

func NewPylonHealthHandler(p *Pylon) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		t, err := template.New("PylonHealthTemplate").Parse(pylonTemplate)
		if err != nil {
			errorLogger(err.Error())
		}
		if err := t.Execute(w, getRenders(p)); err != nil {
			errorLogger("Could not render the HTML template")
		}
	}
}

func getRoute(path string, p *Pylon) (string, error) {
	for _, ser := range p.Services {
		switch ser.Route.Type() {
		case Regex:
			reg := ser.Route.Data().(*regexp.Regexp)
			if reg.Match([]byte(path)) {
				return reg.String(), nil
			}
		case Prefix:
			pref := ser.Route.Data().(string)
			if strings.HasPrefix(path, pref) {
				return pref, nil
			}
		default: return "", ErrInvalidRouteType

		}
	}

	return "", NewError(ErrRouteNoRouteCode, "No route available for path " + path)
}

func (p *Pylon) microFromRoute(route string) *MicroService {
	for _, ser := range p.Services {
		switch ser.Route.Type() {
		case Regex:
			if ser.Route.Data().(*regexp.Regexp).String() == route {
				return ser
			}
		case Prefix:
			if ser.Route.Data().(string) == route {
				return ser
			}
		default:
			return nil
		}
	}

	return nil
}

func (m *MicroService) getLoadBalancedInstance() (*Instance, int, error) {
	instCount := len(m.Instances)
	if instCount == 0 {
		return nil, -1, ErrServiceNoInstance
	}

	if len(m.BlackList) == instCount {
		return nil, -1, ErrAllInstancesDown
	}

	instances := make([]*Instance, instCount)
	copy(instances, m.Instances)

	var idx int
	var err error
	for {
		switch m.Strategy {
		case RoundRobin:
			idx, err = nextRoundRobinInstIdx(instances, m.LastUsedIdx.Get())
		case LeastConnected:
			idx = getLeastConInstIdx(instances)
		case Random:
			idx = getRandomInstIdx(instances)
		default:
			return nil, -1, NewError(ErrInvalidStrategyCode, "Unexpected strategy " + string(m.Strategy))
		}

		if err != nil {
			return nil, -1, err
		}

		if m.isBlacklisted(idx) {
			instances[idx] = nil
		} else {
			m.LastUsedIdx.Set(idx)
			return instances[idx], idx, nil
		}
	}
}

func nextRoundRobinInstIdx(instances []*Instance, idx int) (int, error) {
	tryCount := 1
	instCount := len(instances)
	lastNonNil := -1
	for {
		inst := instances[idx]
		if inst != nil {
			if inst.isRoundRobinPicked() {
				break
			}
			lastNonNil = idx
		}
		idx++
		tryCount++
		if tryCount > instCount {
			if lastNonNil != -1 {
				return lastNonNil, nil
			}
			return -1, ErrFailedRoundRobin
		}
		if idx >= instCount {
			idx = 0
		}
	}
	return idx, nil
}

func getLeastConInstIdx(instances []*Instance) int {
	minLoad := maxFloat32
	idx := 0

	for i, inst := range instances {
		if inst == nil {
			continue
		}
		load := float32(len(inst.ReqCount)) / inst.Weight
		if load < minLoad {
			minLoad = load
			idx = i
		}
	}

	return idx
}

func getRandomInstIdx(instances []*Instance) int {
	var weightSum float32 = 0.0
	for _, inst := range instances {
		if inst == nil {
			continue
		}
		weightSum += inst.Weight
	}

	r := rand.Float32() * weightSum

	for i, inst := range instances {
		if inst == nil {
			continue
		}
		r -= inst.Weight
		if r < 0 {
			return i
		}
	}

	return 0
}

func (m *MicroService) notifyReq(in bool) {
	if in {
		m.ReqCount <- 1
	} else {
		<-m.ReqCount
	}
}

func (m *MicroService) blackList(idx int, blacklist bool) {
	m.Mutex.Lock()
	if blacklist {
		m.BlackList[idx] = blacklisted
	} else {
		delete(m.BlackList, idx)
	}
	m.Mutex.Unlock()
}

func (m *MicroService) blackListHost(host string, blacklist bool) {
	for idx, inst := range m.Instances {
		if inst.Host == host {
			m.blackList(idx, blacklist)
		}
	}
}

func (m *MicroService) isBlacklisted(idx int) bool {
	blackListed := false
	m.Mutex.RLock()
	blackListed = m.BlackList[idx]
	m.Mutex.RUnlock()

	return blackListed
}