package handler_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
)

func serveRouterRequest(router http.Handler, method, target string, body []byte) *httptest.ResponseRecorder {
	recorder := httptest.NewRecorder()
	request := httptest.NewRequestWithContext(context.Background(), method, target, strings.NewReader(string(body)))
	router.ServeHTTP(recorder, request)
	return recorder
}
