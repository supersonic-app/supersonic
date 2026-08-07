package backend

import (
	"bufio"
	"context"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/20after4/configdir"
)

const (
	autoEQIndexURL     = "https://raw.githubusercontent.com/jaakkopasanen/AutoEq/master/results/INDEX.md"
	autoEQBaseURL      = "https://raw.githubusercontent.com/jaakkopasanen/AutoEq/master/results/"
	indexCacheTTL      = 7 * 24 * time.Hour  // 7 days
	profileCacheTTL    = 30 * 24 * time.Hour // 30 days
	maxMemoryCacheSize = 20                  // LRU cache size
)

var (
	ErrProfileNotFound = errors.New("AutoEQ profile not found")
	ErrInvalidFormat   = errors.New("invalid AutoEQ profile format")
)

// AutoEQProfile represents a headphone equalizer profile from AutoEQ
type AutoEQProfile struct {
	Name   string      // Display name (e.g., "Sennheiser HD 650")
	Path   string      // Full path in repo (e.g., "oratory1990/over-ear/Sennheiser HD 650")
	Source string      // Measurement source (e.g., "oratory1990")
	Type   string      // Headphone type (e.g., "over-ear")
	Preamp float64     // Preamp gain in dB
	Bands  [10]float64 // 10-band equalizer gains in dB
}

// AutoEQProfileMetadata contains just the metadata without the EQ data
type AutoEQProfileMetadata struct {
	Name   string
	Path   string
	Source string
	Type   string
}

// AutoEQManager manages fetching and caching of AutoEQ profiles
type AutoEQManager struct {
	cachePath  string
	timeout    time.Duration
	httpClient *http.Client

	// Memory cache (LRU)
	memCache      map[string]*memoryCacheEntry
	memCacheMutex sync.RWMutex
	memCacheLRU   []string // Keys in LRU order (most recently used at end)

	// Index cache
	indexCache      []AutoEQProfileMetadata
	indexCacheTime  time.Time
	indexCacheMutex sync.RWMutex
}

type memoryCacheEntry struct {
	profile      *AutoEQProfile
	lastAccessed time.Time
}

// NewAutoEQManager creates a new AutoEQ manager
func NewAutoEQManager(cachePath string, timeout time.Duration, httpClient *http.Client) *AutoEQManager {
	configdir.MakePath(cachePath)
	log.Printf("Initializing AutoEQ manager: cache=%s, timeout=%v", cachePath, timeout)
	return &AutoEQManager{
		cachePath:   cachePath,
		timeout:     timeout,
		httpClient:  httpClient,
		memCache:    make(map[string]*memoryCacheEntry),
		memCacheLRU: make([]string, 0, maxMemoryCacheSize),
	}
}

// FetchIndex fetches the list of all available AutoEQ profiles
// Results are cached for 7 days
func (m *AutoEQManager) FetchIndex(ctx context.Context) ([]AutoEQProfileMetadata, error) {
	// Check if index is already cached in memory
	m.indexCacheMutex.RLock()
	if len(m.indexCache) > 0 && time.Since(m.indexCacheTime) < indexCacheTTL {
		cached := m.indexCache
		m.indexCacheMutex.RUnlock()
		return cached, nil
	}
	m.indexCacheMutex.RUnlock()

	// Try to load from disk cache
	indexCachePath := filepath.Join(m.cachePath, "index.json")
	if profiles, err := m.loadIndexFromDisk(indexCachePath); err == nil {
		m.indexCacheMutex.Lock()
		m.indexCache = profiles
		m.indexCacheTime = time.Now()
		m.indexCacheMutex.Unlock()
		return profiles, nil
	}

	// Fetch from network
	profiles, err := m.fetchIndexFromNetwork(ctx)
	if err != nil {
		return nil, err
	}
	log.Printf("Successfully fetched %d AutoEQ profiles", len(profiles))

	// Cache in memory and disk
	m.indexCacheMutex.Lock()
	m.indexCache = profiles
	m.indexCacheTime = time.Now()
	m.indexCacheMutex.Unlock()

	m.saveIndexToDisk(indexCachePath, profiles)

	return profiles, nil
}

