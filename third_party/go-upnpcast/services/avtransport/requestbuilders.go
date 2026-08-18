package avtransport

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"math"
	"strings"

	"github.com/supersonic-app/go-upnpcast/internal/utils"
)

type playEnvelope struct {
	XMLName  xml.Name `xml:"s:Envelope"`
	Schema   string   `xml:"xmlns:s,attr"`
	Encoding string   `xml:"s:encodingStyle,attr"`
	PlayBody playBody `xml:"s:Body"`
}

type playBody struct {
	XMLName    xml.Name   `xml:"s:Body"`
	PlayAction playAction `xml:"u:Play"`
}

type playAction struct {
	XMLName     xml.Name `xml:"u:Play"`
	AVTransport string   `xml:"xmlns:u,attr"`
	InstanceID  string
	Speed       string
}

type pauseEnvelope struct {
	XMLName   xml.Name  `xml:"s:Envelope"`
	Schema    string    `xml:"xmlns:s,attr"`
	Encoding  string    `xml:"s:encodingStyle,attr"`
	PauseBody pauseBody `xml:"s:Body"`
}

type pauseBody struct {
	XMLName     xml.Name    `xml:"s:Body"`
	PauseAction pauseAction `xml:"u:Pause"`
}

type pauseAction struct {
	XMLName     xml.Name `xml:"u:Pause"`
	AVTransport string   `xml:"xmlns:u,attr"`
	InstanceID  string
	Speed       string
}

type stopEnvelope struct {
	XMLName  xml.Name `xml:"s:Envelope"`
	Schema   string   `xml:"xmlns:s,attr"`
	Encoding string   `xml:"s:encodingStyle,attr"`
	StopBody stopBody `xml:"s:Body"`
}

type stopBody struct {
	XMLName    xml.Name   `xml:"s:Body"`
	StopAction stopAction `xml:"u:Stop"`
}

type stopAction struct {
	XMLName     xml.Name `xml:"u:Stop"`
	AVTransport string   `xml:"xmlns:u,attr"`
	InstanceID  string
	Speed       string
}

type seekEnvelope struct {
	XMLName  xml.Name `xml:"s:Envelope"`
	Schema   string   `xml:"xmlns:s,attr"`
	Encoding string   `xml:"s:encodingStyle,attr"`
	SeekBody seekBody `xml:"s:Body"`
}

type seekBody struct {
	XMLName    xml.Name   `xml:"s:Body"`
	SeekAction seekAction `xml:"u:Seek"`
}

type seekAction struct {
	XMLName     xml.Name `xml:"u:Seek"`
	AVTransport string   `xml:"xmlns:u,attr"`
	InstanceID  string
	Unit        string
	Target      string
}

type setAVTransportEnvelope struct {
	XMLName  xml.Name           `xml:"s:Envelope"`
	Schema   string             `xml:"xmlns:s,attr"`
	Encoding string             `xml:"s:encodingStyle,attr"`
	Body     setAVTransportBody `xml:"s:Body"`
}

type setAVTransportBody struct {
	XMLName           xml.Name          `xml:"s:Body"`
	SetAVTransportURI setAVTransportURI `xml:"u:SetAVTransportURI"`
}

type setAVTransportURI struct {
	XMLName            xml.Name `xml:"u:SetAVTransportURI"`
	AVTransport        string   `xml:"xmlns:u,attr"`
	InstanceID         string
	CurrentURI         string
	CurrentURIMetaData currentURIMetaData `xml:"CurrentURIMetaData"`
}

type currentURIMetaData struct {
	XMLName xml.Name `xml:"CurrentURIMetaData"`
	Value   []byte   `xml:",chardata"`
}

type setNextAVTransportEnvelope struct {
	XMLName  xml.Name               `xml:"s:Envelope"`
	Schema   string                 `xml:"xmlns:s,attr"`
	Encoding string                 `xml:"s:encodingStyle,attr"`
	Body     setNextAVTransportBody `xml:"s:Body"`
}

