package avtransport

import (
	"io"
	"strings"
	"testing"
	"time"

	"github.com/supersonic-app/go-upnpcast/internal/utils"
)

func TestSetAVTransportSoapBuild(t *testing.T) {
	tt := []struct {
		name  string
		media *MediaItem
	}{
		{
			`setAVTransportSoapBuild Test #1`,
			&MediaItem{
				Title:        "foo",
				URL:          `http://192.168.88.250:3500/video%20%26%20%27example%27.mp4`,
				ContentType:  "video/mp4",
				SubtitlesURL: "http://192.168.88.250:3500/video_example.srt",
				//Transcode:    false,
				Seekable: true,
			},
		},
	}

	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			seekflag := "00"
			if tc.media.Seekable {
				seekflag = "01"
			}

			contentFeatures, err := utils.BuildContentFeatures(tc.media.ContentType, seekflag, false /*transcode - TODO*/)
			if err != nil {
				t.Fatalf("%s: setAVTransportSoapBuild failed to build contentFeatures: %s", tc.name, err.Error())
			}

			want := `<?xml version="1.0" encoding="utf-8"?><s:Envelope xmlns:s="http://schemas.xmlsoap.org/soap/envelope/" s:encodingStyle="http://schemas.xmlsoap.org/soap/encoding/"><s:Body><u:SetAVTransportURI xmlns:u="urn:schemas-upnp-org:service:AVTransport:1"><InstanceID>0</InstanceID><CurrentURI>http://192.168.88.250:3500/video%20%26%20%27example%27.mp4</CurrentURI><CurrentURIMetaData>&lt;DIDL-Lite xmlns="urn:schemas-upnp-org:metadata-1-0/DIDL-Lite/" xmlns:dc="http://purl.org/dc/elements/1.1/" xmlns:sec="http://www.sec.co.kr/" xmlns:upnp="urn:schemas-upnp-org:metadata-1-0/upnp/"&gt;&lt;item id="1" parentID="0" restricted="1"&gt;&lt;sec:CaptionInfo sec:type="srt"&gt;http://192.168.88.250:3500/video_example.srt&lt;/sec:CaptionInfo&gt;&lt;sec:CaptionInfoEx sec:type="srt"&gt;http://192.168.88.250:3500/video_example.srt&lt;/sec:CaptionInfoEx&gt;&lt;dc:title&gt;foo&lt;/dc:title&gt;&lt;upnp:class&gt;object.item.videoItem.movie&lt;/upnp:class&gt;&lt;res protocolInfo="http-get:*:video/mp4:` + contentFeatures + `"&gt;http://192.168.88.250:3500/video%20%26%20%27example%27.mp4&lt;/res&gt;&lt;res protocolInfo="http-get:*:text/srt:*"&gt;http://192.168.88.250:3500/video_example.srt&lt;/res&gt;&lt;/item&gt;&lt;/DIDL-Lite&gt;</CurrentURIMetaData></u:SetAVTransportURI></s:Body></s:Envelope>`

			out, err := setAVTransportSoapBuild(tc.media)
			if err != nil {
				t.Fatalf("%s: Failed to call setAVTransportSoapBuild due to %s", tc.name, err.Error())
			}
			if readerToString(out) != want {
				t.Fatalf("%s: got: %s, want: %s.", tc.name, out, want)
			}
		})
	}
}

