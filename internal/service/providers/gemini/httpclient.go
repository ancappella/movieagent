package gemini

import (
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

// DefaultClashMixedProxy is Clash Verge's default mixed-port on this machine.
const DefaultClashMixedProxy = "http://127.0.0.1:7897"

// NewHTTPClient builds an HTTP client that can reach Google APIs from China.
// Priority: cfg client → HTTPS_PROXY/HTTP_PROXY/ALL_PROXY → Clash 7897 if listening → direct.
func NewHTTPClient(timeout time.Duration) *http.Client {
	if timeout <= 0 {
		timeout = 120 * time.Second
	}
	transport := &http.Transport{
		Proxy: proxyFunc(),
		DialContext: (&net.Dialer{
			Timeout:   20 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		TLSHandshakeTimeout:   20 * time.Second,
		ResponseHeaderTimeout: timeout,
		IdleConnTimeout:       90 * time.Second,
		ForceAttemptHTTP2:     true,
	}
	return &http.Client{Timeout: timeout, Transport: transport}
}

func proxyFunc() func(*http.Request) (*url.URL, error) {
	// Explicit override for Gemini / Clash.
	if raw := firstNonEmpty(
		os.Getenv("GEMINI_HTTP_PROXY"),
		os.Getenv("HTTPS_PROXY"),
		os.Getenv("https_proxy"),
		os.Getenv("HTTP_PROXY"),
		os.Getenv("http_proxy"),
		os.Getenv("ALL_PROXY"),
		os.Getenv("all_proxy"),
	); raw != "" {
		u, err := url.Parse(raw)
		if err == nil && u.Scheme != "" && u.Host != "" {
			return http.ProxyURL(u)
		}
	}
	// Auto-use Clash Verge mixed-port when available (system apps may still go direct).
	if clashListening("127.0.0.1:7897") {
		u, _ := url.Parse(DefaultClashMixedProxy)
		return http.ProxyURL(u)
	}
	return http.ProxyFromEnvironment
}

func clashListening(addr string) bool {
	c, err := net.DialTimeout("tcp", addr, 300*time.Millisecond)
	if err != nil {
		return false
	}
	_ = c.Close()
	return true
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}
