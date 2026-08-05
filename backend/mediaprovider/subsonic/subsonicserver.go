package subsonic

import (
	subsonicCli "github.com/supersonic-app/go-subsonic/subsonic"
	"github.com/supersonic-app/supersonic/backend/mediaprovider"
)

type SubsonicServer struct {
	subsonicCli.Client
}

func (s *SubsonicServer) Login(username, password string) mediaprovider.LoginResponse {
	s.User = username

	pr, err := s.Client.Ping()
	if err != nil {
		return mediaprovider.LoginResponse{
			Error:       err,
			IsAuthError: err == subsonicCli.ErrAuthenticationFailure,
		}
	}
	if pr.Type == "funkwhale" {
		// Funkwhale doesn't use correct types for IDs in the JSON response
		// and shows no progress in fixing this, so use the XML API
		s.Client.UseJSON = false
	}
	err = s.Client.Authenticate(password)
	return mediaprovider.LoginResponse{
		Error:       err,
		IsAuthError: err == subsonicCli.ErrAuthenticationFailure,
	}
}

func (s *SubsonicServer) MediaProvider() mediaprovider.MediaProvider {
	return SubsonicMediaProvider(&s.Client)
}