func (m *AutoEQManager) loadIndexFromDisk(cachePath string) ([]AutoEQProfileMetadata, error) {
	info, err := os.Stat(cachePath)
	if err != nil {
		return nil, err
	}

	// Check if cache is expired
	if time.Since(info.ModTime()) > indexCacheTTL {
		return nil, errors.New("cache expired")
	}

	data, err := os.ReadFile(cachePath)
	if err != nil {
		return nil, err
	}

	var profiles []AutoEQProfileMetadata
	if err := json.Unmarshal(data, &profiles); err != nil {
		return nil, err
	}

	return profiles, nil
}

func (m *AutoEQManager) saveIndexToDisk(cachePath string, profiles []AutoEQProfileMetadata) {
	data, err := json.Marshal(profiles)
	if err != nil {
		log.Printf("Failed to marshal AutoEQ index: %v", err)
		return
	}

	if err := os.WriteFile(cachePath, data, 0644); err != nil {
		log.Printf("Failed to write AutoEQ index cache: %v", err)
	}
}

func (m *AutoEQManager) fetchIndexFromNetwork(ctx context.Context) ([]AutoEQProfileMetadata, error) {
	log.Printf("Fetching AutoEQ index from: %s (timeout: %v)", autoEQIndexURL, m.timeout)

	ctx, cancel := context.WithTimeout(ctx, m.timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, autoEQIndexURL, nil)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}

	resp, err := m.httpClient.Do(req)
	if err != nil {
		log.Printf("HTTP request failed: %v", err)
		return nil, fmt.Errorf("fetching index: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, resp.Status)
	}

	profiles, err := m.parseIndex(resp.Body)
	if err != nil {
		log.Printf("Failed to parse AutoEQ index: %v", err)
		return nil, err
	}

	log.Printf("Successfully parsed %d profiles from AutoEQ index", len(profiles))
	return profiles, nil
}

// parseIndex parses the INDEX.md markdown file
// Format: [Display Name](./path/to/profile) by source
var indexLinkRegex = regexp.MustCompile(`-\s*\[([^\]]+)\]\(\./([^)]+)\)`)

