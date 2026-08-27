package backend

import (
	"crypto/tls"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"testing"
	"time"
)

func TestNormalizeServerURL(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"", ""},
		{"http://192.168.1.1:8096", "http://192.168.1.1:8096"},
		{"https://music.example.com", "https://music.example.com"},
		{"192.168.1.1:4533", "http://192.168.1.1:4533"},
		{"music.example.com", "http://music.example.com"},
		{"http://192.168.1.1:8096/", "http://192.168.1.1:8096"},
		{"http://192.168.1.1:8096///", "http://192.168.1.1:8096"},
		{"192.168.1.1:8096/", "http://192.168.1.1:8096"},
	}
	for _, tt := range tests {
		got := NormalizeServerURL(tt.input)
		if got != tt.want {
			t.Errorf("NormalizeServerURL(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestNormalizeJellyfinURL(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"", ""},
		{"http://192.168.1.1:8096", "http://192.168.1.1:8096"},
		{"192.168.1.1:8096", "http://192.168.1.1:8096"},
		{"192.168.1.1:8096/", "http://192.168.1.1:8096"},
		{"192.168.1.1:8096/web/index.html", "http://192.168.1.1:8096"},
		{"http://192.168.1.1:8096/web/index.html", "http://192.168.1.1:8096"},
		{"http://192.168.1.1:8096/web/", "http://192.168.1.1:8096"},
		{"http://192.168.1.1:8096/web", "http://192.168.1.1:8096"},
		{"https://jellyfin.example.com/web/index.html", "https://jellyfin.example.com"},
		{"https://jellyfin.example.com/web/", "https://jellyfin.example.com"},
	}
	for _, tt := range tests {
		got := NormalizeJellyfinURL(tt.input)
		if got != tt.want {
			t.Errorf("NormalizeJellyfinURL(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestAppHTTPClientFactoryConfiguredProxy(t *testing.T) {
	testNewHTTPClient(t, newHTTPClientTestCase{
		timeout:     3 * time.Second,
		proxy:       "http://proxy.example.com:8080",
		wantProxy:   "http://proxy.example.com:8080",
		wantSkipSSL: false,
	})
}

func TestAppHTTPClientFactoryAddsMissingProxyScheme(t *testing.T) {
	testNewHTTPClient(t, newHTTPClientTestCase{
		timeout:     3 * time.Second,
		proxy:       "proxy.example.com:8080",
		wantProxy:   "http://proxy.example.com:8080",
		wantSkipSSL: false,
	})
}

func TestAppHTTPClientFactoryConfiguredProxyAndSkipSSL(t *testing.T) {
	testNewHTTPClient(t, newHTTPClientTestCase{
		timeout:     3 * time.Second,
		proxy:       "http://proxy.example.com:8080",
		skipSSL:     true,
		wantProxy:   "http://proxy.example.com:8080",
		wantSkipSSL: true,
	})
}

func TestAppHTTPClientFactorySkipSSLOnly(t *testing.T) {
	testNewHTTPClient(t, newHTTPClientTestCase{
		timeout:     3 * time.Second,
		skipSSL:     true,
		wantSkipSSL: true,
	})
}

func TestAppHTTPClientFactoryInvalidProxy(t *testing.T) {
	testNewHTTPClient(t, newHTTPClientTestCase{
		timeout:     3 * time.Second,
		proxy:       "http://%zz",
		wantSkipSSL: false,
	})
}

func TestAppHTTPClientFactoryUsesEnvironmentProxyWithoutConfiguration(t *testing.T) {
	testNewHTTPClient(t, newHTTPClientTestCase{
		timeout:     3 * time.Second,
		wantSkipSSL: false,
	})
}

func TestAppHTTPClientFactoryConfiguredProxyHonorsNoProxy(t *testing.T) {
	t.Setenv("NO_PROXY", "music.example.com")
	client := newAppHTTPClientFactory("http://proxy.example.com:8080").NewClient(time.Second, false)
	transport := client.Transport.(*http.Transport)
	requestURL, err := url.Parse("http://music.example.com/rest/ping")
	if err != nil {
		t.Fatal(err)
	}
	proxyURL, err := transport.Proxy(&http.Request{URL: requestURL})
	if err != nil {
		t.Fatal(err)
	}
	if proxyURL != nil {
		t.Fatalf("proxy URL = %q, want direct connection from NO_PROXY", proxyURL)
	}
}

func TestAppHTTPClientFactoryConfiguredProxyBypassesLoopback(t *testing.T) {
	client := newAppHTTPClientFactory("http://proxy.example.com:8080").NewClient(time.Second, false)
	transport := client.Transport.(*http.Transport)
	requestURL, err := url.Parse("http://127.0.0.1:4533/rest/ping")
	if err != nil {
		t.Fatal(err)
	}
	proxyURL, err := transport.Proxy(&http.Request{URL: requestURL})
	if err != nil {
		t.Fatal(err)
	}
	if proxyURL != nil {
		t.Fatalf("proxy URL = %q, want direct loopback connection", proxyURL)
	}
}

func TestAppHTTPClientFactoryMPVUsesOnlyConfiguredProxy(t *testing.T) {
	t.Setenv("HTTPS_PROXY", "http://environment.example.com:8080")
	if got := newAppHTTPClientFactory("").proxyForMPV(); got != "" {
		t.Fatalf("proxyForMPV() = %q without configuration, want empty", got)
	}
	if got := newAppHTTPClientFactory("proxy.example.com:8080").proxyForMPV(); got != "http://proxy.example.com:8080" {
		t.Fatalf("proxyForMPV() = %q, want normalized configured proxy", got)
	}
}

type newHTTPClientTestCase struct {
	timeout     time.Duration
	proxy       string
	skipSSL     bool
	wantProxy   string
	wantSkipSSL bool
}

func testNewHTTPClient(t *testing.T, tc newHTTPClientTestCase) {
	t.Helper()

	client := newAppHTTPClientFactory(tc.proxy).NewClient(tc.timeout, tc.skipSSL)

	if client.Timeout != tc.timeout {
		t.Fatalf("client timeout = %s, want %s", client.Timeout, tc.timeout)
	}

	transport, ok := client.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("client transport = %T, want *http.Transport", client.Transport)
	}

	requestURL, err := url.Parse("http://music.example.com/rest/ping")
	if err != nil {
		t.Fatal(err)
	}
	req := &http.Request{URL: requestURL}

	if tc.wantProxy != "" {
		if transport.Proxy == nil {
			t.Fatal("transport Proxy = nil, want configured proxy function")
		}
		proxyURL, err := transport.Proxy(req)
		if err != nil {
			t.Fatal(err)
		}
		if proxyURL == nil {
			t.Fatal("proxy URL = nil, want configured proxy")
		}
		if got := proxyURL.String(); got != tc.wantProxy {
			t.Fatalf("proxy URL = %q, want %q", got, tc.wantProxy)
		}
	} else if tc.proxy != "" {
		if transport.Proxy != nil {
			t.Fatal("transport Proxy is set for an invalid proxy configuration")
		}
	} else {
		defaultTransport := http.DefaultTransport.(*http.Transport)
		if transport.Proxy == nil || defaultTransport.Proxy == nil {
			t.Fatal("transport Proxy = nil, want default environment proxy semantics")
		}
		gotProxy, gotErr := transport.Proxy(req)
		wantProxy, wantErr := defaultTransport.Proxy(req)
		if (gotErr == nil) != (wantErr == nil) || (gotErr != nil && gotErr.Error() != wantErr.Error()) {
			t.Fatalf("environment proxy error = %v, want %v", gotErr, wantErr)
		}
		if (gotProxy == nil) != (wantProxy == nil) {
			t.Fatalf("environment proxy = %v, want %v", gotProxy, wantProxy)
		}
		if gotProxy != nil && gotProxy.String() != wantProxy.String() {
			t.Fatalf("environment proxy = %q, want %q", gotProxy, wantProxy)
		}
	}

	if transport.TLSClientConfig == nil {
		if tc.wantSkipSSL {
			t.Fatal("transport TLSClientConfig = nil, want InsecureSkipVerify enabled")
		}
		return
	}
	if transport.TLSClientConfig.InsecureSkipVerify != tc.wantSkipSSL {
		t.Fatalf("transport TLSClientConfig.InsecureSkipVerify = %t, want %t", transport.TLSClientConfig.InsecureSkipVerify, tc.wantSkipSSL)
	}
}

func TestAppHTTPClientFactoryIgnoresInvalidProxyAndPreservesTimeout(t *testing.T) {
	client := newAppHTTPClientFactory("http://%zz").NewClient(7*time.Second, true)

	if client.Timeout != 7*time.Second {
		t.Fatalf("client timeout = %s, want %s", client.Timeout, 7*time.Second)
	}
	transport, ok := client.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("client transport = %T, want *http.Transport", client.Transport)
	}
	if transport.Proxy != nil {
		t.Fatal("transport Proxy is set for an invalid proxy configuration")
	}
	if transport.TLSClientConfig == nil || !transport.TLSClientConfig.InsecureSkipVerify {
		t.Fatal("transport TLSClientConfig.InsecureSkipVerify = false, want true")
	}
}

func TestAppHTTPClientFactoryPreservesExistingTransportWithoutProxyConfiguration(t *testing.T) {
	existing := &http.Transport{TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12}}
	client := &http.Client{Transport: existing}

	newAppHTTPClientFactory("").configureHTTPClient(client, false)

	transport, ok := client.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("client transport = %T, want *http.Transport", client.Transport)
	}
	if transport == existing {
		t.Fatal("transport was reused, want cloned transport")
	}
	if transport.Proxy != nil {
		t.Fatal("transport Proxy is set without a configured proxy")
	}
	if transport.TLSClientConfig == nil || transport.TLSClientConfig.MinVersion != tls.VersionTLS12 {
		t.Fatal("transport TLSClientConfig was not preserved from original transport")
	}
	if transport.TLSClientConfig.InsecureSkipVerify {
		t.Fatal("transport TLSClientConfig.InsecureSkipVerify = true, want false")
	}
}