type setNextAVTransportBody struct {
	XMLName               xml.Name              `xml:"s:Body"`
	SetNextAVTransportURI setNextAVTransportURI `xml:"u:SetNextAVTransportURI"`
}

type setNextAVTransportURI struct {
	XMLName         xml.Name `xml:"u:SetNextAVTransportURI"`
	AVTransport     string   `xml:"xmlns:u,attr"`
	InstanceID      string
	NextURI         string
	NextURIMetaData nextURIMetaData `xml:"NextURIMetaData"`
}

type nextURIMetaData struct {
	XMLName xml.Name `xml:"NextURIMetaData"`
	Value   []byte   `xml:",chardata"`
}

type didLLite struct {
	XMLName      xml.Name     `xml:"DIDL-Lite"`
	SchemaDIDL   string       `xml:"xmlns,attr"`
	DC           string       `xml:"xmlns:dc,attr"`
	Sec          string       `xml:"xmlns:sec,attr"`
	SchemaUPNP   string       `xml:"xmlns:upnp,attr"`
	DIDLLiteItem didLLiteItem `xml:"item"`
}

type didLLiteItem struct {
	SecCaptionInfo       *secCaptionInfo   `xml:"sec:CaptionInfo,omitempty"`
	SecCaptionInfoEx     *secCaptionInfoEx `xml:"sec:CaptionInfoEx,omitempty"`
	XMLName              xml.Name          `xml:"item"`
	DCtitle              string            `xml:"dc:title"`
	DCcreator            string            `xml:"dc:creator,omitempty"`
	UPNPartist           string            `xml:"upnp:artist,omitempty"`
	UPNPalbum            string            `xml:"upnp:album,omitempty"`
	UPNPoriginalTrackNum int               `xml:"upnp:originalTrackNumber,omitempty"`
	UPNPalbumArtURI      string            `xml:"upnp:albumArtURI,omitempty"`
	UPNPClass            string            `xml:"upnp:class"`
	ID                   string            `xml:"id,attr"`
	ParentID             string            `xml:"parentID,attr"`
	Restricted           string            `xml:"restricted,attr"`
	ResNode              []resNode         `xml:"res"`
}

type resNode struct {
	XMLName         xml.Name `xml:"res"`
	Duration        string   `xml:"duration,attr,omitempty"`
	Size            int64    `xml:"size,attr,omitempty"`
	Bitrate         int      `xml:"bitrate,attr,omitempty"`
	SampleFrequency int      `xml:"sampleFrequency,attr,omitempty"`
	BitsPerSample   int      `xml:"bitsPerSample,attr,omitempty"`
	NrAudioChannels int      `xml:"nrAudioChannels,attr,omitempty"`
	ProtocolInfo    string   `xml:"protocolInfo,attr"`
	Value           string   `xml:",chardata"`
}

type secCaptionInfo struct {
	XMLName xml.Name `xml:"sec:CaptionInfo"`
	Type    string   `xml:"sec:type,attr"`
	Value   string   `xml:",chardata"`
}

type secCaptionInfoEx struct {
	XMLName xml.Name `xml:"sec:CaptionInfoEx"`
	Type    string   `xml:"sec:type,attr"`
	Value   string   `xml:",chardata"`
}

type getMediaInfoEnvelope struct {
	XMLName          xml.Name         `xml:"s:Envelope"`
	Schema           string           `xml:"xmlns:s,attr"`
	Encoding         string           `xml:"s:encodingStyle,attr"`
	GetMediaInfoBody getMediaInfoBody `xml:"s:Body"`
}

type getMediaInfoBody struct {
	XMLName            xml.Name           `xml:"s:Body"`
	GetMediaInfoAction getMediaInfoAction `xml:"u:GetMediaInfo"`
}

type getMediaInfoAction struct {
	XMLName     xml.Name `xml:"u:GetMediaInfo"`
	AVTransport string   `xml:"xmlns:u,attr"`
	InstanceID  string
}

type getTransportInfoEnvelope struct {
	XMLName              xml.Name             `xml:"s:Envelope"`
	Schema               string               `xml:"xmlns:s,attr"`
	Encoding             string               `xml:"s:encodingStyle,attr"`
	GetTransportInfoBody getTransportInfoBody `xml:"s:Body"`
}