func (m *AutoEQManager) parseIndex(r io.Reader) ([]AutoEQProfileMetadata, error) {
	scanner := bufio.NewScanner(r)
	var profiles []AutoEQProfileMetadata
	lineCount := 0
	matchCount := 0

	for scanner.Scan() {
		line := scanner.Text()
		lineCount++
		matches := indexLinkRegex.FindStringSubmatch(line)
		if len(matches) == 3 {
			matchCount++
			name := matches[1]
			path := matches[2]

			// URL-decode the name to handle any encoded characters
			if decodedName, err := url.QueryUnescape(name); err == nil {
				name = decodedName
			}

			// Extract source and type from path
			// Format: source/type/name (e.g., "oratory1990/over-ear/Sennheiser HD 650")
			parts := strings.Split(path, "/")
			source := ""
			typ := ""
			if len(parts) >= 2 {
				source = parts[0]
				typ = parts[1]

				// URL-decode source and type to display clean text
				if decodedSource, err := url.QueryUnescape(source); err == nil {
					source = decodedSource
				}
				if decodedType, err := url.QueryUnescape(typ); err == nil {
					typ = decodedType
				}
			}

			profiles = append(profiles, AutoEQProfileMetadata{
				Name:   name,
				Path:   path,
				Source: source,
				Type:   typ,
			})
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("reading index: %w", err)
	}

	log.Printf("Parsed %d lines, found %d profile matches", lineCount, matchCount)
	return profiles, nil
}

// FetchProfile fetches a specific AutoEQ profile by its path
// Results are cached with LRU eviction
func (m *AutoEQManager) FetchProfile(ctx context.Context, path string) (*AutoEQProfile, error) {
	// Check memory cache
	m.memCacheMutex.RLock()
	if entry, ok := m.memCache[path]; ok {
		entry.lastAccessed = time.Now()
		profile := entry.profile
		m.memCacheMutex.RUnlock()
		m.updateLRU(path)
		return profile, nil
	}
	m.memCacheMutex.RUnlock()

	// Check disk cache
	profile, err := m.loadProfileFromDisk(path)
	if err == nil {
		m.addToMemoryCache(path, profile)
		return profile, nil
	}

	// Fetch from network
	profile, err = m.fetchProfileFromNetwork(ctx, path)
	if err != nil {
		return nil, err
	}

	// Cache in memory and disk
	m.addToMemoryCache(path, profile)
	m.saveProfileToDisk(path, profile)

	return profile, nil
}

func (m *AutoEQManager) loadProfileFromDisk(path string) (*AutoEQProfile, error) {
	cacheKey := m.profileCacheKey(path)
	cachePath := filepath.Join(m.cachePath, cacheKey+".json")

	info, err := os.Stat(cachePath)
	if err != nil {
		return nil, err
	}

	// Check if cache is expired
	if time.Since(info.ModTime()) > profileCacheTTL {
		os.Remove(cachePath)
		return nil, errors.New("cache expired")
	}

	data, err := os.ReadFile(cachePath)
	if err != nil {
		return nil, err
	}

	var profile AutoEQProfile
	if err := json.Unmarshal(data, &profile); err != nil {
		return nil, err
	}

	return &profile, nil
}

func (m *AutoEQManager) saveProfileToDisk(path string, profile *AutoEQProfile) {
	cacheKey := m.profileCacheKey(path)
	cachePath := filepath.Join(m.cachePath, cacheKey+".json")

	data, err := json.Marshal(profile)
	if err != nil {
		log.Printf("Failed to marshal AutoEQ profile: %v", err)
		return
	}

	if err := os.WriteFile(cachePath, data, 0644); err != nil {
		log.Printf("Failed to write AutoEQ profile cache: %v", err)
	}
}

func (m *AutoEQManager) profileCacheKey(path string) string {
	hash := md5.Sum([]byte(path))
	return hex.EncodeToString(hash[:])
}

func (m *AutoEQManager) fetchProfileFromNetwork(ctx context.Context, path string) (*AutoEQProfile, error) {
	// URL-decode the path first (INDEX.md contains HTML-encoded paths like %20 for spaces)
	decodedPath, err := url.QueryUnescape(path)
	if err != nil {
		log.Printf("Failed to decode path %s: %v", path, err)
		decodedPath = path // fallback to original
	}

	// Extract the headphone name from the path (last component)
	// Path format: "source/type/Headphone Name"
	pathComponents := strings.Split(decodedPath, "/")
	if len(pathComponents) == 0 {
		return nil, fmt.Errorf("invalid path format: %s", path)
	}
	headphoneName := pathComponents[len(pathComponents)-1]

	// Construct the URL by properly encoding each path component
	for i, component := range pathComponents {
		pathComponents[i] = url.PathEscape(component)
	}
	encodedPath := strings.Join(pathComponents, "/")

	// The file is named "{HeadphoneName} FixedBandEQ.txt"
	encodedFileName := url.PathEscape(headphoneName + " FixedBandEQ.txt")
	profileURL := autoEQBaseURL + encodedPath + "/" + encodedFileName

	ctx, cancel := context.WithTimeout(ctx, m.timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, profileURL, nil)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}

	resp, err := m.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetching profile: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, ErrProfileNotFound
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, resp.Status)
	}

	return m.parseProfile(path, resp.Body)
}

// parseProfile parses the FixedBandEQ.txt file
// Format:
// Preamp: -6.0 dB
// Filter 1: ON PK Fc 31 Hz Gain 5.0 dB Q 0.70
// ... (10 filters total)
var (
	preampRegex = regexp.MustCompile(`Preamp:\s*([-+]?\d+\.?\d*)\s*dB`)
	filterRegex = regexp.MustCompile(`Filter\s+\d+:.*?Fc\s+(\d+)\s+Hz.*?Gain\s+([-+]?\d+\.?\d*)\s*dB`)
)

