package backend

import (
	"context"
	"errors"
	"log"
	"os"
	"path/filepath"
	"slices"
	"sync"

	"github.com/20after4/configdir"
	"github.com/supersonic-app/supersonic/sharedutil"
)

// AudioCache manages temporary local storage of audio files fetched from the remote music server.
// It prefetches and stores tracks on disk based on an upcoming play queue.
type AudioCache struct {
	mutex sync.Mutex

	s            *ServerManager
	rootCtx      context.Context
	baseCacheDir string

	entries map[string]*cacheEntry
}

type cacheEntry struct {
	done            bool
	refCount        int
	pendingDeletion bool
	cancel          context.CancelFunc
}

// AudioCacheRequest represents a request to prefetch and cache an audio file.
type AudioCacheRequest struct {
	ID          string
	DownloadURL string
}

// NewAudioCache initializes an AudioCache using the given context, server manager,
// and local filesystem directory for storing audio files.
func NewAudioCache(ctx context.Context, s *ServerManager, baseCacheDir string) (*AudioCache, error) {
	if err := configdir.MakePath(baseCacheDir); err != nil {
		return nil, errors.New("failed to create audio cache dir")
	}
	return &AudioCache{
		s:            s,
		rootCtx:      ctx,
		baseCacheDir: baseCacheDir,
		entries:      make(map[string]*cacheEntry),
	}, nil
}

// PathForCachedFile returns the local filesystem path for a cached track,
// if the file has finished downloading. If not cached, it returns an empty string.
func (a *AudioCache) PathForCachedFile(id string) string {
	a.mutex.Lock()
	defer a.mutex.Unlock()

	if item, ok := a.entries[id]; ok && item.done {
		return a.pathForID(id)
	}
	return ""
}

// IsFullyDownloaded returns true if the file for the given id is fully downloaded.
func (a *AudioCache) IsFullyDownloaded(id string) bool {
	return a.PathForCachedFile(id) != ""
}

// PathForCachedFile returns the local filesystem path for a cached track,
// including one that is in the process of downloading.
// If it is not cached or downloading, it returns an empty string.
func (a *AudioCache) PathForCachedOrDownloadingFile(id string) string {
	return a.pathForCachedOrDownloadingFile(id, false)
}

// ObtainReferenceToFile returns the local filesystem path for a cached track,
// including one that is in the process of downloading, and obtains a refernce
// to it such that it will not be deleted until ReleaseReferenceToFile is called.

// If it is not cached or downloading, it returns an empty string.
func (a *AudioCache) ObtainReferenceToFile(id string) string {
	return a.pathForCachedOrDownloadingFile(id, true)
}

func (a *AudioCache) pathForCachedOrDownloadingFile(id string, obtainReference bool) string {
	a.mutex.Lock()
	defer a.mutex.Unlock()

	if entry, ok := a.entries[id]; ok {
		if obtainReference {
			entry.refCount++
		}
		return a.pathForID(id)
	}
	return ""
}

func (a *AudioCache) ReleaseReferenceToFile(id string) {
	a.mutex.Lock()
	defer a.mutex.Unlock()

	if e, ok := a.entries[id]; ok {
		e.refCount--
		if e.refCount == 0 && e.pendingDeletion {
			a.deleteEntry(id, e)
		}
	}
}

// CacheFile begins downloading a file (if not already downloading) and stores it
// to the cache directory under its ID as filename. The download is asynchronous.
func (a *AudioCache) CacheFile(id, dlURL string) {
	s := a.s.Server
	if s == nil {
		return
	}
	a.mutex.Lock()
	defer a.mutex.Unlock()

	a.cacheFile(id, dlURL)
}

func (a *AudioCache) cacheFile(id, dlURL string) {
	if _, ok := a.entries[id]; !ok {
		ctx, cancel := context.WithCancel(a.rootCtx)
		a.entries[id] = &cacheEntry{cancel: cancel}
		go func() {
			ok, err := sharedutil.DownloadFileWithContext(ctx, dlURL, a.pathForID(id))
			if ok {
				a.mutex.Lock()
				if e, ok := a.entries[id]; ok {
					e.done = true
				}
				a.mutex.Unlock()
			} else if err != nil && err != context.DeadlineExceeded {
				log.Printf("error downloading audio file: %v", err)
			}
			cancel() // release ctx resources when done
		}()
	}
}

// CacheOnly ensures that only the given 'fetch' list of files (plus one extra 'keep' ID) remain cached.
// Any other cached files are cancelled and deleted from disk.
func (a *AudioCache) CacheOnly(keep string, fetch []AudioCacheRequest) {
	a.mutex.Lock()
	defer a.mutex.Unlock()

	// delete files we're not keeping
	for id, e := range a.entries {
		if id != keep && !slices.ContainsFunc(fetch, func(a AudioCacheRequest) bool {
			return a.ID == id
		}) {
			if e.refCount == 0 {
				a.deleteEntry(id, e)
			} else {
				e.pendingDeletion = true
			}
		}
	}

	// start caching the ones from fetch if not already present
	for _, item := range fetch {
		if _, ok := a.entries[item.ID]; !ok {
			a.cacheFile(item.ID, item.DownloadURL)
		}
	}
}

// Shutdown cancels all in-progress downloads and deletes all files in the audio cache directory.
// This should be called during application shutdown to clean up temporary audio data.
func (a *AudioCache) Shutdown() {
	a.mutex.Lock()
	// Cancel all active downloads
	for _, entry := range a.entries {
		if entry.cancel != nil {
			entry.cancel()
		}
	}
	a.entries = nil // clear cache state
	a.mutex.Unlock()

	// Remove all files in the cache directory
	if a.baseCacheDir != "" {
		_ = os.RemoveAll(a.baseCacheDir)
	}
}

func (a *AudioCache) pathForID(id string) string {
	return filepath.Join(a.baseCacheDir, id)
}

func (a *AudioCache) deleteEntry(id string, e *cacheEntry) {
	e.cancel()
	_ = os.Remove(a.pathForID(id))
	delete(a.entries, id)
}