type getTransportInfoBody struct {
	XMLName                xml.Name               `xml:"s:Body"`
	GetTransportInfoAction getTransportInfoAction `xml:"u:GetTransportInfo"`
}

type getTransportInfoAction struct {
	XMLName     xml.Name `xml:"u:GetTransportInfo"`
	AVTransport string   `xml:"xmlns:u,attr"`
	InstanceID  string
}

type getPositionInfoEnvelope struct {
	XMLName             xml.Name            `xml:"s:Envelope"`
	Schema              string              `xml:"xmlns:s,attr"`
	Encoding            string              `xml:"s:encodingStyle,attr"`
	GetPositionInfoBody getPositionInfoBody `xml:"s:Body"`
}

type getPositionInfoBody struct {
	XMLName               xml.Name              `xml:"s:Body"`
	GetPositionInfoAction getPositionInfoAction `xml:"u:GetPositionInfo"`
}

type getPositionInfoAction struct {
	XMLName     xml.Name `xml:"u:GetPositionInfo"`
	AVTransport string   `xml:"xmlns:u,attr"`
	InstanceID  string
}

func buildURIMetadataPayload(media *MediaItem) ([]byte, error) {
	mediaTypeSlice := strings.Split(media.ContentType, "/")
	seekflag := "00"
	if media.Seekable {
		seekflag = "01"
	}

	contentFeatures, err := utils.BuildContentFeatures(media.ContentType, seekflag, false /*transcode*/)
	if err != nil {
		return nil, fmt.Errorf("buildURIMetadataPayload failed to build contentFeatures: %w", err)
	}

	var class string
	switch mediaTypeSlice[0] {
	case "audio":
		class = "object.item.audioItem.musicTrack"
	case "image":
		class = "object.item.imageItem.photo"
	default:
		class = "object.item.videoItem.movie"
	}

	var didl didLLiteItem

	primary := resNode{
		XMLName:         xml.Name{},
		Size:            media.Size,
		Bitrate:         media.Bitrate,
		SampleFrequency: media.SampleFrequency,
		BitsPerSample:   media.BitsPerSample,
		NrAudioChannels: media.NrAudioChannels,
		ProtocolInfo:    fmt.Sprintf("http-get:*:%s:%s", media.ContentType, contentFeatures),
		Value:           media.URL,
	}
	if media.Duration > 0 {
		primary.Duration = utils.SecondsToClockTime(int(math.Round(media.Duration.Seconds())))
	}
	resNodeData := []resNode{primary}

	// Pre-escape free-text fields so that the Samsung TV un-escape pass below
	// leaves them XML-safe (xml.Marshal escapes them once, then the un-escape
	// removes one layer, netting a single layer of escaping).
	escape := func(s string) string {
		var buf bytes.Buffer
		if err := xml.EscapeText(&buf, []byte(s)); err != nil {
			return ""
		}
		return buf.String()
	}
	didl = didLLiteItem{
		XMLName:              xml.Name{},
		ID:                   "1",
		ParentID:             "0",
		Restricted:           "1",
		UPNPClass:            class,
		DCtitle:              escape(media.Title),
		DCcreator:            escape(media.Artist),
		UPNPartist:           escape(media.Artist),
		UPNPalbum:            escape(media.Album),
		UPNPoriginalTrackNum: media.TrackNumber,
		UPNPalbumArtURI:      escape(media.AlbumArtURI),
		ResNode:              resNodeData,
	}

	if strings.Contains(media.SubtitlesURL, "srt") {
		resNodeData = append(resNodeData, resNode{
			XMLName:      xml.Name{},
			ProtocolInfo: "http-get:*:text/srt:*",
			Value:        media.SubtitlesURL,
		})
		didl.ResNode = resNodeData
		didl.SecCaptionInfo = &secCaptionInfo{
			XMLName: xml.Name{},
			Type:    "srt",
			Value:   media.SubtitlesURL,
		}
		didl.SecCaptionInfoEx = &secCaptionInfoEx{
			XMLName: xml.Name{},
			Type:    "srt",
			Value:   media.SubtitlesURL,
		}
	}

	l := didLLite{
		XMLName:      xml.Name{},
		SchemaDIDL:   "urn:schemas-upnp-org:metadata-1-0/DIDL-Lite/",
		DC:           "http://purl.org/dc/elements/1.1/",
		Sec:          "http://www.sec.co.kr/",
		SchemaUPNP:   "urn:schemas-upnp-org:metadata-1-0/upnp/",
		DIDLLiteItem: didl,
	}

	a, err := xml.Marshal(l)
	if err != nil {
		return nil, fmt.Errorf("buildURIMetadataPayload #1 Marshal error: %w", err)
	}
	return a, nil
}

