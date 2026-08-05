package backend

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/20after4/configdir"
	"github.com/supersonic-app/supersonic/backend/mediaprovider"
)

const lrclibCacheFolder = "lrclib"

type LrcLibFetcher struct {
	cachePath       string
	customLrcLibUrl string
	timeout         time.Duration
}

func NewLrcLibFetcher(baseCacheDir string, customLrcLibUrl string, timeout time.Duration) *LrcLibFetcher {
	cachePath := filepath.Join(baseCacheDir, lrclibCacheFolder)
	configdir.MakePath(cachePath)
	return &LrcLibFetcher{cachePath: cachePath, customLrcLibUrl: customLrcLibUrl, timeout: timeout}
}

func (l *LrcLibFetcher) FetchLrcLibLyrics(name, artist, album string, durationSecs int) (*mediaprovider.Lyrics, error) {
	hash := makeTrackIdHash(name, artist, album, durationSecs)
	cacheFilePath := filepath.Join(l.cachePath, fmt.Sprintf("%s.txt", hash))

	// File is cached. Try to use it
	if _, err := os.Stat(cacheFilePath); err == nil {
		lyrics, err := readCachedLyrics(cacheFilePath)
		if err == nil {
			return lyrics, nil
		}

		// On an error, remove the file.
		if !os.IsNotExist(err) {
			os.Remove(cacheFilePath)
		}
	}

	// Fetch the lyrics
	lyrics, err := l.fetchFromServer(name, artist, album, durationSecs)
	if err != nil {
		return nil, err
	}

	// Try to write it into cache
	err = writeCachedLyrics(cacheFilePath, lyrics)
	if err != nil {
		log.Printf("Failed to serialize fetched lyrics: %s", err)
	}

	return lyrics, nil
}

func (l *LrcLibFetcher) fetchFromServer(name, artist, album string, durationSecs int) (*mediaprovider.Lyrics, error) {
	ctx, cancel := context.WithTimeout(context.Background(), l.timeout)
	defer cancel()

	lrclibUrl := l.getLrclibUrl()
	fmt.Println(lrclibUrl)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, lrclibUrl, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Add("Accept", "application/json")
	req.Header.Add("User-Agent", "Supersonic")

	// Navidrome and Gonic substitute "[Unknown Album]" and "Unknown Album", respectively,
	// for an empty album name. This will break LrcLib matching.
	// TODO: if OpenSubsonic later clarifies that servers should not do this, remove this workaround.
	// N.B.: This workaround will break if servers decide to internationalize the default album name
	if strings.Contains(album, "Unknown Album") {
		album = ""
	}
	if strings.Contains(artist, "Unknown Artist") {
		artist = ""
	}

	q := req.URL.Query()
	addIf := func(key, value string) {
		if value != "" {
			q.Add(key, value)
		}
	}
	q.Add("track_name", name)
	addIf("artist_name", artist)
	addIf("album_name", album)
	q.Add("duration", strconv.Itoa(durationSecs))
	req.URL.RawQuery = q.Encode()

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	return parseLrcLibResponse(resp)
}

// Returns the Lrclib url the fetcher should use, based on the configuration.
func (l *LrcLibFetcher) getLrclibUrl() string {
	if l.customLrcLibUrl != "" {
		u, err := url.JoinPath(l.customLrcLibUrl, "/api/get")
		if err == nil {
			// Return custom cunfig url
			return u
		}

		log.Printf("Invalid LrcLib URL passed (err: %s). Falling back to default", err)
	}

	return "https://lrclib.net/api/get"
}

func parseLrcLibResponse(resp *http.Response) (*mediaprovider.Lyrics, error) {
	if resp.StatusCode == http.StatusNotFound {
		return nil, errors.New("lrclib lyrics not found")
	} else if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("error from lrclib: status %d", resp.StatusCode)
	}

	var parsedResponse lrcLibResponse
	if err := json.NewDecoder(resp.Body).Decode(&parsedResponse); err != nil {
		return nil, fmt.Errorf("failed to decode lrclib response: %w", err)
	}
	lrcs := &mediaprovider.Lyrics{
		Title:  parsedResponse.TrackName,
		Artist: parsedResponse.ArtistName,
	}
	if parsedResponse.SyncedLyrics != "" {
		lines, err := parseSyncedLyrics(parsedResponse.SyncedLyrics)
		if err != nil {
			return nil, err
		}
		lrcs.Synced = true
		lrcs.Lines = lines
	} else {
		for _, line := range strings.Split(parsedResponse.PlainLyrics, "\n") {
			lrcs.Lines = append(lrcs.Lines, mediaprovider.LyricLine{Text: line})
		}
	}
	return lrcs, nil
}

var syncedRegex = regexp.MustCompile(`^\[(\d\d):(\d\d\.\d\d\d?)\] ?(.+)$`)

func parseSyncedLyrics(synced string) ([]mediaprovider.LyricLine, error) {
	var lines []mediaprovider.LyricLine
	for _, line := range strings.Split(synced, "\n") {
		matches := syncedRegex.FindStringSubmatch(line)
		if len(matches) != 4 {
			continue // malformed lyric line, attempt to continue
		}
		min, _ := strconv.Atoi(matches[1])
		sec, _ := strconv.ParseFloat(matches[2], 64)
		lines = append(lines, mediaprovider.LyricLine{
			Start: float64(min)*60 + sec,
			Text:  matches[3],
		})
	}
	var err error
	if len(lines) == 0 {
		err = errors.New("failed to parse synced lyrics")
	}
	return lines, err
}

type lrcLibResponse struct {
	ID           int     `json:"id"`
	TrackName    string  `json:"trackName"`
	ArtistName   string  `json:"artistName"`
	AlbumName    string  `json:"albumName"`
	Duration     float64 `json:"duration"`
	Instrumental bool    `json:"instrumental"`
	PlainLyrics  string  `json:"plainLyrics"`
	SyncedLyrics string  `json:"syncedLyrics"`
}

// Write lyrics to the given file.
func writeCachedLyrics(cacheFile string, lyrics *mediaprovider.Lyrics) error {
	serialized, err := json.Marshal(lyrics)
	if err != nil {
		return err
	}

	f, err := os.Create(cacheFile)
	if err != nil {
		return nil
	}
	defer f.Close()

	f.Write(serialized)

	return nil
}

// Read lyrics from the given cache file.
func readCachedLyrics(cacheFile string) (*mediaprovider.Lyrics, error) {
	cachedBytes, err := os.ReadFile(cacheFile)
	if err != nil {
		return nil, err
	}

	var lyrics mediaprovider.Lyrics

	err = json.Unmarshal(cachedBytes, &lyrics)
	if err != nil {
		return nil, err
	}

	return &lyrics, nil
}

// Create a "unique" hash for a song to identify it.
func makeTrackIdHash(name, artist, album string, durationSecs int) string {
	hasher := md5.New()
	identifier := fmt.Sprintf("%s;%s;%s;%d", name, artist, album, durationSecs)
	hasher.Write([]byte(identifier))
	return hex.EncodeToString(hasher.Sum(nil))
}
