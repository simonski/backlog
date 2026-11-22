package server

import (
	"io"
	"log"
	"net/http"
	"time"
)

// AccessLog writes one line per HTTP request without query strings, bodies, or headers.
func AccessLog(out io.Writer, next http.Handler) http.Handler {
	logger := log.New(out, "", log.Ldate|log.Ltime|log.LUTC)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		started := time.Now()
		response := &accessResponse{ResponseWriter: w}
		defer func() {
			logger.Printf("peer=%q method=%q path=%q status=%d bytes=%d duration=%s",
				r.RemoteAddr, r.Method, r.URL.EscapedPath(), response.status, response.bytes, time.Since(started).Round(time.Microsecond))
		}()
		next.ServeHTTP(response, r)
		if response.status == 0 {
			response.status = http.StatusOK
		}
	})
}

type accessResponse struct {
	http.ResponseWriter
	status int
	bytes  int64
}

func (response *accessResponse) WriteHeader(status int) {
	response.ResponseWriter.WriteHeader(status)
	if response.status == 0 && (status >= 200 || status == http.StatusSwitchingProtocols) {
		response.status = status
	}
}

func (response *accessResponse) Write(data []byte) (int, error) {
	if response.status == 0 {
		response.status = http.StatusOK
	}
	n, err := response.ResponseWriter.Write(data)
	response.bytes += int64(n)
	return n, err
}

func (response *accessResponse) Unwrap() http.ResponseWriter {
	return response.ResponseWriter
}
