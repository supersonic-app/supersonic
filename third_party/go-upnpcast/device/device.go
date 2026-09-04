package device

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/koron/go-ssdp"
	"github.com/supersonic-app/go-upnpcast/services"
	"github.com/supersonic-app/go-upnpcast/services/avtransport"
	"github.com/supersonic-app/go-upnpcast/services/renderingcontrol"
)

var ErrUnsupportedService = errors.New("the device does not support the requested service")

// MediaRenderer represents a Digital Media Renderer (DMR) device discovered on the LAN
type MediaRenderer struct {
	// URL for the device's service descrption manifest
	URL string

	// Friendly name of the device
	FriendlyName string

	// Model name of the device
	ModelName string

	avTransportControlURL  string
	avTransportEventSubURL string
	renderingControlURL    string
	connectionManagerURL   string
}

// SearchMediaRenderers searches for MediaRenderer devices on the LAN
// that implement all the service types specified in `requiredServices`
// - waitSec is how many seconds to wait for device responses to the SSDP search
// If passing a context with deadline/expiration, it should be longer than waitSec
func SearchMediaRenderers(ctx context.Context, waitSec int, requiredServices ...services.Type) ([]*MediaRenderer, error) {
	deviceLocations, err := getSSDPAVTransportDeviceLocations(waitSec)
	if err != nil {
		return nil, err
	}

	devices := make([]*MediaRenderer, 0, len(deviceLocations))
	errors := []error{}
	for _, l := range deviceLocations {
		mr, err := mediaRendererFromDeviceURL(ctx, l)
		if err != nil {
			errors = append(errors, err)
			continue
		}
		for _, required := range requiredServices {
			if !mr.SupportsService(required) {
				errors = append(errors, fmt.Errorf("%s does not support %s", mr.FriendlyName, required))
				mr = nil
				break
			}
		}
		if mr == nil {
			continue
		}
		devices = append(devices, mr)
	}

	return devices, cmp.Or(errors...)
}

// SupportsService returns true if the MediaRenderer supports the given service type
func (m *MediaRenderer) SupportsService(serviceType services.Type) bool {
	switch serviceType {
	case services.AVTransport:
		return m.avTransportControlURL != "" && m.avTransportEventSubURL != ""
	case services.ConnectionManager:
		return m.connectionManagerURL != ""
	case services.RenderingControl:
		return m.renderingControlURL != ""
	}
	return false
}

// AVTransportClient returns a new client to the device's AVTransport service.
func (m *MediaRenderer) AVTransportClient() (*avtransport.Client, error) {
	if !m.SupportsService(services.AVTransport) {
		return nil, ErrUnsupportedService
	}
	return avtransport.NewClient(m.avTransportControlURL, m.avTransportEventSubURL), nil
}

// RenderingControlClient returns a new client to the device's RenderingControl service.
func (m *MediaRenderer) RenderingControlClient() (*renderingcontrol.Client, error) {
	if !m.SupportsService(services.RenderingControl) {
		return nil, ErrUnsupportedService
	}
	return renderingcontrol.NewClient(m.renderingControlURL), nil
}

// Gets the list of DMR schema URLs for all found devices that support the AVTransport service
func getSSDPAVTransportDeviceLocations(waitSec int) ([]string, error) {
	ssdpServices, err := ssdp.Search(ssdp.All, waitSec, "")
	if err != nil {
		return nil, fmt.Errorf("SSDP search error: %w", err)
	}

	var deviceLocations listSet
	for _, srv := range ssdpServices {
		// All DMRs we care about must support the AVTransport service
		if srv.Type == services.AVTransport {
			deviceLocations.add(srv.Location)
		}
	}
	return deviceLocations, nil
}

type listSet []string

func (l *listSet) add(s string) {
	if slices.Contains(*l, s) {
		return
	}
	*l = append(*l, s)
}