func TestSetNextAVTransportSoapBuild(t *testing.T) {
	tt := []struct {
		name string
		tv   *MediaItem
	}{
		{
			`setNextAVTransportSoapBuild Test #1`,
			&MediaItem{
				Title:        "foo",
				URL:          `http://192.168.88.250:3500/video%20%26%20%27example%27.mp4`,
				ContentType:  "video/mp4",
				SubtitlesURL: "http://192.168.88.250:3500/video_example.srt",
				//Transcode:    false,
				Seekable: true,
			},
		},
	}

	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			seekflag := "00"
			if tc.tv.Seekable {
				seekflag = "01"
			}

			contentFeatures, err := utils.BuildContentFeatures(tc.tv.ContentType, seekflag, false /*transcode*/)
			if err != nil {
				t.Fatalf("%s: setNextAVTransportSoapBuild failed to build contentFeatures: %s", tc.name, err.Error())
			}

			want := `<?xml version="1.0" encoding="utf-8"?><s:Envelope xmlns:s="http://schemas.xmlsoap.org/soap/envelope/" s:encodingStyle="http://schemas.xmlsoap.org/soap/encoding/"><s:Body><u:SetNextAVTransportURI xmlns:u="urn:schemas-upnp-org:service:AVTransport:1"><InstanceID>0</InstanceID><NextURI>http://192.168.88.250:3500/video%20%26%20%27example%27.mp4</NextURI><NextURIMetaData>&lt;DIDL-Lite xmlns="urn:schemas-upnp-org:metadata-1-0/DIDL-Lite/" xmlns:dc="http://purl.org/dc/elements/1.1/" xmlns:sec="http://www.sec.co.kr/" xmlns:upnp="urn:schemas-upnp-org:metadata-1-0/upnp/"&gt;&lt;item id="1" parentID="0" restricted="1"&gt;&lt;sec:CaptionInfo sec:type="srt"&gt;http://192.168.88.250:3500/video_example.srt&lt;/sec:CaptionInfo&gt;&lt;sec:CaptionInfoEx sec:type="srt"&gt;http://192.168.88.250:3500/video_example.srt&lt;/sec:CaptionInfoEx&gt;&lt;dc:title&gt;foo&lt;/dc:title&gt;&lt;upnp:class&gt;object.item.videoItem.movie&lt;/upnp:class&gt;&lt;res protocolInfo="http-get:*:video/mp4:` + contentFeatures + `"&gt;http://192.168.88.250:3500/video%20%26%20%27example%27.mp4&lt;/res&gt;&lt;res protocolInfo="http-get:*:text/srt:*"&gt;http://192.168.88.250:3500/video_example.srt&lt;/res&gt;&lt;/item&gt;&lt;/DIDL-Lite&gt;</NextURIMetaData></u:SetNextAVTransportURI></s:Body></s:Envelope>`

			out, err := setNextAVTransportSoapBuild(tc.tv)
			if err != nil {
				t.Fatalf("%s: Failed to call setNextAVTransportSoapBuild due to %s", tc.name, err.Error())
			}
			if readerToString(out) != want {
				t.Fatalf("%s: got: %s, want: %s.", tc.name, out, want)
			}
		})
	}
}

func TestPlaySoapBuild(t *testing.T) {
	tt := []struct {
		name string
		want string
	}{
		{
			`playSoapBuild Test #1`,
			`<?xml version="1.0" encoding="utf-8"?><s:Envelope xmlns:s="http://schemas.xmlsoap.org/soap/envelope/" s:encodingStyle="http://schemas.xmlsoap.org/soap/encoding/"><s:Body><u:Play xmlns:u="urn:schemas-upnp-org:service:AVTransport:1"><InstanceID>0</InstanceID><Speed>1</Speed></u:Play></s:Body></s:Envelope>`,
		},
	}

	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			out, err := playSoapBuild()
			if err != nil {
				t.Fatalf("%s: Failed to call playSoapBuild due to %s", tc.name, err.Error())
			}
			if readerToString(out) != tc.want {
				t.Fatalf("%s: got: %s, want: %s.", tc.name, out, tc.want)
			}
		})
	}
}

func TestStopSoapBuild(t *testing.T) {
	tt := []struct {
		name string
		want string
	}{
		{
			`stopSoapBuild Test #1`,
			`<?xml version="1.0" encoding="utf-8"?><s:Envelope xmlns:s="http://schemas.xmlsoap.org/soap/envelope/" s:encodingStyle="http://schemas.xmlsoap.org/soap/encoding/"><s:Body><u:Stop xmlns:u="urn:schemas-upnp-org:service:AVTransport:1"><InstanceID>0</InstanceID><Speed>1</Speed></u:Stop></s:Body></s:Envelope>`,
		},
	}

	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			out, err := stopSoapBuild()
			if err != nil {
				t.Fatalf("%s: Failed to call stopSoapBuild due to %s", tc.name, err.Error())
			}
			if readerToString(out) != tc.want {
				t.Fatalf("%s: got: %s, want: %s.", tc.name, out, tc.want)
			}
		})
	}
}

