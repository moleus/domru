package reverseproxy

import (
	"io"
	"net/http"
	"net/url"

	myhttp "github.com/moleus/domru/pkg/domru/http"
)

type ReverseProxy struct {
	Client myhttp.HTTPClient
	target *url.URL
}

func NewReverseProxy(target *url.URL) *ReverseProxy {
	return &ReverseProxy{target: target, Client: http.DefaultClient}
}

func (p *ReverseProxy) ProxyRequestHandler() func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, req *http.Request) {
		// Step 1: rewrite URL
		req.URL.Scheme = p.target.Scheme
		req.URL.Host = p.target.Host
		req.RequestURI = ""

		resp, err := p.Client.Do(req)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		defer resp.Body.Close()

		// Step 4: copy payload to response writer
		copyHeader(w.Header(), resp.Header)
		w.WriteHeader(resp.StatusCode)
		_, err = io.Copy(w, resp.Body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
		}
		resp.Body.Close()
	}
}

func copyHeader(dst, src http.Header) {
	for k, vv := range src {
		for _, v := range vv {
			dst.Add(k, v)
		}
	}
}
