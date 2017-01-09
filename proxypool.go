package pylon

import (
	"net/http/httputil"
	"net/http"
	"time"
)

type Pool interface {
	Get() interface{}
	Put() interface{}
}

type ProxyPool struct {
	pool chan *httputil.ReverseProxy
}

func NewProxyPool(capacity int) *ProxyPool {
	return &ProxyPool{
		make(chan *httputil.ReverseProxy, capacity),
	}
}

func (p *ProxyPool) Get() *httputil.ReverseProxy {
	var proxy *httputil.ReverseProxy
	select {
	case proxy = <-p.pool:
	default:
		proxy = NewProxy()
	}
	return proxy
}

func (p *ProxyPool) Put(rp *httputil.ReverseProxy) {
	select {
	case p.pool <- rp:
	default:
	}
}

func NewProxy() *httputil.ReverseProxy {
	return &httputil.ReverseProxy{
		Transport: &http.Transport{
			Proxy: http.ProxyFromEnvironment,
		},
		FlushInterval: 1 * time.Second,
	}

}

