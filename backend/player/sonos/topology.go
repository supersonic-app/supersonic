// Package sonos resolves the zone group topology of a Sonos system.
//
// Sonos players are ordinary UPnP MediaRenderers, but each one advertises
// itself individually - including the halves of a stereo pair and the
// satellites of a home theater set, which a user never addresses on its
// own. The ZoneGroupTopology service describes how the players are really
// arranged, which of them are hidden, and which one coordinates playback
// for the rest. Casting has to be driven through that coordinator.
package sonos

import (
	"context"
	"encoding/xml"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/supersonic-app/go-upnpcast/device"
)

const (
	topologyServiceType = "urn:schemas-upnp-org:service:ZoneGroupTopology:1"
	topologyControlPath = "/ZoneGroupTopology/Control"

	// Sonos players always serve their UPnP endpoints on port 1400.
	sonosPort = "1400"

	getZoneGroupStateBody = `<?xml version="1.0" encoding="utf-8"?>` +
		`<s:Envelope xmlns:s="http://schemas.xmlsoap.org/soap/envelope/"` +
		` s:encodingStyle="http://schemas.xmlsoap.org/soap/encoding/"><s:Body>` +
		`<u:GetZoneGroupState xmlns:u="` + topologyServiceType + `"></u:GetZoneGroupState>` +
		`</s:Body></s:Envelope>`
)

// A Group is a set of Sonos players that plays as a unit: a lone speaker,
// a bonded stereo pair or home theater set, or several rooms the user has
// grouped together. Playback is driven through Coordinator.
type Group struct {
	Name        string
	Coordinator *device.MediaRenderer
}

// Groups sorts discovered renderers into the Sonos zone groups they belong
// to, and the renderers that are not part of a Sonos system. Every Sonos
// player is claimed by the topology, so a player that is not separately
// addressable is dropped rather than returned as a renderer of its own.
//
// If no Sonos system answers, every renderer is returned as an "other".
func Groups(ctx context.Context, renderers []*device.MediaRenderer) (groups []Group, others []*device.MediaRenderer) {
	// The topology covers the whole household, so the first player to
	// answer describes every other one.
	var state zoneGroupState
	for _, r := range renderers {
		if !mayBeSonos(r) {
			continue
		}
		if s, err := getZoneGroupState(ctx, r.URL); err == nil {
			state = s
			break
		}
	}
	return state.group(renderers)
}

// group matches the topology against the renderers that were actually
// discovered, splitting them into playable zone groups and non-Sonos
// leftovers.
func (s zoneGroupState) group(renderers []*device.MediaRenderer) (groups []Group, others []*device.MediaRenderer) {
	byLocation := make(map[string]*device.MediaRenderer, len(renderers))
	for _, r := range renderers {
		byLocation[r.URL] = r
	}

	claimed := make(map[string]bool)
	for _, g := range s.Groups {
		var visible []zoneGroupMember
		for _, m := range g.members() {
			claimed[m.Location] = true
			if !m.hidden() {
				visible = append(visible, m)
			}
		}

		coordinator, ok := byLocation[g.coordinatorLocation()]
		if !ok || len(visible) == 0 {
			continue // not reachable, or nothing the user would target
		}
		groups = append(groups, Group{
			Name:        groupName(g.zoneName(), len(visible)),
			Coordinator: coordinator,
		})
	}
	slices.SortFunc(groups, func(a, b Group) int { return strings.Compare(a.Name, b.Name) })

	for _, r := range renderers {
		if !claimed[r.URL] {
			others = append(others, r)
		}
	}
	return groups, others
}

// groupName names a zone group the way the Sonos app does: the
// coordinator's room, plus a count of the other rooms playing along.
func groupName(zoneName string, visibleMembers int) string {
	if visibleMembers < 2 {
		return zoneName
	}
	return fmt.Sprintf("%s + %d", zoneName, visibleMembers-1)
}

func mayBeSonos(d *device.MediaRenderer) bool {
	u, err := url.Parse(d.URL)
	return err == nil && u.Port() == sonosPort
}

func getZoneGroupState(ctx context.Context, deviceURL string) (zoneGroupState, error) {
	u, err := url.Parse(deviceURL)
	if err != nil {
		return zoneGroupState{}, err
	}
	controlURL := u.Scheme + "://" + u.Host + topologyControlPath

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, controlURL,
		strings.NewReader(getZoneGroupStateBody))
	if err != nil {
		return zoneGroupState{}, err
	}
	req.Header.Set("Content-Type", `text/xml; charset="utf-8"`)
	req.Header.Set("SOAPAction", `"`+topologyServiceType+`#GetZoneGroupState"`)
	req.Header.Set("Connection", "close")

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return zoneGroupState{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return zoneGroupState{}, fmt.Errorf("zone group topology returned %s", resp.Status)
	}

	// The state is an XML document escaped into the SOAP response.
	var envelope zoneGroupStateEnvelope
	if err := xml.NewDecoder(resp.Body).Decode(&envelope); err != nil {
		return zoneGroupState{}, err
	}
	var state zoneGroupState
	if err := xml.Unmarshal([]byte(envelope.State), &state); err != nil {
		return zoneGroupState{}, err
	}
	return state, nil
}

type zoneGroupStateEnvelope struct {
	XMLName xml.Name `xml:"Envelope"`
	State   string   `xml:"Body>GetZoneGroupStateResponse>ZoneGroupState"`
}

type zoneGroupState struct {
	XMLName xml.Name    `xml:"ZoneGroupState"`
	Groups  []zoneGroup `xml:"ZoneGroups>ZoneGroup"`
}

type zoneGroup struct {
	Coordinator string            `xml:"Coordinator,attr"`
	Members     []zoneGroupMember `xml:"ZoneGroupMember"`
}

type zoneGroupMember struct {
	UUID      string `xml:"UUID,attr"`
	ZoneName  string `xml:"ZoneName,attr"`
	Location  string `xml:"Location,attr"`
	Invisible string `xml:"Invisible,attr"`

	// Satellites are the surrounds and subwoofer bonded to a home
	// theater player. They are hidden members of their host's zone.
	Satellites []zoneGroupMember `xml:"Satellite"`
}

// members flattens the group's players, satellites included.
func (g zoneGroup) members() []zoneGroupMember {
	var all []zoneGroupMember
	for _, m := range g.Members {
		all = append(all, m)
		all = append(all, m.Satellites...)
	}
	return all
}

func (g zoneGroup) coordinator() (zoneGroupMember, bool) {
	for _, m := range g.Members {
		if m.UUID == g.Coordinator {
			return m, true
		}
	}
	return zoneGroupMember{}, false
}

func (g zoneGroup) coordinatorLocation() string {
	c, _ := g.coordinator()
	return c.Location
}

func (g zoneGroup) zoneName() string {
	c, _ := g.coordinator()
	return c.ZoneName
}

// hidden reports whether the player is bonded to another one - the second
// speaker of a stereo pair, or a home theater satellite - and so is not
// something the user can play to on its own.
func (m zoneGroupMember) hidden() bool {
	return m.Invisible == "1"
}