func setAVTransportSoapBuild(media *MediaItem) (io.Reader, error) {
	meta, err := buildURIMetadataPayload(media)
	if err != nil {
		return nil, err
	}
	d := setAVTransportEnvelope{
		XMLName:  xml.Name{},
		Schema:   "http://schemas.xmlsoap.org/soap/envelope/",
		Encoding: "http://schemas.xmlsoap.org/soap/encoding/",
		Body: setAVTransportBody{
			XMLName: xml.Name{},
			SetAVTransportURI: setAVTransportURI{
				XMLName:     xml.Name{},
				AVTransport: "urn:schemas-upnp-org:service:AVTransport:1",
				InstanceID:  "0",
				CurrentURI:  media.URL,
				CurrentURIMetaData: currentURIMetaData{
					XMLName: xml.Name{},
					Value:   meta,
				},
			},
		},
	}
	b, err := xml.Marshal(d)
	if err != nil {
		return nil, fmt.Errorf("setAVTransportSoapBuild #2 Marshal error: %w", err)
	}

	// Samsung TV hack.
	b = bytes.ReplaceAll(b, []byte("&#34;"), []byte(`"`))
	b = bytes.ReplaceAll(b, []byte("&amp;"), []byte("&"))

	return io.MultiReader(strings.NewReader(utils.XMLStart), bytes.NewReader(b)), nil
}

func setNextAVTransportSoapBuild(media *MediaItem) (io.Reader, error) {
	meta, err := buildURIMetadataPayload(media)
	if err != nil {
		return nil, err
	}

	d := setNextAVTransportEnvelope{
		XMLName:  xml.Name{},
		Schema:   "http://schemas.xmlsoap.org/soap/envelope/",
		Encoding: "http://schemas.xmlsoap.org/soap/encoding/",
		Body: setNextAVTransportBody{
			XMLName: xml.Name{},
			SetNextAVTransportURI: setNextAVTransportURI{
				XMLName:     xml.Name{},
				AVTransport: "urn:schemas-upnp-org:service:AVTransport:1",
				InstanceID:  "0",
				NextURI:     media.URL,
				NextURIMetaData: nextURIMetaData{
					XMLName: xml.Name{},
					Value:   meta,
				},
			},
		},
	}
	b, err := xml.Marshal(d)
	if err != nil {
		return nil, fmt.Errorf("setNextAVTransportSoapBuild #2 Marshal error: %w", err)
	}

	// Samsung TV hack.
	b = bytes.ReplaceAll(b, []byte("&#34;"), []byte(`"`))
	b = bytes.ReplaceAll(b, []byte("&amp;"), []byte("&"))

	return io.MultiReader(strings.NewReader(utils.XMLStart), bytes.NewReader(b)), nil
}

func playSoapBuild() (io.Reader, error) {
	d := playEnvelope{
		XMLName:  xml.Name{},
		Schema:   "http://schemas.xmlsoap.org/soap/envelope/",
		Encoding: "http://schemas.xmlsoap.org/soap/encoding/",
		PlayBody: playBody{
			XMLName: xml.Name{},
			PlayAction: playAction{
				XMLName:     xml.Name{},
				AVTransport: "urn:schemas-upnp-org:service:AVTransport:1",
				InstanceID:  "0",
				Speed:       "1",
			},
		},
	}
	return utils.MarshalXMLWithStart(d)
}