func TestPauseSoapBuild(t *testing.T) {
	tt := []struct {
		name string
		want string
	}{
		{
			`pauseSoapBuild Test #1`,
			`<?xml version="1.0" encoding="utf-8"?><s:Envelope xmlns:s="http://schemas.xmlsoap.org/soap/envelope/" s:encodingStyle="http://schemas.xmlsoap.org/soap/encoding/"><s:Body><u:Pause xmlns:u="urn:schemas-upnp-org:service:AVTransport:1"><InstanceID>0</InstanceID><Speed>1</Speed></u:Pause></s:Body></s:Envelope>`,
		},
	}

	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			out, err := pauseSoapBuild()
			if err != nil {
				t.Fatalf("%s: Failed to call pauseSoapBuild due to %s", tc.name, err.Error())
			}
			if readerToString(out) != tc.want {
				t.Fatalf("%s: got: %s, want: %s.", tc.name, out, tc.want)
			}
		})
	}
}

func TestGetTransportInfoSoapBuild(t *testing.T) {
	tt := []struct {
		name string
		want string
	}{
		{
			`getTransportInfoSoapBuildTest #1`,
			`<?xml version="1.0" encoding="utf-8"?><s:Envelope xmlns:s="http://schemas.xmlsoap.org/soap/envelope/" s:encodingStyle="http://schemas.xmlsoap.org/soap/encoding/"><s:Body><u:GetTransportInfo xmlns:u="urn:schemas-upnp-org:service:AVTransport:1"><InstanceID>0</InstanceID></u:GetTransportInfo></s:Body></s:Envelope>`,
		},
	}

	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			out, err := getTransportInfoSoapBuild()
			if err != nil {
				t.Fatalf("%s: Failed to call getTransportInfoSoapBuild due to %s", tc.name, err.Error())
			}
			if readerToString(out) != tc.want {
				t.Fatalf("%s: got: %s, want: %s.", tc.name, out, tc.want)
			}
		})
	}
}

func TestGetPositionInfoSoapBuild(t *testing.T) {
	tt := []struct {
		name string
		want string
	}{
		{
			`getPositionInfoSoapBuildTest #1`,
			`<?xml version="1.0" encoding="utf-8"?><s:Envelope xmlns:s="http://schemas.xmlsoap.org/soap/envelope/" s:encodingStyle="http://schemas.xmlsoap.org/soap/encoding/"><s:Body><u:GetPositionInfo xmlns:u="urn:schemas-upnp-org:service:AVTransport:1"><InstanceID>0</InstanceID></u:GetPositionInfo></s:Body></s:Envelope>`,
		},
	}

	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			out, err := getPositionInfoSoapBuild()
			if err != nil {
				t.Fatalf("%s: Failed to call getPositionInfoSoapBuild due to %s", tc.name, err.Error())
			}
			if readerToString(out) != tc.want {
				t.Fatalf("%s: got: %s, want: %s.", tc.name, out, tc.want)
			}
		})
	}
}

func TestSeekSoapBuild(t *testing.T) {
	tt := []struct {
		name   string
		target string
		want   string
	}{
		{
			`seekSoapBuildTest #1`,
			"00:01:30",
			`<?xml version="1.0" encoding="utf-8"?><s:Envelope xmlns:s="http://schemas.xmlsoap.org/soap/envelope/" s:encodingStyle="http://schemas.xmlsoap.org/soap/encoding/"><s:Body><u:Seek xmlns:u="urn:schemas-upnp-org:service:AVTransport:1"><InstanceID>0</InstanceID><Unit>REL_TIME</Unit><Target>00:01:30</Target></u:Seek></s:Body></s:Envelope>`,
		},
	}

	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			out, err := seekSoapBuild(tc.target)
			if err != nil {
				t.Fatalf("%s: Failed to call seekSoapBuild due to %s", tc.name, err.Error())
			}
			if readerToString(out) != tc.want {
				t.Fatalf("%s: got: %s, want: %s.", tc.name, out, tc.want)
			}
		})
	}
}

