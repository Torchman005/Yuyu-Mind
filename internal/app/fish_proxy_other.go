//go:build !windows

package app

import (
	"net/http"
	"net/url"
)

func fishAudioProxy(req *http.Request) (*url.URL, error) {
	return http.ProxyFromEnvironment(req)
}
