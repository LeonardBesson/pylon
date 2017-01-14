package pylon

type Strategy string

const (
	RoundRobin Strategy = "round_robin"
	LeastConnected Strategy = "least_connected"
	Random Strategy = "random"
)

type Config struct {
	Servers []Server `json:"servers"`
}

type Server struct {
	Name        string    `json:"name"`
	Port        int       `json:"port"`
	HealthRoute string    `json:"health_route"`
	Services    []Service `json:"services"`
}

type Service struct {
	Pattern     string      `json:"route_pattern"`
	Prefix      string      `json:"route_prefix"`
	Instances   []Instance  `json:"instances"`
	Strategy    Strategy    `json:"balancing_strategy"`
	MaxCon      int         `json:"max_connections"`
	HealthCheck HealthCheck `json:"health_check"`
}

type HealthCheck struct {
	Enabled  bool `json:"enabled"`
	Interval int  `json:"interval"`
}

type Instance struct {
	Host     string    `json:"host"`
	Weight   float32   `json:"weight"`
	ReqCount chan int
	RRPos    SharedInt
}

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