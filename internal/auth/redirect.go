// Copyright (c) 2025 AUTHORS All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package auth

import (
	"net/http"
	"net/url"
	"strings"
)

func BuildRedirectURL(r *http.Request) string {
	scheme := forwardedProto(r)
	host := forwardedHost(r)
	uri := forwardedURI(r)
	if scheme == "" {
		if r.TLS != nil {
			scheme = "https"
		} else {
			scheme = "http"
		}
	}
	if host == "" {
		host = r.Host
	}
	if uri == "" {
		uri = "/"
	}
	return scheme + "://" + host + uri
}

func SafeRedirect(r *http.Request, raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "/"
	}
	if strings.HasPrefix(raw, "/") && !strings.HasPrefix(raw, "//") {
		return raw
	}
	u, err := url.Parse(raw)
	if err != nil {
		return "/"
	}
	if u.Scheme == "" || u.Host == "" {
		return "/"
	}
	targetHost := strings.ToLower(u.Hostname())
	reqHost := strings.ToLower(stripPort(r.Host))
	if targetHost == "" || reqHost == "" || targetHost != reqHost {
		return "/"
	}
	path := u.RequestURI()
	if path == "" {
		path = "/"
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	return path
}

func forwardedProto(r *http.Request) string {
	return strings.TrimSpace(r.Header.Get("X-Forwarded-Proto"))
}

func forwardedHost(r *http.Request) string {
	if value := strings.TrimSpace(r.Header.Get("X-Forwarded-Host")); value != "" {
		return value
	}
	return strings.TrimSpace(r.Header.Get("Host"))
}

func forwardedURI(r *http.Request) string {
	if value := strings.TrimSpace(r.Header.Get("X-Forwarded-Uri")); value != "" {
		return value
	}
	return strings.TrimSpace(r.URL.RequestURI())
}

func stripPort(host string) string {
	host = strings.TrimSpace(host)
	if host == "" {
		return ""
	}
	if idx := strings.LastIndex(host, ":"); idx > 0 {
		return host[:idx]
	}
	return host
}
