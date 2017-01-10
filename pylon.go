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
	"errors"
	"fmt"
	"strconv"
	"sync"
	"math/rand"
	"net"
	"time"
	"strings"
)

var (
	dialer = &net.Dialer{Timeout: dialerTimeout}
	NoService error = errors.New("Service has no route")
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
	//LastUsedIdx chan int
	LastUsedIdx int
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
		go func() {
			defer wg.Done()
			serve(p, s.Port)
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
		return nil, NoService
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
			//make(chan int, 1),
			0,
		}
		//newInst.RRPos <- 0
		m.Instances = append(m.Instances, newInst)
	}
	m.Strategy = s.Strategy
	m.BlackList = make(map[int]bool, len(s.Instances))
	m.Mutex = &sync.RWMutex{}
	//m.LastUsedIdx = make(chan int, 1)
	//m.LastUsedIdx <- 0
	m.ReqCount = make(chan int, maxCon)
	m.WeightSum = weightSum
	m.HealthCheck = s.HealthCheck
	if m.HealthCheck.Interval == 0 {
		m.HealthCheck.Interval = defaultHealthCheckInterval
	}

	return m, nil
}

func serve(p *Pylon, port int) {
	mux := http.NewServeMux()
	mux.Handle("/", NewPylonHandler(p))
	fmt.Println("Serving on " + strconv.Itoa(port))
	server := &http.Server{
		Addr:           ":" + strconv.Itoa(port),
		Handler:        mux,
		ReadTimeout:    20 * time.Second,
		WriteTimeout:   20 * time.Second,
		MaxHeaderBytes: 1 << 20,
	}

	for _, s := range p.Services {
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

func handleHealthCheck(m *MicroService) {
	for i, inst := range m.Instances {
		_, err := dialer.Dial("tcp", inst.Host)
		if err != nil {
			m.blackList(i, true)
		} else {
			m.blackList(i, false)
		}
	}
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
		inst, err := m.getLoadBalancedInst()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		//m.notifyReq(true)
		m.ReqCount <- 1
		inst.ReqCount <- 1
		fmt.Println("Serving new request, count: " + strconv.Itoa(len(m.ReqCount)))
		proxy := proxyPool.Get()
		proxy.Director = func(req *http.Request) {
			req.URL.Scheme = "http"
			req.URL.Host = inst.Host
		}
		proxy.ServeHTTP(w, r)
		proxyPool.Put(proxy)

		//r.Body.Close()
		//m.notifyReq(false)
		<-inst.ReqCount
		<-m.ReqCount
		fmt.Println("Request served, count: " + strconv.Itoa(len(m.ReqCount)))
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
		default: return "", errors.New("Route has no correct type")

		}
	}
	return "no_route", errors.New("No route available for path " + path)
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

func (m *MicroService) getLoadBalancedInst() (*Instance, error) {
	if m == nil {
		return nil, errors.New("No service")
	}

	for {
		inst, _, err := m.getInstance()
		if err != nil {
			return nil, err
		}

		//fmt.Println("Dialing instance " + inst.Host + "...")
		//_, err = dialer.Dial("tcp", inst.Host)
		//if err != nil {
		//	m.blackList(idx)
		//	fmt.Println("Could not contact instance " + inst.Host)
		//	continue
		//}

		fmt.Println("Instance is " + inst.Host)
		return inst, nil
	}
	return nil, fmt.Errorf("No endpoint available for %+v\n", m)
}

func (m *MicroService) getInstance() (*Instance, int, error) {
	instCount := len(m.Instances)
	if instCount == 0 {
		return nil, -1, errors.New("Microservice has no instances")
	}

	if len(m.BlackList) == instCount {
		return nil, -1, errors.New("All instances are dead")
	}

	var idx int
	switch m.Strategy {
	case RoundRobin:
		idx = m.nextRoundRobinInstIdx()
	case LeastConnected:
		idx = m.getLeastConInstIdx()
	case Random:
		idx = m.getRandomInstIdx()
	default:
		return nil, -1, errors.New("Unexpected strategy " + string(m.Strategy))
	}

	if m.isBlacklisted(idx) {
		return m.getInstance()
	} else {
		return m.Instances[idx], idx, nil
	}
}

func (m *MicroService) nextRoundRobinInstIdx() int {
	//m.Mutex.Lock()
	for !m.Instances[m.LastUsedIdx].isRoundRobinPicked() {
		m.LastUsedIdx++
		if m.LastUsedIdx >= len(m.Instances) {
			m.LastUsedIdx = 0
		}
	}
	//m.Mutex.Unlock()
	return m.LastUsedIdx
}

func (m *MicroService) getLeastConInstIdx() int {
	minLoad := maxFloat32
	idx := 0

	for i, inst := range m.Instances {
		load := float32(len(inst.ReqCount)) / inst.Weight
		if load < minLoad {
			minLoad = load
			idx = i
		}
	}

	return idx
}

func (m *MicroService) getRandomInstIdx() int {
	r := rand.Float32() * m.WeightSum

	for i, inst := range m.Instances {
		r -= inst.Weight
		if r < 0 {
			return i
		}
	}

	return 0
}

/*func (m *MicroService) nextInst() int {
	cur := <- m.LastUsedIdx
	cur++
	if cur >= len(m.Instances) {
		cur = 0
	}
	m.LastUsedIdx <- cur
	return cur
}*/

/*func (m *MicroService) notifyReq(in bool) {
	if in {
		m.ReqCount++
	} else {
		m.ReqCount--
	}
}*/

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

func (m *MicroService) isBlacklisted(idx int) bool {
	blackListed := false
	m.Mutex.RLock()
	blackListed = m.BlackList[idx]
	m.Mutex.RUnlock()

	return blackListed
}