package renderingcontrol

import "encoding/xml"

type getMuteRespBody struct {
	XMLName       xml.Name `xml:"Envelope"`
	Text          string   `xml:",chardata"`
	EncodingStyle string   `xml:"encodingStyle,attr"`
	S             string   `xml:"s,attr"`
	Body          struct {
		Text            string `xml:",chardata"`
		GetMuteResponse struct {
			Text        string `xml:",chardata"`
			U           string `xml:"u,attr"`
			CurrentMute string `xml:"CurrentMute"`
		} `xml:"GetMuteResponse"`
	} `xml:"Body"`
}

type getVolumeRespBody struct {
	XMLName       xml.Name `xml:"Envelope"`
	Text          string   `xml:",chardata"`
	EncodingStyle string   `xml:"encodingStyle,attr"`
	S             string   `xml:"s,attr"`
	Body          struct {
		Text              string `xml:",chardata"`
		GetVolumeResponse struct {
			Text          string `xml:",chardata"`
			U             string `xml:"u,attr"`
			CurrentVolume string `xml:"CurrentVolume"`
		} `xml:"GetVolumeResponse"`
	} `xml:"Body"`
}