func stopSoapBuild() (io.Reader, error) {
	d := stopEnvelope{
		XMLName:  xml.Name{},
		Schema:   "http://schemas.xmlsoap.org/soap/envelope/",
		Encoding: "http://schemas.xmlsoap.org/soap/encoding/",
		StopBody: stopBody{
			XMLName: xml.Name{},
			StopAction: stopAction{
				XMLName:     xml.Name{},
				AVTransport: "urn:schemas-upnp-org:service:AVTransport:1",
				InstanceID:  "0",
				Speed:       "1",
			},
		},
	}
	return utils.MarshalXMLWithStart(d)
}

func pauseSoapBuild() (io.Reader, error) {
	d := pauseEnvelope{
		XMLName:  xml.Name{},
		Schema:   "http://schemas.xmlsoap.org/soap/envelope/",
		Encoding: "http://schemas.xmlsoap.org/soap/encoding/",
		PauseBody: pauseBody{
			XMLName: xml.Name{},
			PauseAction: pauseAction{
				XMLName:     xml.Name{},
				AVTransport: "urn:schemas-upnp-org:service:AVTransport:1",
				InstanceID:  "0",
				Speed:       "1",
			},
		},
	}
	return utils.MarshalXMLWithStart(d)
}

func getMediaInfoSoapBuild() (io.Reader, error) {
	d := getMediaInfoEnvelope{
		XMLName:  xml.Name{},
		Schema:   "http://schemas.xmlsoap.org/soap/envelope/",
		Encoding: "http://schemas.xmlsoap.org/soap/encoding/",
		GetMediaInfoBody: getMediaInfoBody{
			XMLName: xml.Name{},
			GetMediaInfoAction: getMediaInfoAction{
				XMLName:     xml.Name{},
				AVTransport: "urn:schemas-upnp-org:service:AVTransport:1",
				InstanceID:  "0",
			},
		},
	}
	return utils.MarshalXMLWithStart(d)
}

func getTransportInfoSoapBuild() (io.Reader, error) {
	d := getTransportInfoEnvelope{
		XMLName:  xml.Name{},
		Schema:   "http://schemas.xmlsoap.org/soap/envelope/",
		Encoding: "http://schemas.xmlsoap.org/soap/encoding/",
		GetTransportInfoBody: getTransportInfoBody{
			XMLName: xml.Name{},
			GetTransportInfoAction: getTransportInfoAction{
				XMLName:     xml.Name{},
				AVTransport: "urn:schemas-upnp-org:service:AVTransport:1",
				InstanceID:  "0",
			},
		},
	}
	return utils.MarshalXMLWithStart(d)
}

func getPositionInfoSoapBuild() (io.Reader, error) {
	d := getPositionInfoEnvelope{
		XMLName:  xml.Name{},
		Schema:   "http://schemas.xmlsoap.org/soap/envelope/",
		Encoding: "http://schemas.xmlsoap.org/soap/encoding/",
		GetPositionInfoBody: getPositionInfoBody{
			XMLName: xml.Name{},
			GetPositionInfoAction: getPositionInfoAction{
				XMLName:     xml.Name{},
				AVTransport: "urn:schemas-upnp-org:service:AVTransport:1",
				InstanceID:  "0",
			},
		},
	}
	return utils.MarshalXMLWithStart(d)
}

func seekSoapBuild(reltime string) (io.Reader, error) {
	d := seekEnvelope{
		XMLName:  xml.Name{},
		Schema:   "http://schemas.xmlsoap.org/soap/envelope/",
		Encoding: "http://schemas.xmlsoap.org/soap/encoding/",
		SeekBody: seekBody{
			XMLName: xml.Name{},
			SeekAction: seekAction{
				XMLName:     xml.Name{},
				AVTransport: "urn:schemas-upnp-org:service:AVTransport:1",
				InstanceID:  "0",
				Unit:        "REL_TIME",
				Target:      reltime,
			},
		},
	}
	return utils.MarshalXMLWithStart(d)
}
