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
	"strconv"
	"sync"
	"math/rand"
	"net"
	"time"
	"strings"
	"html/template"
)

var (
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
	defaultDialerTimeout = time.Second * 3
)

type Pylon struct {
	Services []*MicroService
}

type Route interface {
	Type() RouteType
	Data() interface{}
}

type MicroService struct {
	Name	    string
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

// ListenAndServe tries to parse the config at the given path and serve it
func ListenAndServe(p string) error {
	jsonParser := JSONConfigParser{}
	c, err := jsonParser.ParseFromPath(p)
	if err != nil {
		return err
	}

	return ListenAndServeConfig(c)
}

// ListenAndServeConfig converts a given config to an exploitable
// structure (MicroService) and serves them
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
	logDebug("Pool size is", poolSize)
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

// NewPylon returns a new Pylon object given a Server
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

// NewMicroService returns a new MicroService object given a Service
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

		m.Instances = append(m.Instances, &Instance{
			inst.Host,
			weight,
			make(chan int, maxCon),
			NewSharedInt(0),
		})
	}
	m.Name = s.Name
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

// serve serves a Pylon with all of its MicroServices given
// a port to listen to and a route that will be used to access
// some stats about this very Pylon
func serve(p *Pylon, port int, healthRoute string) {
	mux := http.NewServeMux()
	mux.Handle("/", NewPylonHandler(p))
	mux.Handle(healthRoute, NewPylonHealthHandler(p))
	server := &http.Server{
		Addr:           ":" + strconv.Itoa(port),
		Handler:        mux,
		ReadTimeout:    20 * time.Second,
		WriteTimeout:   20 * time.Second,
		MaxHeaderBytes: 1 << 20,
	}

	for _, s := range p.Services {
		logDebug("Starting initial health check of service: " + s.Name)
		d := &net.Dialer{
			Timeout: defaultDialerTimeout,
		}
		if s.HealthCheck.DialTO != 0 {
			d.Timeout = time.Second * time.Duration(s.HealthCheck.DialTO)
		}
		// Do an initial health check
		go handleHealthCheck(s, d)

		if s.HealthCheck.Enabled {
			go startPeriodicHealthCheck(s, time.Second * time.Duration(s.HealthCheck.Interval), d)
			logDebug("Periodic Health checks started for service: " + s.Name)
		}
	}

	logInfo("Serving on " + strconv.Itoa(port))
	server.ListenAndServe()
}

// startPeriodicHealthCheck starts a timer that will check
// the health of the given MicroService given an interval and
// a dialer which is used to ping the instances/endpoints
func startPeriodicHealthCheck(m *MicroService, interval time.Duration, d *net.Dialer) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for t := range ticker.C {
		logVerbose("Checking health of Service:", m.Route, " ---tick:", t)
		handleHealthCheck(m, d)
	}
}

// handleHealthCheck checks whether every instance of the given
// MicroService is UP or DOWN. Performed by the given Dialer
func handleHealthCheck(m *MicroService, d *net.Dialer) bool {
	change := false
	for i, inst := range m.Instances {
		_, err := d.Dial("tcp", inst.Host)
		if err != nil {
			if !m.isBlacklisted(i) {
				m.blackList(i, true)
				logInfo("Instance: " + inst.Host + " is now marked as DOWN")
				change = true
			}
		} else {
			if m.isBlacklisted(i) {
				m.blackList(i, false)
				logInfo("Instance: " + inst.Host + " is now marked as UP")
				change = true
			}
		}
	}
	return change
}

