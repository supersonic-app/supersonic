package avtransport

import "encoding/xml"

type getPositionInfoResponse struct {
	XMLName       xml.Name `xml:"Envelope"`
	Text          string   `xml:",chardata"`
	S             string   `xml:"s,attr"`
	EncodingStyle string   `xml:"encodingStyle,attr"`
	Body          struct {
		Text                    string `xml:",chardata"`
		GetPositionInfoResponse struct {
			Text          string `xml:",chardata"`
			U             string `xml:"u,attr"`
			Track         string `xml:"Track"`
			TrackDuration string `xml:"TrackDuration"`
			TrackMetaData string `xml:"TrackMetaData"`
			TrackURI      string `xml:"TrackURI"`
			RelTime       string `xml:"RelTime"`
			AbsTime       string `xml:"AbsTime"`
			RelCount      string `xml:"RelCount"`
			AbsCount      string `xml:"AbsCount"`
		} `xml:"GetPositionInfoResponse"`
	} `xml:"Body"`
}

type getTransportInfoResponse struct {
	XMLName       xml.Name `xml:"Envelope"`
	Text          string   `xml:",chardata"`
	S             string   `xml:"s,attr"`
	EncodingStyle string   `xml:"encodingStyle,attr"`
	Body          struct {
		Text                     string `xml:",chardata"`
		GetTransportInfoResponse struct {
			Text                   string `xml:",chardata"`
			U                      string `xml:"u,attr"`
			CurrentTransportState  string `xml:"CurrentTransportState"`
			CurrentTransportStatus string `xml:"CurrentTransportStatus"`
			CurrentSpeed           string `xml:"CurrentSpeed"`
		} `xml:"GetTransportInfoResponse"`
	} `xml:"Body"`
}

type setAVTransportURIResponse struct {
	XMLName       xml.Name `xml:"Envelope"`
	Text          string   `xml:",chardata"`
	S             string   `xml:"s,attr"`
	EncodingStyle string   `xml:"encodingStyle,attr"`
	Body          struct {
		Text                      string `xml:",chardata"`
		SetAVTransportURIResponse struct {
			Text string `xml:",chardata"`
			U    string `xml:"u,attr"`
		} `xml:"SetAVTransportURIResponse"`
	} `xml:"Body"`
}

type setNextAVTransportURIResponse struct {
	XMLName       xml.Name `xml:"Envelope"`
	Text          string   `xml:",chardata"`
	S             string   `xml:"s,attr"`
	EncodingStyle string   `xml:"encodingStyle,attr"`
	Body          struct {
		Text                      string `xml:",chardata"`
		SetAVTransportURIResponse struct {
			Text string `xml:",chardata"`
			U    string `xml:"u,attr"`
		} `xml:"SetNextAVTransportURIResponse"`
	} `xml:"Body"`
}
