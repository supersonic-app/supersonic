package backend

import (
	"crypto/tls"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"time"
)

// appHTTPClientFactory creates outbound HTTP clients with the application
// proxy setting applied. When no application proxy is configured, it leaves
// the default transport's ProxyFromEnvironment behavior intact.
type appHTTPClientFactory struct {
	configuredProxy    *url.URL
	hasConfiguredProxy bool
}

func newAppHTTPClientFactory(rawProxy string) *appHTTPClientFactory {
	factory := &appHTTPClientFactory{}
	if rawProxy == "" {
		return factory
	}

	factory.hasConfiguredProxy = true
	proxy, err := parseHTTPProxyURL(rawProxy)
	if err != nil {
		log.Printf("Warning: invalid proxy URL %q, ignoring proxy: %s", redactProxyURL(rawProxy), err)
		return factory
	}

	factory.configuredProxy = proxy
	log.Printf("Setting application HTTP proxy: %s", redactProxyURL(rawProxy))
	return factory
}

func parseHTTPProxyURL(rawProxy string) (*url.URL, error) {
	proxy, err := url.Parse(rawProxy)
	if err != nil {
		return nil, err
	}
	if proxy.Host == "" || (proxy.Scheme != "http" && proxy.Scheme != "https") {
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
	case f.configuredProxy != nil:
		transport.Proxy = http.ProxyURL(f.configuredProxy)
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

// proxyForMPV returns the configured proxy, or the HTTPS environment proxy
// that MPV historically honored when no application proxy is configured.
func (f *appHTTPClientFactory) proxyForMPV() string {
	if f.configuredProxy != nil {
		return f.configuredProxy.String()
	}
	if f.hasConfiguredProxy {
		return ""
	}
	return resolveHTTPSProxyFromEnvironment()
}