// NewPylonHandler returns a func(w http.ResponseWriter, r *http.Request)
// that will handle incoming requests to the given Pylon
func NewPylonHandler(p *Pylon) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		//route, err := p.getRoute(r.URL.Path)
		//if err != nil {
		//	logError(err)
		//	http.Error(w, err.Error(), http.StatusInternalServerError)
		//	return
		//}
		m, err := p.getMicroServiceFromRoute(r.URL.Path)
		if err != nil || m == nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		inst, _, err := m.getLoadBalancedInstance()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		m.ReqCount <- 1
		inst.ReqCount <- 1

		logVerbose("Serving " + r.URL.Path + r.URL.RawQuery + ", current request count: " + strconv.Itoa(len(m.ReqCount)))
		logVerbose("Instance is " + inst.Host)
		proxy := proxyPool.Get()
		setUpProxy(proxy, m, inst.Host)
		proxy.ServeHTTP(w, r)
		proxyPool.Put(proxy)

		<-inst.ReqCount
		<-m.ReqCount
		logVerbose("Request served, count: " + strconv.Itoa(len(m.ReqCount)))
	}
}

// NewPylonHealthHandler returns a func(w http.ResponseWriter, r *http.Request)
// that will collect and render some stats about the given Pylon:
// (Name / Strategy / Current request count)
// For every instance: (UP or DOWN / Host / Weight / Current request count)
func NewPylonHealthHandler(p *Pylon) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		t, err := template.New("PylonHealthTemplate").Parse(pylonTemplate)
		if err != nil {
			logError(err.Error())
		}
		if err := t.Execute(w, getRenders(p)); err != nil {
			logError("Could not render the HTML template")
		}
		logDebug("Served heath page HTML")
	}
}

// getMicroServiceFromRoute returns the first MicroService
// that matches the given route (nil and an error if no
// MicroService could match that route)
func (p *Pylon) getMicroServiceFromRoute(path string) (*MicroService, error) {
	for _, ser := range p.Services {
		switch ser.Route.Type() {
		case Regex:
			reg := ser.Route.Data().(*regexp.Regexp)
			if reg.Match([]byte(path)) {
				return ser, nil
			}
		case Prefix:
			pref := ser.Route.Data().(string)
			if strings.HasPrefix(path, pref) {
				return ser, nil
			}
		default:
			return nil, ErrInvalidRouteType
		}
	}

	return nil, NewError(ErrRouteNoRouteCode, "No route available for path " + path)
}

// getLoadBalancedInstance will return a load balanced Instance
// according to the MicroService strategy and current state
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
			idx, err = getRoundRobinInstIdx(instances, m.LastUsedIdx.Get())
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

// getRoundRobinInstIdx returns the index of the Instance that should be
// picked according to round robin rules and a given slice of Instance
func getRoundRobinInstIdx(instances []*Instance, idx int) (int, error) {
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

// getLeastConInstIdx returns the index of the Instance that should be
// picked according to the least connected rules and a given slice of Instance
// Least Connected returns the least loaded instance, which is
// computed by the current request count divided by the weight of the instance
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

// getRandomInstIdx returns the index of an Instance that is picked
// Randomly from the given slice of Instance. Weights of Instances
// are taken into account
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

// blackList blacklists the Instance of the MicroService
// identified by the given index
func (m *MicroService) blackList(idx int, blacklist bool) {
	m.Mutex.Lock()
	if blacklist {
		m.BlackList[idx] = blacklisted
	} else {
		delete(m.BlackList, idx)
	}
	m.Mutex.Unlock()
}

// blackList blacklists the Instance of the MicroService
// identified by the given Host
func (m *MicroService) blackListHost(host string, blacklist bool) {
	for idx, inst := range m.Instances {
		if inst.Host == host {
			m.blackList(idx, blacklist)
		}
	}
}

// isBlacklisted returns whether the Instance identified
// by the given index is black listed
func (m *MicroService) isBlacklisted(idx int) bool {
	blackListed := false
	m.Mutex.RLock()
	blackListed = m.BlackList[idx]
	m.Mutex.RUnlock()

	return blackListed
}

// Returns whether the Instance should still be picked
// according to the round robin rules and internal state
func (i *Instance) isRoundRobinPicked() bool {
	i.RRPos.Lock()
	defer i.RRPos.Unlock()

	i.RRPos.value++
	if i.RRPos.value > int(i.Weight) {
		i.RRPos.value = 0
		return false
	}

	return true
}