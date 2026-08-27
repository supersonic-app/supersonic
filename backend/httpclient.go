package backend

import (
	"crypto/tls"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"golang.org/x/net/http/httpproxy"
)

// appHTTPClientFactory creates outbound HTTP clients with the application
// proxy setting applied. Configured proxies use httpproxy semantics so
// NO_PROXY and loopback exemptions remain in effect; without an application
// proxy, the default transport's environment behavior is preserved.
type appHTTPClientFactory struct {
	configuredProxy     *url.URL
	configuredProxyFunc func(*url.URL) (*url.URL, error)
	hasConfiguredProxy  bool
	configuredProxyErr  error
	mpvProxyErr         error
}

func newAppHTTPClientFactory(rawProxy string) *appHTTPClientFactory {
	factory := &appHTTPClientFactory{}
	if rawProxy == "" {
		return factory
	}
	proxy, err := parseHTTPProxyURL(rawProxy)
	if err != nil {
		log.Printf("Warning: invalid proxy configuration, ignoring configured proxy")
		factory.hasConfiguredProxy = true
		factory.configuredProxyErr = err
		return factory
	}
	factory.hasConfiguredProxy = true

	envProxy := httpproxy.FromEnvironment()
	proxyConfig := &httpproxy.Config{
		HTTPProxy:  proxy.String(),
		HTTPSProxy: proxy.String(),
		NoProxy:    envProxy.NoProxy,
	}
	factory.configuredProxy = proxy
	factory.configuredProxyFunc = proxyConfig.ProxyFunc()
	if proxy.Scheme == "https" {
		factory.mpvProxyErr = fmt.Errorf("configured proxy uses HTTPS, which MPV does not support")
	}
	log.Printf("Setting application HTTP proxy: %s", redactProxyURL(proxy.String()))
	return factory
}

func parseHTTPProxyURL(rawProxy string) (*url.URL, error) {
	rawProxy = strings.TrimSpace(rawProxy)
	if rawProxy == "" {
		return nil, fmt.Errorf("proxy URL is empty")
	}
	if !strings.Contains(rawProxy, "://") {
		// Match httpproxy's host[:port] form by assuming http:// when the
		// configured value omits a scheme.
		rawProxy = "http://" + rawProxy
	}
	proxy, err := url.Parse(rawProxy)
	if err != nil {
		return nil, fmt.Errorf("invalid proxy URL")
	}
	proxy.Scheme = strings.ToLower(proxy.Scheme)
	if proxy.Host == "" || proxy.Hostname() == "" || (proxy.Scheme != "http" && proxy.Scheme != "https") {
		return nil, fmt.Errorf("proxy URL must use http or https and include a host")
	}
	return proxy, nil
}

func (f *appHTTPClientFactory) NewClient(timeout time.Duration, skipSSLVerify bool) *http.Client {
	client := &http.Client{Timeout: timeout}
	f.configureHTTPClient(client, skipSSLVerify)
	return client
}

func (f *appHTTPClientFactory) configureHTTPClient(client *http.Client, skipSSLVerify bool) {
	var transport *http.Transport
	if existing, ok := client.Transport.(*http.Transport); ok {
		transport = existing.Clone()
	} else {
		transport = http.DefaultTransport.(*http.Transport).Clone()
	}

	switch {
	case f.configuredProxyFunc != nil:
		transport.Proxy = f.proxyForRequest
	case f.hasConfiguredProxy:
		// A configured but invalid proxy must not silently fall back to an
		// environment proxy.
		transport.Proxy = nil
	}

	if skipSSLVerify {
		if transport.TLSClientConfig == nil {
			transport.TLSClientConfig = &tls.Config{}
		} else {
			transport.TLSClientConfig = transport.TLSClientConfig.Clone()
		}
		transport.TLSClientConfig.InsecureSkipVerify = true
	}
	client.Transport = transport
}

// proxyForMPV returns a configured HTTP proxy URL when MPV supports it.
// Go clients may support HTTPS proxy endpoints, but MPV's option does not.
func (f *appHTTPClientFactory) proxyForMPV() string {
	if f.configuredProxy == nil || f.configuredProxy.Scheme != "http" {
		return ""
	}
	return f.configuredProxy.String()
}

// validateMPVProxy rejects configured proxy forms that MPV would otherwise
// silently ignore. MPV accepts HTTP proxy URLs, while the Go clients also
// support HTTPS proxy URLs.
func (f *appHTTPClientFactory) validateMPVProxy() error {
	if f.configuredProxyErr != nil {
		return fmt.Errorf("invalid configured proxy")
	}
	return f.mpvProxyErr
}

// configureMPVProxyEnvironment keeps MPV/FFmpeg's no_proxy environment in
// sync with the application proxy. MPV does not receive Go's implicit
// loopback exemption, so add the loopback host literals explicitly while
// preserving existing entries. CIDR entries are preserved, but MPV/FFmpeg
// may not support every NO_PROXY form that Go's httpproxy package supports.
func (f *appHTTPClientFactory) configureMPVProxyEnvironment() error {
	if f.configuredProxy == nil {
		return nil
	}

	parts := make([]string, 0, 8)
	seen := make(map[string]struct{}, 8)
	for _, value := range []string{os.Getenv("no_proxy"), os.Getenv("NO_PROXY"), "localhost", "127.0.0.1", "::1"} {
		for _, entry := range strings.Split(value, ",") {
			entry = strings.TrimSpace(entry)
			if entry == "" {
				continue
			}
			key := strings.ToLower(entry)
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			parts = append(parts, entry)
		}
	}
	noProxy := strings.Join(parts, ",")
	if err := os.Setenv("no_proxy", noProxy); err != nil {
		return err
	}
	return os.Setenv("NO_PROXY", noProxy)
}

func (f *appHTTPClientFactory) proxyForRequest(req *http.Request) (*url.URL, error) {
	return f.configuredProxyFunc(req.URL)
}
