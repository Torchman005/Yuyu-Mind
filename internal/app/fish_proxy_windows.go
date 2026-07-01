//go:build windows

package app

import (
	"net/http"
	"net/url"
	"strings"

	"golang.org/x/sys/windows/registry"
)

func fishAudioProxy(req *http.Request) (*url.URL, error) {
	proxyURL, err := http.ProxyFromEnvironment(req)
	if proxyURL != nil || err != nil {
		return proxyURL, err
	}
	return windowsInternetProxy(req.URL.Scheme)
}

func windowsInternetProxy(scheme string) (*url.URL, error) {
	key, err := registry.OpenKey(registry.CURRENT_USER, `Software\Microsoft\Windows\CurrentVersion\Internet Settings`, registry.QUERY_VALUE)
	if err != nil {
		return nil, nil
	}
	defer key.Close()

	enabled, _, err := key.GetIntegerValue("ProxyEnable")
	if err != nil || enabled == 0 {
		return nil, nil
	}
	proxyServer, _, err := key.GetStringValue("ProxyServer")
	if err != nil {
		return nil, nil
	}

	raw := selectWindowsProxy(proxyServer, scheme)
	if raw == "" {
		return nil, nil
	}
	if !strings.Contains(raw, "://") {
		raw = "http://" + raw
	}
	proxyURL, err := url.Parse(raw)
	if err != nil {
		return nil, nil
	}
	return proxyURL, nil
}

func selectWindowsProxy(proxyServer, scheme string) string {
	proxyServer = strings.TrimSpace(proxyServer)
	if proxyServer == "" {
		return ""
	}
	if !strings.Contains(proxyServer, "=") {
		return proxyServer
	}

	for _, entry := range strings.Split(proxyServer, ";") {
		name, value, ok := strings.Cut(strings.TrimSpace(entry), "=")
		if ok && strings.EqualFold(strings.TrimSpace(name), scheme) {
			return strings.TrimSpace(value)
		}
	}
	for _, entry := range strings.Split(proxyServer, ";") {
		name, value, ok := strings.Cut(strings.TrimSpace(entry), "=")
		if ok && strings.EqualFold(strings.TrimSpace(name), "http") {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
