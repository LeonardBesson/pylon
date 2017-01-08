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
	Name     string    `json:"name"`
	Port     int       `json:"port"`
	Services []Service `json:"services"`
}

type Service struct {
	Pattern   string     `json:"route_pattern"`
	Prefix    string     `json:"route_prefix"`
	Instances []Instance `json:"instances"`
	Strategy  Strategy   `json:"balancing_strategy"`
	MaxCon    int        `json:"max_connections"`
}

type Instance struct {
	Host     string  `json:"host"`
	Weight   float32 `json:"weight"`
	ReqCount chan int
	//RRPos	 chan int
	RRPos	 int
}

func (i *Instance) isRoundRobinPicked() bool {
	//cur := <-i.RRPos
	cur := i.RRPos
	cur++
	if cur > int(i.Weight) {
		cur = 0
		//i.RRPos <- cur
		i.RRPos = cur
		return false
	}

	//i.RRPos <- cur
	i.RRPos = cur
	return true
}
