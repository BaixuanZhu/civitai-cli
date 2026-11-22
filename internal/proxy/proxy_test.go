package proxy

import (
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
)

func TestValidate_AcceptsSupportedSchemes(t *testing.T) {
	cases := []struct {
		name       string
		rawURL     string
		wantScheme string
		wantHost   string
	}{
		{"http", "http://127.0.0.1:7890", "http", "127.0.0.1:7890"},
		{"https", "https://proxy.example.com:8443", "https", "proxy.example.com:8443"},
		{"socks5", "socks5://127.0.0.1:1080", "socks5", "127.0.0.1:1080"},
		{"socks5h", "socks5h://127.0.0.1:1080", "socks5h", "127.0.0.1:1080"},
		{"userinfo preserved", "http://user:pass@127.0.0.1:7890", "http", "127.0.0.1:7890"},
		{"uppercase scheme lowercased by url.Parse", "HTTP://127.0.0.1:7890", "http", "127.0.0.1:7890"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			u, err := Validate(tc.rawURL)
			if err != nil {
				t.Fatalf("Validate(%q) unexpected error: %v", tc.rawURL, err)
			}
			if u.Scheme != tc.wantScheme || u.Host != tc.wantHost {
				t.Errorf("Validate(%q) = scheme %q host %q, want scheme %q host %q",
					tc.rawURL, u.Scheme, u.Host, tc.wantScheme, tc.wantHost)
			}
		})
	}
}

func TestValidate_RejectsBadInput(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want string // substring of the descriptive error
	}{
		{"empty", "", "is empty"},
		{"no scheme, bare host", "proxy.example.com", "missing a scheme"},
		{"no scheme, protocol-relative", "//127.0.0.1:7890", "missing a scheme"},
		{"whitespace only", "   ", "missing a scheme"},
		{"missing host", "http://", "missing a host"},
		{"missing host socks5", "socks5://", "missing a host"},
		{"unsupported scheme ftp", "ftp://127.0.0.1:21", "unsupported proxy scheme"},
		{"host-with-dots parsed as scheme", "proxy.example.com:7890", "unsupported proxy scheme"},
		{"unparseable URL", "http://exa mple:1", "parse proxy URL"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			u, err := Validate(tc.raw)
			if err == nil {
				t.Fatalf("Validate(%q) = nil error, want descriptive error", tc.raw)
			}
			if u != nil {
				t.Errorf("Validate(%q) = %v, want nil URL on error", tc.raw, u)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("Validate(%q) error = %q, want it to contain %q", tc.raw, err.Error(), tc.want)
			}
		})
	}
}

func TestResolve_Priority(t *testing.T) {
	cases := []struct {
		name   string
		flag   string
		config string
		want   string
	}{
		{"flag wins over config", "http://flag:1", "http://config:2", "http://flag:1"},
		{"config when flag empty", "", "http://config:2", "http://config:2"},
		{"empty when both unset (caller keeps env/direct)", "", "", ""},
		{"whitespace-only flag falls through to config", "  \t ", "http://config:2", "http://config:2"},
		{"whitespace-only flag with empty config", "  ", "", ""},
		{"non-blank flag returned verbatim (design section 2 pseudocode)", " http://flag:1 ", "http://config:2", " http://flag:1 "},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := Resolve(tc.flag, tc.config); got != tc.want {
				t.Errorf("Resolve(%q, %q) = %q, want %q", tc.flag, tc.config, got, tc.want)
			}
		})
	}
}

// Resolve must not read the environment: with proxy env vars set the
// empty+empty result stays "", so env fallback keeps happening in
// http.DefaultTransport instead of here (design D2).
func TestResolve_IgnoresEnvironment(t *testing.T) {
	t.Setenv("HTTP_PROXY", "http://env:1")
	t.Setenv("HTTPS_PROXY", "http://env:1")
	t.Setenv("http_proxy", "http://env:1")
	t.Setenv("https_proxy", "http://env:1")
	if got := Resolve("", ""); got != "" {
		t.Errorf("Resolve(\"\", \"\") = %q with proxy env vars set, want \"\" (env applies via DefaultTransport)", got)
	}
}

func TestNewTransport_SetsFixedProxy(t *testing.T) {
	// Env proxies set on purpose: the fixed Proxy func must return the
	// configured URL for every request regardless (NO_PROXY deliberately
	// not consulted, design D8).
	t.Setenv("HTTP_PROXY", "http://env-would-win:1")
	tr, err := NewTransport("socks5://127.0.0.1:1080")
	if err != nil {
		t.Fatalf("NewTransport: %v", err)
	}
	if tr == nil || tr.Proxy == nil {
		t.Fatalf("NewTransport returned %v with nil Proxy func, want a transport with a fixed Proxy", tr)
	}
	req := httptest.NewRequest(http.MethodGet, "http://civitai.com/api/v1/models", nil)
	for i := 0; i < 2; i++ { // same URL for every call and every request
		got, err := tr.Proxy(req)
		if err != nil {
			t.Fatalf("Proxy call %d: %v", i, err)
		}
		if got.Scheme != "socks5" || got.Host != "127.0.0.1:1080" {
			t.Errorf("Proxy call %d = %s://%s, want socks5://127.0.0.1:1080", i, got.Scheme, got.Host)
		}
	}
}

// NewTransport must clone DefaultTransport (keeping dial/TLS/HTTP2 tuning)
// without mutating the shared global: every civ client built on it stays
// independent (design section 2).
func TestNewTransport_ClonesDefaultTransport(t *testing.T) {
	def := http.DefaultTransport.(*http.Transport)
	proxyFuncBefore := reflect.ValueOf(def.Proxy).Pointer()
	tr, err := NewTransport("http://127.0.0.1:7890")
	if err != nil {
		t.Fatalf("NewTransport: %v", err)
	}
	if !tr.ForceAttemptHTTP2 {
		t.Errorf("clone lost ForceAttemptHTTP2 (DefaultTransport has %v)", def.ForceAttemptHTTP2)
	}
	if tr.TLSHandshakeTimeout != def.TLSHandshakeTimeout {
		t.Errorf("clone TLSHandshakeTimeout = %v, want DefaultTransport's %v", tr.TLSHandshakeTimeout, def.TLSHandshakeTimeout)
	}
	if got := reflect.ValueOf(def.Proxy).Pointer(); got != proxyFuncBefore {
		t.Errorf("NewTransport mutated http.DefaultTransport.Proxy (func pointer changed)")
	}
	if got := reflect.ValueOf(tr.Proxy).Pointer(); got == proxyFuncBefore {
		t.Errorf("returned transport still uses the default ProxyFromEnvironment func, want the fixed proxy func")
	}
}

func TestNewTransport_InvalidInputErrors(t *testing.T) {
	for _, raw := range []string{"", "proxy.example.com", "ftp://127.0.0.1:21", "http://"} {
		tr, err := NewTransport(raw)
		if err == nil {
			t.Errorf("NewTransport(%q) error = nil, want descriptive error", raw)
		}
		if tr != nil {
			t.Errorf("NewTransport(%q) = %v, want nil transport on error", raw, tr)
		}
	}
}
