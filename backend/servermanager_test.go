package backend

import (
	"crypto/tls"
	"net/http"
	"net/url"
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

func TestNewHTTPClient_ProxyOnly(t *testing.T) {
	testNewHTTPClient(t, newHTTPClientTestCase{
		timeout:     3 * time.Second,
		proxy:       "http://proxy.example.com:8080",
		wantProxy:   "http://proxy.example.com:8080",
		wantSkipSSL: false,
	})
}

func TestNewHTTPClient_ProxyAndSkipSSL(t *testing.T) {
	testNewHTTPClient(t, newHTTPClientTestCase{
		timeout:     3 * time.Second,
		proxy:       "http://proxy.example.com:8080",
		skipSSL:     true,
		wantProxy:   "http://proxy.example.com:8080",
		wantSkipSSL: true,
	})
}

func TestNewHTTPClient_SkipSSLOnly(t *testing.T) {
	testNewHTTPClient(t, newHTTPClientTestCase{
		timeout:     3 * time.Second,
		skipSSL:     true,
		wantSkipSSL: true,
	})
}

func TestNewHTTPClient_InvalidProxy(t *testing.T) {
	testNewHTTPClient(t, newHTTPClientTestCase{
		timeout:     3 * time.Second,
		proxy:       "http://%zz",
		wantSkipSSL: false,
	})
}

func TestNewHTTPClient_EmptyProxy(t *testing.T) {
	testNewHTTPClient(t, newHTTPClientTestCase{
		timeout:     3 * time.Second,
		proxy:       "",
		wantSkipSSL: false,
	})
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

	client := newHTTPClient(tc.timeout, tc.proxy, tc.skipSSL)

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

	if tc.wantProxy == "" {
		if transport.Proxy != nil {
			proxyURL, err := transport.Proxy(&http.Request{URL: requestURL})
			if err != nil {
				t.Fatal(err)
			}
			if proxyURL != nil {
				t.Fatalf("proxy URL = %q, want nil", proxyURL.String())
			}
		}
	} else {
		if transport.Proxy == nil {
			t.Fatal("transport Proxy = nil, want proxy function")
		}
		proxyURL, err := transport.Proxy(&http.Request{URL: requestURL})
		if err != nil {
			t.Fatal(err)
		}
		if proxyURL == nil {
			t.Fatal("proxy URL = nil, want configured proxy")
		}
		if got := proxyURL.String(); got != tc.wantProxy {
			t.Fatalf("proxy URL = %q, want %q", got, tc.wantProxy)
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

func TestApplyTransportSettingsIgnoresInvalidProxyAndPreservesTimeout(t *testing.T) {
	client := &http.Client{Timeout: 7 * time.Second}

	applyTransportSettings(client, "http://%zz", true)

	if client.Timeout != 7*time.Second {
		t.Fatalf("client timeout = %s, want %s", client.Timeout, 7*time.Second)
	}
	transport, ok := client.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("client transport = %T, want *http.Transport", client.Transport)
	}
	requestURL, err := url.Parse("http://music.example.com/rest/ping")
	if err != nil {
		t.Fatal(err)
	}
	if transport.Proxy != nil {
		proxyURL, err := transport.Proxy(&http.Request{URL: requestURL})
		if err != nil {
			t.Fatal(err)
		}
		if proxyURL != nil {
			t.Fatalf("proxy URL = %q for invalid proxy configuration, want nil", proxyURL.String())
		}
	}
	if transport.TLSClientConfig == nil || !transport.TLSClientConfig.InsecureSkipVerify {
		t.Fatal("transport TLSClientConfig.InsecureSkipVerify = false, want true")
	}
}

func TestApplyTransportSettingsDoesNotSetTLSConfigWhenSkipSSLVerifyDisabled(t *testing.T) {
	client := &http.Client{}
	client.Transport = &http.Transport{TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12}}

	applyTransportSettings(client, "", false)

	transport, ok := client.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("client transport = %T, want *http.Transport", client.Transport)
	}
	if transport.Proxy != nil {
		t.Fatal("transport Proxy is set without a proxy URL, want nil")
	}
	if transport.TLSClientConfig == nil {
		t.Fatal("transport TLSClientConfig was not preserved from original transport")
	}
	if transport.TLSClientConfig.InsecureSkipVerify {
		t.Fatal("transport TLSClientConfig.InsecureSkipVerify = true, want false")
	}
}

func TestApplyTransportSettingsClonesExistingTransport(t *testing.T) {
	existing := &http.Transport{
		MaxIdleConns:        42,
		MaxIdleConnsPerHost: 7,
	}
	client := &http.Client{Transport: existing}

	applyTransportSettings(client, "http://proxy.example.com:8080", true)

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
