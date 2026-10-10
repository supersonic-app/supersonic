package services

import "net/http"

type Type = string

const (
	AVTransport       Type = "urn:schemas-upnp-org:service:AVTransport:1"
	ConnectionManager Type = "urn:schemas-upnp-org:service:ConnectionManager:1"
	RenderingControl  Type = "urn:schemas-upnp-org:service:RenderingControl:1"
)

type RequestHandler interface {
	Do(request *http.Request) (*http.Response, error)
}
