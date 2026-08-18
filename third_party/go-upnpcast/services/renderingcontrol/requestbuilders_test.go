package renderingcontrol

import (
	"io"
	"strings"
	"testing"
)

func TestSetMuteSoapBuild(t *testing.T) {
	tt := []struct {
		name  string
		input bool
		want  string
	}{
		{
			`setMuteSoapBuild Test #1`,
			true,
			`<?xml version="1.0" encoding="utf-8"?><s:Envelope xmlns:s="http://schemas.xmlsoap.org/soap/envelope/" s:encodingStyle="http://schemas.xmlsoap.org/soap/encoding/"><s:Body><u:SetMute xmlns:u="urn:schemas-upnp-org:service:RenderingControl:1"><InstanceID>0</InstanceID><Channel>Master</Channel><DesiredMute>1</DesiredMute></u:SetMute></s:Body></s:Envelope>`,
		},
		{
			`setMuteSoapBuild Test #2`,
			false,
			`<?xml version="1.0" encoding="utf-8"?><s:Envelope xmlns:s="http://schemas.xmlsoap.org/soap/envelope/" s:encodingStyle="http://schemas.xmlsoap.org/soap/encoding/"><s:Body><u:SetMute xmlns:u="urn:schemas-upnp-org:service:RenderingControl:1"><InstanceID>0</InstanceID><Channel>Master</Channel><DesiredMute>0</DesiredMute></u:SetMute></s:Body></s:Envelope>`,
		},
	}

	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			out, err := setMuteSoapBuild(tc.input)
			if err != nil {
				t.Fatalf("%s: Failed to call setMuteSoapBuild due to %s", tc.name, err.Error())
			}
			if readerToString(out) != tc.want {
				t.Fatalf("%s: got: %s, want: %s.", tc.name, out, tc.want)
			}
		})
	}
}

func TestGetVolumeSoapBuild(t *testing.T) {
	tt := []struct {
		name string
		want string
	}{
		{
			`getVolumeSoapBuild Test #1`,
			`<?xml version="1.0" encoding="utf-8"?><s:Envelope xmlns:s="http://schemas.xmlsoap.org/soap/envelope/" s:encodingStyle="http://schemas.xmlsoap.org/soap/encoding/"><s:Body><u:GetVolume xmlns:u="urn:schemas-upnp-org:service:RenderingControl:1"><InstanceID>0</InstanceID><Channel>Master</Channel></u:GetVolume></s:Body></s:Envelope>`,
		},
	}

	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			out, err := getVolumeSoapBuild()
			if err != nil {
				t.Fatalf("%s: Failed to call setMuteSoapBuild due to %s", tc.name, err.Error())
			}
			if readerToString(out) != tc.want {
				t.Fatalf("%s: got: %s, want: %s.", tc.name, out, tc.want)
			}
		})
	}
}

func TestGetMuteSoapBuild(t *testing.T) {
	tt := []struct {
		name string
		want string
	}{
		{
			`getMuteSoapBuild Test #1`,
			`<?xml version="1.0" encoding="utf-8"?><s:Envelope xmlns:s="http://schemas.xmlsoap.org/soap/envelope/" s:encodingStyle="http://schemas.xmlsoap.org/soap/encoding/"><s:Body><u:GetMute xmlns:u="urn:schemas-upnp-org:service:RenderingControl:1"><InstanceID>0</InstanceID><Channel>Master</Channel></u:GetMute></s:Body></s:Envelope>`,
		},
	}

	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			out, err := getMuteSoapBuild()
			if err != nil {
				t.Fatalf("%s: Failed to call getMuteSoapBuild due to %s", tc.name, err.Error())
			}
			if readerToString(out) != tc.want {
				t.Fatalf("%s: got: %s, want: %s.", tc.name, out, tc.want)
			}
		})
	}
}

func TestSetVolumeSoapBuild(t *testing.T) {
	tt := []struct {
		name   string
		intput string
		want   string
	}{
		{
			`setVolumeSoapBuild Test #1`,
			`100`,
			`<?xml version="1.0" encoding="utf-8"?><s:Envelope xmlns:s="http://schemas.xmlsoap.org/soap/envelope/" s:encodingStyle="http://schemas.xmlsoap.org/soap/encoding/"><s:Body><u:SetVolume xmlns:u="urn:schemas-upnp-org:service:RenderingControl:1"><InstanceID>0</InstanceID><Channel>Master</Channel><DesiredVolume>100</DesiredVolume></u:SetVolume></s:Body></s:Envelope>`,
		},
	}

	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			out, err := setVolumeSoapBuild(tc.intput)
			if err != nil {
				t.Fatalf("%s: Failed to call setVolumeSoapBuild due to %s", tc.name, err.Error())
			}
			if readerToString(out) != tc.want {
				t.Fatalf("%s: got: %s, want: %s.", tc.name, out, tc.want)
			}
		})
	}
}

func readerToString(r io.Reader) string {
	var buf strings.Builder
	io.Copy(&buf, r)
	return buf.String()
}
