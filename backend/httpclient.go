package backend

import (
	"crypto/tls"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"golang.org/x/net/http/httpproxy"
)

// appHTTPClientFactory creates outbound HTTP clients with the application
// HTTP proxy setting applied. A configured HTTP endpoint is used for both
// HTTP and HTTPS requests; without one, the transport keeps environment
// proxy semantics intact.
type appHTTPClientFactory struct {
	configuredProxy     *url.URL
	configuredProxyFunc func(*url.URL) (*url.URL, error)
	hasConfiguredProxy  bool
	configuredProxyErr  error
}

func newAppHTTPClientFactory(rawProxy string) *appHTTPClientFactory {
	factory := &appHTTPClientFactory{}
	rawProxy = strings.TrimSpace(rawProxy)
	if rawProxy == "" {
		return factory
	}

	proxy, err := parseHTTPProxyURL(rawProxy)
	if err != nil {
		log.Printf("Warning: invalid configured HTTP proxy; refusing to start with proxy configuration")
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
	log.Printf("Setting application HTTP proxy: %s", redactProxyURL(proxy.String()))
	return factory
}

func parseHTTPProxyURL(rawProxy string) (*url.URL, error) {
	rawProxy = strings.TrimSpace(rawProxy)
	if rawProxy == "" {
		return nil, fmt.Errorf("proxy URL is empty")
	}
	if !strings.Contains(rawProxy, "://") {
		// Treat a host[:port] value as an HTTP proxy, matching httpproxy.
		rawProxy = "http://" + rawProxy
	}
	proxy, err := url.Parse(rawProxy)
	if err != nil {
		return nil, fmt.Errorf("invalid proxy URL")
	}
	proxy.Scheme = strings.ToLower(proxy.Scheme)
	if proxy.Host == "" || proxy.Hostname() == "" || proxy.Scheme != "http" {
		return nil, fmt.Errorf("proxy URL must use the http scheme and include a host")
	}
	if err := validateHTTPProxyPort(proxy.Host); err != nil {
		return nil, err
	}
	return proxy, nil
}

func validateHTTPProxyPort(host string) error {
	var rawPort string
	switch {
	case strings.HasPrefix(host, "["):
		closeBracket := strings.LastIndexByte(host, ']')
		if closeBracket < 0 {
			return fmt.Errorf("proxy URL has malformed host")
		}
		if len(host) == closeBracket+1 {
			return nil
		}
		if host[closeBracket+1] != ':' {
			return fmt.Errorf("proxy URL has malformed port")
		}
		rawPort = host[closeBracket+2:]
	case strings.Count(host, ":") == 0:
		return nil
	case strings.Count(host, ":") == 1:
		rawPort = host[strings.LastIndexByte(host, ':')+1:]
	default:
		return fmt.Errorf("proxy URL has malformed host or port")
	}
	if rawPort == "" {
		return fmt.Errorf("proxy URL has an empty port")
	}
	for _, char := range rawPort {
		if char < '0' || char > '9' {
			return fmt.Errorf("proxy URL port must be numeric")
		}
	}
	port, err := strconv.Atoi(rawPort)
	if err != nil || port < 1 || port > 65535 {
		return fmt.Errorf("proxy URL port must be between 1 and 65535")
	}
	return nil
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

// proxyForMPV returns the configured HTTP proxy URL. An empty return value
// leaves proxy selection to MPV's own environment handling.
func (f *appHTTPClientFactory) proxyForMPV() string {
	if f.configuredProxy == nil {
		return ""
	}
	return f.configuredProxy.String()
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
