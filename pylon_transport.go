package pylon

import (
	"net/http"
)

type PylonTransport struct {
	microservice *MicroService
	// use the default transport underneath
	transport    *http.Transport
}

func NewPylonTransport() *PylonTransport {
	return &PylonTransport{
		transport: &http.Transport{
			Proxy: http.ProxyFromEnvironment,
		},
	}
}

func (pt *PylonTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	res, err := pt.transport.RoundTrip(req)
	if err != nil {
		logError(req.URL.Host + " is down")
		pt.microservice.blackListHost(req.URL.Host, true)
		inst, _, err := pt.microservice.getLoadBalancedInstance()
		if err != nil {
			return nil, err
		}
		req.URL.Host = inst.Host
		logError("Trying with new Instance:", inst.Host)
		return pt.RoundTrip(req)
	}

	return res, err
}

func (pt *PylonTransport) setMicroService(m *MicroService) {
	pt.microservice = m
}