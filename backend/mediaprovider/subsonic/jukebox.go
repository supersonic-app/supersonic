package subsonic

import (
	"fmt"
	"strconv"

	"github.com/supersonic-app/supersonic/backend/mediaprovider"
)

var _ mediaprovider.JukeboxProvider = (*subsonicMediaProvider)(nil)

func (s *subsonicMediaProvider) JukeboxStart() error {
	_, err := s.client.JukeboxControl("start", nil)
	return err
}

func (s *subsonicMediaProvider) JukeboxStop() error {
	_, err := s.client.JukeboxControl("stop", nil)
	return err
}

func (s *subsonicMediaProvider) JukeboxClear() error {
	_, err := s.client.JukeboxControl("clear", nil)
	return err
}

func (s *subsonicMediaProvider) JukeboxSetVolume(vol int) error {
	v := float64(vol) / 100
	_, err := s.client.JukeboxControl("setGain",
		map[string]string{"gain": fmt.Sprintf("%0.2f", v)})
	return err
}

func (s *subsonicMediaProvider) JukeboxSeek(idx, seconds int) error {
	_, err := s.client.JukeboxControl("skip",
		map[string]string{"index": strconv.Itoa(idx), "offset": strconv.Itoa(seconds)})
	return err
}

func (s *subsonicMediaProvider) JukeboxRemove(idx int) error {
	_, err := s.client.JukeboxControl("remove",
		map[string]string{"index": strconv.Itoa(idx)})
	return err
}

func (s *subsonicMediaProvider) JukeboxSet(trackID string) error {
	_, err := s.client.JukeboxControl("set",
		map[string]string{"id": trackID})
	return err
}

func (s *subsonicMediaProvider) JukeboxAdd(trackID string) error {
	_, err := s.client.JukeboxControl("add",
		map[string]string{"id": trackID})
	return err
}

func (s *subsonicMediaProvider) JukeboxGetStatus() (*mediaprovider.JukeboxStatus, error) {
	stat, err := s.client.JukeboxControl("status", nil)
	if err != nil {
		return nil, err
	}
	return &mediaprovider.JukeboxStatus{
		Volume:          int(stat.Gain * 100),
		CurrentTrack:    stat.CurrentIndex,
		Playing:         stat.Playing,
		PositionSeconds: float64(stat.Position),
	}, nil
}
