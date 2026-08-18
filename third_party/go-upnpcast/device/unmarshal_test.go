package device

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

const testRendererDescription = `<?xml version="1.0"?>
<root xmlns="urn:schemas-upnp-org:device-1-0"><device>
<deviceType>urn:schemas-upnp-org:device:MediaRenderer:1</deviceType>
<friendlyName>Renderer</friendlyName><modelName>Model</modelName>
<serviceList>
<service><serviceType>urn:schemas-upnp-org:service:AVTransport:1</serviceType><controlURL>control/transport</controlURL><eventSubURL>event/transport</eventSubURL></service>
<service><serviceType>urn:schemas-upnp-org:service:RenderingControl:1</serviceType><controlURL>/control/render</controlURL></service>
</serviceList></device></root>`

func TestMediaRendererFromDeviceURLResolvesServiceURLs(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, testRendererDescription)
	}))
	defer server.Close()

	renderer, err := mediaRendererFromDeviceURL(context.Background(), server.URL+"/upnp/description.xml")
	if err != nil {
		t.Fatal(err)
	}
	client, err := renderer.AVTransportClient()
	if err != nil {
		t.Fatal(err)
	}
	if client == nil || renderer.renderingControlURL != server.URL+"/control/render" {
		t.Fatalf("service URLs were not resolved correctly: %#v", renderer)
	}
}

func TestMediaRendererFromDeviceURLRejectsHTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
	}))
	defer server.Close()

	if _, err := mediaRendererFromDeviceURL(context.Background(), server.URL+"/description.xml"); err == nil {
		t.Fatal("expected HTTP error")
	}
}
