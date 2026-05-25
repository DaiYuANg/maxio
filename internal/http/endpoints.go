package http

import (
	"github.com/arcgolabs/httpx"
	"github.com/lyonbrown4d/maxio/s3"
)

type endpointRegistry struct {
	endpoints []httpx.Endpoint
}

func newEndpointRegistry(s3Endpoint *s3.Endpoint) endpointRegistry {
	return endpointRegistry{
		endpoints: []httpx.Endpoint{s3Endpoint},
	}
}

func (r endpointRegistry) Register(server httpx.ServerRuntime) {
	for _, endpoint := range r.endpoints {
		server.RegisterOnly(endpoint)
	}
}
