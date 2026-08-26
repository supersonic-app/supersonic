package sonos

import (
	"encoding/xml"
	"testing"

	"github.com/supersonic-app/go-upnpcast/device"
)

// Captured from a household with a standalone Era 300 ("Office") and a
// stereo pair of Sonos Ones ("Bedroom"), whose right speaker coordinates
// the pair and whose left speaker is invisible.
const pairAndStandaloneState = `<ZoneGroupState><ZoneGroups>` +
	`<ZoneGroup Coordinator="RINCON_ERA300" ID="RINCON_ERA300:1001672193">` +
	`<ZoneGroupMember UUID="RINCON_ERA300" Location="http://192.168.1.178:1400/xml/device_description.xml" ZoneName="Office"/>` +
	`</ZoneGroup>` +
	`<ZoneGroup Coordinator="RINCON_ONE_RF" ID="RINCON_ONE_RF:2220472879">` +
	`<ZoneGroupMember UUID="RINCON_ONE_RF" Location="http://192.168.1.111:1400/xml/device_description.xml" ZoneName="Bedroom" ChannelMapSet="RINCON_ONE_RF:RF,RF;RINCON_ONE_LF:LF,LF"/>` +
	`<ZoneGroupMember UUID="RINCON_ONE_LF" Location="http://192.168.1.233:1400/xml/device_description.xml" ZoneName="Bedroom" Invisible="1" ChannelMapSet="RINCON_ONE_RF:RF,RF;RINCON_ONE_LF:LF,LF"/>` +
	`</ZoneGroup>` +
	`</ZoneGroups><VanishedDevices></VanishedDevices></ZoneGroupState>`

func renderers(urls ...string) []*device.MediaRenderer {
	var r []*device.MediaRenderer
	for _, u := range urls {
		r = append(r, &device.MediaRenderer{URL: u, FriendlyName: u})
	}
	return r
}

func parseState(t *testing.T, s string) zoneGroupState {
	t.Helper()
	var state zoneGroupState
	if err := xml.Unmarshal([]byte(s), &state); err != nil {
		t.Fatalf("failed to parse zone group state: %v", err)
	}
	return state
}

func TestGroupStereoPairAndStandalone(t *testing.T) {
	state := parseState(t, pairAndStandaloneState)
	discovered := renderers(
		"http://192.168.1.233:1400/xml/device_description.xml",
		"http://192.168.1.111:1400/xml/device_description.xml",
		"http://192.168.1.178:1400/xml/device_description.xml",
		"http://192.168.1.50:8200/rootDesc.xml", // a non-Sonos renderer
	)

	groups, others := state.group(discovered)

	if len(groups) != 2 {
		t.Fatalf("expected 2 zone groups, got %d: %+v", len(groups), groups)
	}
	if groups[0].Name != "Bedroom" {
		t.Errorf("expected the stereo pair to be named Bedroom, got %q", groups[0].Name)
	}
	// The pair must be driven through its coordinator, not the invisible speaker.
	if want := "http://192.168.1.111:1400/xml/device_description.xml"; groups[0].Coordinator.URL != want {
		t.Errorf("expected coordinator %q, got %q", want, groups[0].Coordinator.URL)
	}
	if groups[1].Name != "Office" {
		t.Errorf("expected the standalone speaker to be named Office, got %q", groups[1].Name)
	}

	if len(others) != 1 || others[0].URL != "http://192.168.1.50:8200/rootDesc.xml" {
		t.Errorf("expected only the non-Sonos renderer to be left over, got %+v", others)
	}
}

func TestGroupNamesGroupedRooms(t *testing.T) {
	state := parseState(t, `<ZoneGroupState><ZoneGroups>`+
		`<ZoneGroup Coordinator="A" ID="A:1">`+
		`<ZoneGroupMember UUID="A" Location="http://a:1400/d.xml" ZoneName="Kitchen"/>`+
		`<ZoneGroupMember UUID="B" Location="http://b:1400/d.xml" ZoneName="Patio"/>`+
		`<ZoneGroupMember UUID="C" Location="http://c:1400/d.xml" ZoneName="Den"/>`+
		`</ZoneGroup></ZoneGroups></ZoneGroupState>`)

	groups, others := state.group(renderers("http://a:1400/d.xml", "http://b:1400/d.xml", "http://c:1400/d.xml"))

	if len(groups) != 1 {
		t.Fatalf("expected grouped rooms to collapse into 1 entry, got %d", len(groups))
	}
	if groups[0].Name != "Kitchen + 2" {
		t.Errorf(`expected "Kitchen + 2", got %q`, groups[0].Name)
	}
	if len(others) != 0 {
		t.Errorf("expected no leftover renderers, got %+v", others)
	}
}

func TestGroupHomeTheaterSatellitesAreHidden(t *testing.T) {
	state := parseState(t, `<ZoneGroupState><ZoneGroups>`+
		`<ZoneGroup Coordinator="ARC" ID="ARC:1">`+
		`<ZoneGroupMember UUID="ARC" Location="http://arc:1400/d.xml" ZoneName="Living Room">`+
		`<Satellite UUID="SUB" Location="http://sub:1400/d.xml" ZoneName="Living Room" Invisible="1"/>`+
		`<Satellite UUID="SURR" Location="http://surr:1400/d.xml" ZoneName="Living Room" Invisible="1"/>`+
		`</ZoneGroupMember></ZoneGroup></ZoneGroups></ZoneGroupState>`)

	groups, others := state.group(renderers("http://arc:1400/d.xml", "http://sub:1400/d.xml", "http://surr:1400/d.xml"))

	if len(groups) != 1 || groups[0].Name != "Living Room" {
		t.Fatalf(`expected a single "Living Room" entry, got %+v`, groups)
	}
	if len(others) != 0 {
		t.Errorf("expected satellites to be claimed by the topology, got %+v", others)
	}
}

// A Sonos player that answers SSDP but is missing from the topology (or
// whose coordinator is) must not fall through to the plain DLNA list.
func TestGroupSkipsUnreachableCoordinator(t *testing.T) {
	state := parseState(t, `<ZoneGroupState><ZoneGroups>`+
		`<ZoneGroup Coordinator="A" ID="A:1">`+
		`<ZoneGroupMember UUID="A" Location="http://a:1400/d.xml" ZoneName="Kitchen"/>`+
		`<ZoneGroupMember UUID="B" Location="http://b:1400/d.xml" ZoneName="Patio"/>`+
		`</ZoneGroup></ZoneGroups></ZoneGroupState>`)

	groups, others := state.group(renderers("http://b:1400/d.xml"))

	if len(groups) != 0 {
		t.Errorf("expected no playable groups without the coordinator, got %+v", groups)
	}
	if len(others) != 0 {
		t.Errorf("expected the Sonos member to stay claimed, got %+v", others)
	}
}

func TestGroupWithoutSonosLeavesRenderersAlone(t *testing.T) {
	groups, others := zoneGroupState{}.group(renderers("http://tv:8200/rootDesc.xml"))

	if len(groups) != 0 {
		t.Errorf("expected no groups, got %+v", groups)
	}
	if len(others) != 1 {
		t.Fatalf("expected the renderer to be passed through, got %+v", others)
	}
}
