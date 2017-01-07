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
}
