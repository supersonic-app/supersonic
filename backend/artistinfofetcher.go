package backend

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/dweymouth/supersonic/backend/mediaprovider"
)

const (
	deezerBaseURL      = "https://api.deezer.com"
	artistInfoTimeout  = 10 * time.Second
	artistInfoCacheTTL = 24 * time.Hour
	// Maximum number of concurrent external artist-info HTTP requests.
	// Prevents thundering-herd when a full grid of artists loads at once.
	maxConcurrentArtistFetches = 4
)

// artistInfoCacheEntry stores cached artist info with expiration.
type artistInfoCacheEntry struct {
	info      *mediaprovider.ArtistInfo
	expiresAt time.Time
}

// inflightCall tracks an in-progress fetch so that concurrent callers for the
// same artist can wait on the same result instead of all issuing HTTP requests.
type inflightCall struct {
	done chan struct{}
	info *mediaprovider.ArtistInfo
	err  error
}

// ArtistInfoFetcher fetches artist info from Deezer (images) and Wikipedia (biography).
// It can be used as a fallback for any media server that doesn't provide complete
// artist metadata (biography, images, etc.).
type ArtistInfoFetcher struct {
	httpClient *http.Client
	cache      map[string]artistInfoCacheEntry
	cacheMu    sync.RWMutex
	language   string // User's preferred language code (e.g., "it", "de", "fr")

	// inflight deduplicates concurrent requests for the same artist.
	inflightMu sync.Mutex
	inflight   map[string]*inflightCall

	// fetchSem limits the number of simultaneous outbound HTTP requests.
	fetchSem chan struct{}
}

// NewArtistInfoFetcher creates a new ArtistInfoFetcher.
// The language parameter sets the preferred Wikipedia language (e.g. "it", "de").
// Use "" or "auto" to auto-detect from system locale.
func NewArtistInfoFetcher(language string) *ArtistInfoFetcher {
	return &ArtistInfoFetcher{
		httpClient: &http.Client{
			Timeout: artistInfoTimeout,
		},
		cache:    make(map[string]artistInfoCacheEntry),
		inflight: make(map[string]*inflightCall),
		fetchSem: make(chan struct{}, maxConcurrentArtistFetches),
		language: language,
	}
}

// ClearCache clears the in-memory artist info cache.
func (f *ArtistInfoFetcher) ClearCache() {
	f.cacheMu.Lock()
	defer f.cacheMu.Unlock()
	f.cache = make(map[string]artistInfoCacheEntry)
}

// FetchArtistInfo fetches artist info from Deezer (images) and Wikipedia (biography) with caching.
// Concurrent calls for the same artist are deduplicated — only one HTTP round-trip is made.
// Overall concurrency is bounded by maxConcurrentArtistFetches to avoid hammering external APIs.
func (f *ArtistInfoFetcher) FetchArtistInfo(artistName string) (*mediaprovider.ArtistInfo, error) {
	if artistName == "" {
		return &mediaprovider.ArtistInfo{}, nil
	}

	cacheKey := strings.ToLower(artistName)

	// Fast path: cache hit (read-lock only).
	f.cacheMu.RLock()
	if entry, ok := f.cache[cacheKey]; ok && time.Now().Before(entry.expiresAt) {
		f.cacheMu.RUnlock()
		return entry.info, nil
	}
	f.cacheMu.RUnlock()

	// Deduplicate in-flight requests for the same artist.
	f.inflightMu.Lock()
	if call, ok := f.inflight[cacheKey]; ok {
		// Another goroutine is already fetching this artist — wait for it.
		f.inflightMu.Unlock()
		<-call.done
		return call.info, call.err
	}
	call := &inflightCall{done: make(chan struct{})}
	f.inflight[cacheKey] = call
	f.inflightMu.Unlock()

	// Acquire semaphore slot to bound total concurrent HTTP requests.
	f.fetchSem <- struct{}{}
	defer func() { <-f.fetchSem }()

	// Do the actual fetch.
	info, err := f.fetchFromDeezer(artistName)
	if err != nil {
		info = &mediaprovider.ArtistInfo{}
	}

	biography, wikiURL := f.fetchBiographyFromWikipedia(artistName)
	if biography != "" {
		info.Biography = biography
		if wikiURL != "" && info.LastFMUrl == "" {
			info.LastFMUrl = wikiURL
		}
	}

	// Store in cache.
	f.cacheMu.Lock()
	f.cache[cacheKey] = artistInfoCacheEntry{
		info:      info,
		expiresAt: time.Now().Add(artistInfoCacheTTL),
	}
	f.cacheMu.Unlock()

	// Publish result and unblock waiters.
	call.info = info
	call.err = nil // Deezer/Wikipedia failures are swallowed; result is always non-nil.
	close(call.done)

	// Remove from in-flight map.
	f.inflightMu.Lock()
	delete(f.inflight, cacheKey)
	f.inflightMu.Unlock()

	return info, nil
}

