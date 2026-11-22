// Package proxy is the single source of truth for civ's outbound proxy
// behavior: proxy URL validation, flag-over-config precedence resolution,
// and proxy transport construction. It is a leaf package (standard library
// only) so the api, cmd, config, and upgrade packages can all reuse it
// without import cycles (design D3). When Resolve returns "", callers keep
// a nil Transport and http.DefaultTransport applies
// HTTP_PROXY/HTTPS_PROXY/NO_PROXY exactly as before the feature existed
// (design D2).
package proxy

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

// Validate parses and checks a proxy URL. Accepted schemes are http, https,
// socks5, and socks5h — net/http's Transport dials SOCKS proxies natively
// (Go >= 1.9), so no extra dependency is needed (design D6) — and the host
// must be non-empty. Empty input, a missing scheme, a missing host, and any
// other scheme are rejected with a descriptive error. Validation happens at
// the system boundary (civ proxy set), so a bad value never reaches a live
// dial. It returns the parsed URL on success.
func Validate(rawURL string) (*url.URL, error) {
	if rawURL == "" {
		return nil, fmt.Errorf("proxy: proxy URL is empty (expected http://, https://, socks5://, or socks5h://)")
	}
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, fmt.Errorf("proxy: parse proxy URL %q: %w", rawURL, err)
	}
	if u.Scheme == "" {
		return nil, fmt.Errorf("proxy: proxy URL %q is missing a scheme (expected http://, https://, socks5://, or socks5h://)", rawURL)
	}
	if !supportedScheme(u.Scheme) {
		return nil, fmt.Errorf("proxy: unsupported proxy scheme %q (expected http, https, socks5, or socks5h)", u.Scheme)
	}
	if u.Host == "" {
		return nil, fmt.Errorf("proxy: proxy URL %q is missing a host (expected host:port)", rawURL)
	}
	return u, nil
}

// supportedScheme reports whether scheme is one of the proxy schemes civ
// accepts. url.Parse lowercases the scheme, so no case folding is needed.
func supportedScheme(scheme string) bool {
	switch scheme {
	case "http", "https", "socks5", "socks5h":
		return true
	default:
		return false
	}
}

// Resolve implements the explicit-proxy precedence chain: a flagProxy that
// is non-empty after trimming wins, otherwise the persisted configProxy is
// returned as-is (possibly ""). It is pure and never reads the environment:
// an empty result tells the caller to keep a nil Transport so
// http.DefaultTransport (ProxyFromEnvironment) applies env proxies and
// NO_PROXY as before (design D2).
func Resolve(flagProxy, configProxy string) string {
	if strings.TrimSpace(flagProxy) != "" {
		return flagProxy
	}
	return configProxy
}

// NewTransport returns a Transport that routes every request through the
// proxy at rawURL. The base is a clone of http.DefaultTransport, so dial
// keepalive, TLS handshake timeout, ForceAttemptHTTP2, and the standard
// timeouts carry over; the global DefaultTransport itself is never mutated.
// The Proxy function returns the validated URL for every request, and
// NO_PROXY is deliberately not consulted: an explicit flag/config proxy
// applies to all civ outbound traffic (civitai.com and github.com), and
// honoring NO_PROXY would require golang.org/x/net/http/httpproxy, breaking
// the zero-dependency contract (design D8).
func NewTransport(rawURL string) (*http.Transport, error) {
	u, err := Validate(rawURL)
	if err != nil {
		return nil, err
	}
	tr := http.DefaultTransport.(*http.Transport).Clone()
	tr.Proxy = func(*http.Request) (*url.URL, error) { return u, nil }
	return tr, nil
}