func (m *AutoEQManager) parseProfile(path string, r io.Reader) (*AutoEQProfile, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("reading profile: %w", err)
	}

	content := string(data)

	// Parse preamp
	preampMatch := preampRegex.FindStringSubmatch(content)
	if len(preampMatch) < 2 {
		return nil, fmt.Errorf("%w: preamp not found", ErrInvalidFormat)
	}

	preamp, err := strconv.ParseFloat(preampMatch[1], 64)
	if err != nil {
		return nil, fmt.Errorf("%w: invalid preamp value", ErrInvalidFormat)
	}

	// Parse filters
	filterMatches := filterRegex.FindAllStringSubmatch(content, -1)
	if len(filterMatches) != 10 {
		return nil, fmt.Errorf("%w: expected 10 filters, found %d", ErrInvalidFormat, len(filterMatches))
	}

	var bands [10]float64
	expectedFreqs := []int{31, 62, 125, 250, 500, 1000, 2000, 4000, 8000, 16000}

	for i, match := range filterMatches {
		if len(match) < 3 {
			return nil, fmt.Errorf("%w: invalid filter %d", ErrInvalidFormat, i+1)
		}

		freq, err := strconv.Atoi(match[1])
		if err != nil {
			return nil, fmt.Errorf("%w: invalid frequency in filter %d", ErrInvalidFormat, i+1)
		}

		// Verify frequency matches expected (with tolerance for rounding)
		if freq != expectedFreqs[i] {
			log.Printf("Warning: AutoEQ filter %d has unexpected frequency %d Hz (expected %d Hz)",
				i+1, freq, expectedFreqs[i])
		}

		gain, err := strconv.ParseFloat(match[2], 64)
		if err != nil {
			return nil, fmt.Errorf("%w: invalid gain in filter %d", ErrInvalidFormat, i+1)
		}

		bands[i] = gain
	}

	// Extract name and metadata from path
	// URL-decode the path first to get clean names
	decodedPath, err := url.QueryUnescape(path)
	if err != nil {
		decodedPath = path // fallback to original if decode fails
	}

	parts := strings.Split(decodedPath, "/")
	name := decodedPath
	source := ""
	typ := ""

	if len(parts) >= 3 {
		name = parts[len(parts)-1]
		source = parts[0]
		typ = parts[1]
	}

	return &AutoEQProfile{
		Name:   name,
		Path:   path,
		Source: source,
		Type:   typ,
		Preamp: preamp,
		Bands:  bands,
	}, nil
}

func (m *AutoEQManager) addToMemoryCache(path string, profile *AutoEQProfile) {
	m.memCacheMutex.Lock()
	defer m.memCacheMutex.Unlock()

	// If already in cache, just update
	if _, exists := m.memCache[path]; exists {
		m.memCache[path].profile = profile
		m.memCache[path].lastAccessed = time.Now()
		return
	}

	// Evict LRU if at capacity
	if len(m.memCache) >= maxMemoryCacheSize {
		lruKey := m.memCacheLRU[0]
		delete(m.memCache, lruKey)
		m.memCacheLRU = m.memCacheLRU[1:]
	}

	// Add new entry
	m.memCache[path] = &memoryCacheEntry{
		profile:      profile,
		lastAccessed: time.Now(),
	}
	m.memCacheLRU = append(m.memCacheLRU, path)
}

func (m *AutoEQManager) updateLRU(path string) {
	m.memCacheMutex.Lock()
	defer m.memCacheMutex.Unlock()

	// Find and move to end
	for i, key := range m.memCacheLRU {
		if key == path {
			m.memCacheLRU = append(m.memCacheLRU[:i], m.memCacheLRU[i+1:]...)
			m.memCacheLRU = append(m.memCacheLRU, path)
			break
		}
	}
}