func TestBuildURIMetadataPayload_MusicMetadata(t *testing.T) {
	media := &MediaItem{
		Title:           "Needed Time",
		Artist:          "Some Artist",
		Album:           "Some Album",
		AlbumArtURI:     "http://192.0.2.10:56736/art",
		TrackNumber:     7,
		URL:             "http://192.0.2.10:56736/stream",
		ContentType:     "audio/x-dsf",
		Seekable:        true,
		Duration:        313 * time.Second,
		Size:            221457176,
		Bitrate:         5645000,
		SampleFrequency: 2822400,
		BitsPerSample:   1,
		NrAudioChannels: 2,
	}

	contentFeatures, err := utils.BuildContentFeatures(media.ContentType, "01", false)
	if err != nil {
		t.Fatalf("BuildContentFeatures: %v", err)
	}

	want := `<DIDL-Lite xmlns="urn:schemas-upnp-org:metadata-1-0/DIDL-Lite/" xmlns:dc="http://purl.org/dc/elements/1.1/" xmlns:sec="http://www.sec.co.kr/" xmlns:upnp="urn:schemas-upnp-org:metadata-1-0/upnp/"><item id="1" parentID="0" restricted="1"><dc:title>Needed Time</dc:title><dc:creator>Some Artist</dc:creator><upnp:artist>Some Artist</upnp:artist><upnp:album>Some Album</upnp:album><upnp:originalTrackNumber>7</upnp:originalTrackNumber><upnp:albumArtURI>http://192.0.2.10:56736/art</upnp:albumArtURI><upnp:class>object.item.audioItem.musicTrack</upnp:class><res duration="00:05:13" size="221457176" bitrate="5645000" sampleFrequency="2822400" bitsPerSample="1" nrAudioChannels="2" protocolInfo="http-get:*:audio/x-dsf:` + contentFeatures + `">http://192.0.2.10:56736/stream</res></item></DIDL-Lite>`

	got, err := buildURIMetadataPayload(media)
	if err != nil {
		t.Fatalf("buildURIMetadataPayload: %v", err)
	}
	if string(got) != want {
		t.Fatalf("\ngot:  %s\nwant: %s", got, want)
	}
}

func TestBuildURIMetadataPayload_AudioMinimal(t *testing.T) {
	// When only the required fields are populated, no new metadata elements
	// or <res> audio attributes should appear (omitempty path).
	media := &MediaItem{
		Title:       "Just A Title",
		URL:         "http://192.0.2.10:56736/stream",
		ContentType: "audio/flac",
		Seekable:    true,
	}

	contentFeatures, err := utils.BuildContentFeatures(media.ContentType, "01", false)
	if err != nil {
		t.Fatalf("BuildContentFeatures: %v", err)
	}

	want := `<DIDL-Lite xmlns="urn:schemas-upnp-org:metadata-1-0/DIDL-Lite/" xmlns:dc="http://purl.org/dc/elements/1.1/" xmlns:sec="http://www.sec.co.kr/" xmlns:upnp="urn:schemas-upnp-org:metadata-1-0/upnp/"><item id="1" parentID="0" restricted="1"><dc:title>Just A Title</dc:title><upnp:class>object.item.audioItem.musicTrack</upnp:class><res protocolInfo="http-get:*:audio/flac:` + contentFeatures + `">http://192.0.2.10:56736/stream</res></item></DIDL-Lite>`

	got, err := buildURIMetadataPayload(media)
	if err != nil {
		t.Fatalf("buildURIMetadataPayload: %v", err)
	}
	if string(got) != want {
		t.Fatalf("\ngot:  %s\nwant: %s", got, want)
	}
}

func readerToString(r io.Reader) string {
	var buf strings.Builder
	io.Copy(&buf, r)
	return buf.String()
}
