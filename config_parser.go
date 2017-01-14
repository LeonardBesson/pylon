package pylon

import (
	"encoding/json"
	"io/ioutil"
	"regexp"
	"io"
	"os"
	"strings"
)

var (
	hostRegexp *regexp.Regexp = regexp.MustCompile("^(([0-9]|[1-9][0-9]|1[0-9]{2}|2[0-4][0-9]|25[0-5])\\.){3}([0-9]|[1-9][0-9]|1[0-9]{2}|2[0-4][0-9]|25[0-5]):[0-9]{1,5}$")
	//hostNameRegexp *regexp.Regexp = regexp.MustCompile("^(([a-zA-Z0-9]|[a-zA-Z0-9][a-zA-Z0-9\\-]*[a-zA-Z0-9])\\.)*([A-Za-z0-9]|[A-Za-z0-9][A-Za-z0-9\\-]*[A-Za-z0-9])$")
)

type ConfigParser interface {
	Parse(r io.Reader) (*Config, error)
}

type JSONConfigParser struct {
}

func (p *JSONConfigParser) ParseFromPath(path string) (c *Config, err error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	return p.Parse(f)
}

func (p *JSONConfigParser) Parse(r io.Reader) (c *Config, err error) {
	bytes, err := ioutil.ReadAll(r)
	if err != nil {
		return nil, err
	}

	if err = json.Unmarshal(bytes, &c); err != nil {
		return nil, err
	}

	if err = validateConfig(c); err != nil {
		return nil, err
	}

	return c, nil
}

func validateConfig(c *Config) (error) {
	for i, s := range c.Servers {
		if err := validateServer(&s); err != nil {
			return err
		}
		if s.HealthRoute != "" && !strings.HasPrefix(s.HealthRoute, "/") {
			c.Servers[i].HealthRoute = "/" + s.HealthRoute
		}
	}
	return nil
}

func validateServer(s *Server) (error) {
	for _, ser := range s.Services {
		if err := validateService(&ser); err != nil {
			return err
		}
	}
	return nil
}

func validateService(s *Service) (error) {
	if _, err := regexp.Compile(s.Pattern); err != nil {
		return NewError(ErrInvalidRouteRegexCode, "Route: " + s.Pattern + " is not a correct regular expression")
	}

	if len(s.Instances) <= 0 {
		return ErrServiceNoInstance
	}

	if !isStrategyValid(s.Strategy) {
		return NewError(ErrInvalidStrategyCode, "Strategy: " + string(s.Strategy) + " is not valid")
	}

	for _, inst := range s.Instances {
		if err := validateInstance(&inst); err != nil {
			return err
		}
	}
	return nil
}

func validateInstance(inst *Instance) (error) {
	if !hostRegexp.MatchString(inst.Host) /*&& !hostNameRegexp.MatchString(inst.IP)*/ {
		return NewError(ErrInvalidHostCode, "Host: " + inst.Host + " is not correct")
	}
	return nil
}

func isStrategyValid(s Strategy) bool {
	switch s {
	case RoundRobin: fallthrough
	case LeastConnected: fallthrough
	case Random:
		return true
	default:
		return false
	}
}