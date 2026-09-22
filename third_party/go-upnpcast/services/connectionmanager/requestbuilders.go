package connectionmanager

import (
	"encoding/xml"
	"io"

	"github.com/supersonic-app/go-upnpcast/internal/utils"
)

type getProtocolInfoEnvelope struct {
	XMLName             xml.Name            `xml:"s:Envelope"`
	Schema              string              `xml:"xmlns:s,attr"`
	Encoding            string              `xml:"s:encodingStyle,attr"`
	GetProtocolInfoBody getProtocolInfoBody `xml:"s:Body"`
}

type getProtocolInfoBody struct {
	XMLName               xml.Name              `xml:"s:Body"`
	GetProtocolInfoAction getProtocolInfoAction `xml:"u:GetProtocolInfo"`
}

type getProtocolInfoAction struct {
	XMLName           xml.Name `xml:"u:GetProtocolInfo"`
	ConnectionManager string   `xml:"xmlns:u,attr"`
}

func getProtocolInfoSoapBuild() (io.Reader, error) {
	d := getProtocolInfoEnvelope{
		XMLName:  xml.Name{},
		Schema:   "http://schemas.xmlsoap.org/soap/envelope/",
		Encoding: "http://schemas.xmlsoap.org/soap/encoding/",
		GetProtocolInfoBody: getProtocolInfoBody{
			XMLName: xml.Name{},
			GetProtocolInfoAction: getProtocolInfoAction{
				XMLName:           xml.Name{},
				ConnectionManager: "urn:schemas-upnp-org:service:ConnectionManager:1",
			},
		},
	}
	return utils.MarshalXMLWithStart(d)
}
