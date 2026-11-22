package server

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestAccessLogRecordsResponsesWithoutQueryStrings(t *testing.T) {
	var output bytes.Buffer
	handler := AccessLog(&output, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/missing" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		fmt.Fprint(w, "ok")
	}))
	for _, path := range []string{"/api/status?key=secret", "/missing"} {
		handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, path, nil))
	}
	lines := strings.Split(strings.TrimSpace(output.String()), "\n")
	if len(lines) != 2 || !strings.Contains(lines[0], `method="GET" path="/api/status" status=200 bytes=2`) ||
		!strings.Contains(lines[1], `method="GET" path="/missing" status=404 bytes=0`) || strings.Contains(output.String(), "secret") {
		t.Fatalf("unexpected access log: %s", &output)
	}
}
