package backend

import (
	"context"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// A component to check for updates from the
// Github releases/latest URL.
type UpdateChecker struct {
	OnUpdatedVersionFound func()

	versionTagFound  string
	latestReleaseURL string
	appVersionTag    string
	lastCheckedTag   *string
	httpClient       *http.Client
}

func NewUpdateChecker(appVersionTag, latestReleaseURL string, lastCheckedTag *string, httpClient *http.Client) UpdateChecker {
	return UpdateChecker{
		appVersionTag:    appVersionTag,
		latestReleaseURL: latestReleaseURL,
		lastCheckedTag:   lastCheckedTag,
		httpClient:       httpClient,
	}
}

// Start automatically polling for updates in the background.
// Quits when the given ctx's Done channel returns a value.
func (u *UpdateChecker) Start(ctx context.Context, interval time.Duration) {
	go func() {
		u.checkForUpdate() // check once at startup
		t := time.NewTicker(interval)
		for {
			select {
			case <-ctx.Done():
				t.Stop()
				return
			case <-t.C:
				u.checkForUpdate()
			}
		}
	}()
}

func (u *UpdateChecker) VersionTagFound() string {
	return u.versionTagFound
}

func (u *UpdateChecker) LatestReleaseURL() *url.URL {
	url, _ := url.Parse(u.latestReleaseURL)
	return url
}

func (u *UpdateChecker) checkForUpdate() {
	t := u.CheckLatestVersionTag()
	if t != "" && t != *u.lastCheckedTag {
		u.versionTagFound = t
		if u.OnUpdatedVersionFound != nil {
			u.OnUpdatedVersionFound()
		}
	}
}

func (u *UpdateChecker) CheckLatestVersionTag() string {
	resp, err := u.httpClient.Head(u.latestReleaseURL)
	if err != nil {
		log.Printf("failed to check for newest version: %s", err.Error())
		return ""
	}
	defer resp.Body.Close()
	url := resp.Request.URL.String()
	url = strings.TrimSuffix(url, "/")
	idx := strings.LastIndex(url, "/")
	if idx >= len(url)-1 {
		return ""
	}
	return url[idx+1:]
}
