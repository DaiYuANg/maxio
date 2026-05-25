package s3_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	maxios3 "github.com/lyonbrown4d/maxio/s3"
)

func TestNewConfiguresPathPrefix(t *testing.T) {
	t.Parallel()

	service := maxios3.New(nil, maxios3.WithPathPrefix("storage"))
	if got := service.PathPrefix(); got != "/storage" {
		t.Fatalf("path prefix = %q, want /storage", got)
	}

	request := httptest.NewRequestWithContext(
		context.Background(),
		http.MethodGet,
		"/storage/bucket/object.txt",
		http.NoBody,
	)
	if !service.Match(request) {
		t.Fatal("custom prefix request did not match")
	}

	endpoint := maxios3.NewEndpoint(service)
	if got := endpoint.EndpointSpec().Prefix; got != "/storage" {
		t.Fatalf("endpoint prefix = %q, want /storage", got)
	}
}