// fetchFromDeezer fetches artist info from Deezer API.
func (f *ArtistInfoFetcher) fetchFromDeezer(artistName string) (*mediaprovider.ArtistInfo, error) {
	reqURL := fmt.Sprintf("%s/search/artist?q=%s", deezerBaseURL, url.QueryEscape(artistName))

	req, err := http.NewRequest(http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "Supersonic/1.0 (https://github.com/dweymouth/supersonic)")

	resp, err := f.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch artist info: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("artist info fetch failed with status %d", resp.StatusCode)
	}

	var result deezerSearchResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	if result.Error.Code != 0 {
		return nil, fmt.Errorf("deezer API error %d: %s", result.Error.Code, result.Error.Message)
	}

	if len(result.Data) == 0 {
		return &mediaprovider.ArtistInfo{}, nil
	}

	// Find best match (exact name match preferred)
	var bestMatch *deezerArtist
	artistLower := strings.ToLower(artistName)
	for i := range result.Data {
		if strings.ToLower(result.Data[i].Name) == artistLower {
			bestMatch = &result.Data[i]
			break
		}
	}
	if bestMatch == nil {
		bestMatch = &result.Data[0]
	}

	info := &mediaprovider.ArtistInfo{}

	// Prefer XL image, fall back to big, then medium
	if bestMatch.PictureXL != "" && !isDeezerPlaceholder(bestMatch.PictureXL) {
		info.ImageURL = bestMatch.PictureXL
	} else if bestMatch.PictureBig != "" && !isDeezerPlaceholder(bestMatch.PictureBig) {
		info.ImageURL = bestMatch.PictureBig
	} else if bestMatch.PictureMedium != "" && !isDeezerPlaceholder(bestMatch.PictureMedium) {
		info.ImageURL = bestMatch.PictureMedium
	}

	if bestMatch.Link != "" {
		info.LastFMUrl = bestMatch.Link
	}

	return info, nil
}

// isDeezerPlaceholder checks if the URL is a Deezer default placeholder image.
func isDeezerPlaceholder(imgURL string) bool {
	return strings.Contains(imgURL, "/artist//") || strings.Contains(imgURL, "d-artist")
}

// fetchBiographyFromWikipedia fetches artist biography from Wikipedia.
// It tries the user's preferred language first, then always falls back to English.
func (f *ArtistInfoFetcher) fetchBiographyFromWikipedia(artistName string) (string, string) {
	langs := []string{"en"}

	lang := f.language
	if lang == "" || lang == "auto" {
		lang = getSystemLanguage()
	}

	if lang != "" && lang != "en" {
		wikiLang := mapToWikipediaLang(lang)
		if wikiLang != "" && wikiLang != "en" {
			langs = []string{wikiLang, "en"}
		}
	}

	for _, l := range langs {
		extract, pageURL := f.fetchWikipediaBio(artistName, l)
		if extract != "" {
			return extract, pageURL
		}
	}

	return "", ""
}

// getSystemLanguage detects the system language from environment variables.
func getSystemLanguage() string {
	for _, envVar := range []string{"LANG", "LC_MESSAGES", "LC_ALL", "LANGUAGE"} {
		if val := os.Getenv(envVar); val != "" {
			lang := strings.Split(val, "_")[0]
			lang = strings.Split(lang, ".")[0]
			if lang != "" && lang != "C" && lang != "POSIX" {
				return lang
			}
		}
	}
	return ""
}

// mapToWikipediaLang maps app language codes to Wikipedia language codes.
func mapToWikipediaLang(appLang string) string {
	switch appLang {
	case "zhHans", "zhHant", "zh":
		return "zh"
	case "pt_BR":
		return "pt"
	default:
		return appLang
	}
}

// isValidLangCode checks that a language code is safe to use in a URL hostname.
var validLangCode = regexp.MustCompile(`^[a-z]{2,3}$`)

func isValidLangCode(code string) bool {
	return validLangCode.MatchString(code)
}

// fetchWikipediaBio fetches biography from a specific Wikipedia language edition.
func (f *ArtistInfoFetcher) fetchWikipediaBio(artistName, wikiLang string) (string, string) {
	// Validate language code to prevent URL injection
	if !isValidLangCode(wikiLang) {
		return "", ""
	}

	title := strings.ReplaceAll(artistName, " ", "_")
	reqURL := fmt.Sprintf("https://%s.wikipedia.org/api/rest_v1/page/summary/%s", wikiLang, url.PathEscape(title))

	req, err := http.NewRequest(http.MethodGet, reqURL, nil)
	if err != nil {
		return "", ""
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "Supersonic/1.0 (https://github.com/dweymouth/supersonic)")

	resp, err := f.httpClient.Do(req)
	if err != nil {
		return "", ""
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", ""
	}

	var result wikipediaSummary
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", ""
	}

	pageURL := ""
	if result.ContentURLs.Desktop.Page != "" {
		pageURL = result.ContentURLs.Desktop.Page
	}

	return result.Extract, pageURL
}

// Deezer API response types

type deezerSearchResponse struct {
	Data  []deezerArtist `json:"data"`
	Error deezerError    `json:"error"`
}

type deezerError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type deezerArtist struct {
	ID            int    `json:"id"`
	Name          string `json:"name"`
	Link          string `json:"link"`
	PictureSmall  string `json:"picture_small"`
	PictureMedium string `json:"picture_medium"`
	PictureBig    string `json:"picture_big"`
	PictureXL     string `json:"picture_xl"`
	NbFan         int    `json:"nb_fan"`
}

// Wikipedia API response types

type wikipediaSummary struct {
	Title       string `json:"title"`
	Extract     string `json:"extract"`
	ContentURLs struct {
		Desktop struct {
			Page string `json:"page"`
		} `json:"desktop"`
	} `json:"content_urls"`
}
