package server

import (
	"errors"
	"mime"
	"net"
	"net/http"
	"net/url"
	"strings"

	"harness.engineering/internal/local"
)

type TailscaleArchiveAccess struct {
	host  string
	login string
}

func NewTailscaleArchiveAccess(host, login, listen string) (TailscaleArchiveAccess, error) {
	if host == "" && login == "" {
		return TailscaleArchiveAccess{}, nil
	}
	if host == "" || strings.TrimSpace(login) == "" {
		return TailscaleArchiveAccess{}, errors.New("Tailscale project management requires both -tailscale-archive-host and -tailscale-archive-user")
	}
	endpoint, err := url.Parse("https://" + host)
	if err != nil || endpoint.Host != host || !strings.HasSuffix(strings.ToLower(endpoint.Hostname()), ".ts.net") {
		return TailscaleArchiveAccess{}, errors.New("Tailscale archive host must be a .ts.net hostname with an optional port")
	}
	listenHost, _, err := net.SplitHostPort(listen)
	if err != nil || !local.IsLoopback(listenHost) {
		return TailscaleArchiveAccess{}, errors.New("Tailscale project management requires a loopback -addr")
	}
	return TailscaleArchiveAccess{host: strings.ToLower(host), login: strings.TrimSpace(login)}, nil
}

func (access TailscaleArchiveAccess) allows(r *http.Request) bool {
	if access.host == "" || !strings.EqualFold(r.Host, access.host) || !loopbackPeer(r) {
		return false
	}
	if !strings.EqualFold(r.Header.Get("X-Forwarded-Host"), access.host) ||
		r.Header.Get("X-Forwarded-Proto") != "https" || r.Header.Get("Tailscale-Funnel-Request") != "" {
		return false
	}
	logins := r.Header.Values("Tailscale-User-Login")
	if len(logins) != 1 {
		return false
	}
	login, err := new(mime.WordDecoder).DecodeHeader(logins[0])
	return err == nil && login == access.login
}

func loopbackPeer(r *http.Request) bool {
	peer, _, err := net.SplitHostPort(r.RemoteAddr)
	return err == nil && local.IsLoopback(peer)
}

func (app server) canManageProjects(r *http.Request) bool {
	return localRequest(r) || app.archiveAccess.allows(r)
}

func requireAccess(allowed func(*http.Request) bool, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !allowed(r) {
			http.Error(w, "this action requires an authorized connection", http.StatusForbidden)
			return
		}
		next(w, r)
	}
}