func TestAppHTTPClientFactoryClonesExistingTransport(t *testing.T) {
	existing := &http.Transport{
		MaxIdleConns:        42,
		MaxIdleConnsPerHost: 7,
	}
	client := &http.Client{Transport: existing}

	newAppHTTPClientFactory("http://proxy.example.com:8080").configureHTTPClient(client, true)

	transport, ok := client.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("client transport = %T, want *http.Transport", client.Transport)
	}
	if transport == existing {
		t.Fatal("transport was reused, want cloned transport")
	}
	if transport.MaxIdleConns != existing.MaxIdleConns {
		t.Fatalf("MaxIdleConns = %d, want %d", transport.MaxIdleConns, existing.MaxIdleConns)
	}
	if transport.MaxIdleConnsPerHost != existing.MaxIdleConnsPerHost {
		t.Fatalf("MaxIdleConnsPerHost = %d, want %d", transport.MaxIdleConnsPerHost, existing.MaxIdleConnsPerHost)
	}
	if transport.TLSClientConfig == nil || !transport.TLSClientConfig.InsecureSkipVerify {
		t.Fatal("transport TLSClientConfig.InsecureSkipVerify = false, want true")
	}
}

func TestAppHTTPClientFactoryRoutesThroughConfiguredProxy(t *testing.T) {
	var requestedURL string
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestedURL = r.URL.String()
		_, _ = io.WriteString(w, "proxied")
	}))
	defer proxy.Close()

	client := newAppHTTPClientFactory(proxy.URL).NewClient(time.Second, false)
	resp, err := client.Get("http://music.example.test/stream")
	if err != nil {
		t.Fatalf("request through configured proxy: %v", err)
	}
	defer resp.Body.Close()
	if _, err := io.ReadAll(resp.Body); err != nil {
		t.Fatalf("reading proxy response: %v", err)
	}
	if requestedURL != "http://music.example.test/stream" {
		t.Fatalf("proxy request URL = %q, want music server URL", requestedURL)
	}
}

func TestAppHTTPClientFactoryDoesNotMutateDefaultTransport(t *testing.T) {
	defaultTransport := http.DefaultTransport.(*http.Transport)
	originalProxy := defaultTransport.Proxy
	originalTLSConfig := defaultTransport.TLSClientConfig

	client := newAppHTTPClientFactory("http://proxy.example.com:8080").NewClient(time.Second, true)
	if client.Transport == defaultTransport {
		t.Fatal("client reused http.DefaultTransport")
	}
	if originalProxy != nil && reflect.ValueOf(defaultTransport.Proxy).Pointer() != reflect.ValueOf(originalProxy).Pointer() {
		t.Fatal("http.DefaultTransport Proxy was modified")
	}
	if defaultTransport.TLSClientConfig != originalTLSConfig {
		t.Fatal("http.DefaultTransport TLSClientConfig was modified")
	}
}
