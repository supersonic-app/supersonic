package dlna

import (
	"bytes"
	"context"
	"crypto/md5"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/hashicorp/go-retryablehttp"
	"github.com/supersonic-app/go-upnpcast/device"
	"github.com/supersonic-app/go-upnpcast/services/avtransport"
	"github.com/supersonic-app/go-upnpcast/services/renderingcontrol"
	"github.com/supersonic-app/supersonic/backend/mediaprovider"
	"github.com/supersonic-app/supersonic/backend/player"
	"github.com/supersonic-app/supersonic/backend/util"
)

const (
	stopped = 0
	playing = 1
	paused  = 2
)

// AVTransport state reported by a device that is still loading media
const transitioning = "TRANSITIONING"

type proxyMapEntry struct {
	key string
	url string
}

type DLNAPlayer struct {
	player.BasePlayerCallbackImpl

	destroyed     bool
	cancelRequest context.CancelFunc

	avTransport   *avtransport.Client
	renderControl *renderingcontrol.Client

	// coverArtPathFn returns a local filesystem path to the cached cover
	// art image for the given CoverArtID, or an error if no path is
	// available. When set and the resolver succeeds, the path is exposed
	// through the local proxy and emitted as upnp:albumArtURI in DIDL-Lite
	// so the renderer can fetch it.
	coverArtPathFn func(coverArtID string) (string, error)

	state   int // stopped, playing, paused
	seeking bool

	metaLock      sync.Mutex
	curTrackMeta  mediaprovider.MediaItemMetadata
	nextTrackMeta mediaprovider.MediaItemMetadata

	// if true, report playback time 00:00
	// pending time sync with player after beginning playback
	pendingPlayStart bool
	// start playback position in seconds of the last seek/time sync
	lastStartTime int
	// how long the track has been playing since last time sync
	stopwatch util.Stopwatch

	proxyServer *http.Server
	proxyActive atomic.Bool
	localIP     string
	proxyPort   int

	pendingSeek     bool
	pendingSeekSecs float64

	// keep in order of most recently accessed at the end
	// that way the item in proxyURLs[0] can be kicked out
	// when adding a new URL to the proxy. The current and the next
	// track each occupy a stream and a cover art entry, and the
	// track they replaced can still be being read.
	proxyURLs    [6]proxyMapEntry
	proxyURLLock sync.Mutex

	// If SetNextAVTransport fails (e.g. because the device
	// does not support the API/gapless), this flag is set
	// true, and the next firing of the track change timer
	// should clear it to false and use SetAVTransport
	// to begin playing the item in nextTrackMeta.
	failedToSetNext    bool
	unsetNextMediaItem *avtransport.MediaItem

	timerActive atomic.Bool
	timer       *time.Timer
	resetChan   chan (time.Duration)
}

func NewDLNAPlayer(device *device.MediaRenderer, coverArtPathFn func(coverArtID string) (string, error)) (*DLNAPlayer, error) {
	retry := retryablehttp.NewClient()
	retry.RetryMax = 3
	retry.RetryWaitMin = 100 * time.Millisecond
	retry.Logger = retryLogger{}
	retry.HTTPClient.Transport = lengthedBodyTransport{retry.HTTPClient.Transport}
	cli := retry.StandardClient()

	avt, err := device.AVTransportClient()
	if err != nil {
		return nil, err
	}
	avt.RequestHandler = httpClientHandler{cli}
	rc, err := device.RenderingControlClient()
	if err != nil {
		return nil, err
	}
	rc.Requesthandler = cli

	// ping to test connectivity
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, err := avt.GetTransportInfo(ctx); err != nil {
		return nil, fmt.Errorf("failed to connect to %s", device.FriendlyName)
	}

	// a renderer that was left muted plays silence, and Supersonic
	// offers no way to unmute it
	rc.SetMute(ctx, false)

	return &DLNAPlayer{
		avTransport:    avt,
		renderControl:  rc,
		resetChan:      make(chan time.Duration),
		coverArtPathFn: coverArtPathFn,
	}, nil
}

// buildMediaItem assembles the avtransport.MediaItem for a track. It
// populates the audio metadata fields (artist, album, track number, sample
// rate, bit depth, channel count, size, bitrate, duration) so the
// resulting DIDL-Lite includes the information the renderer needs to
// display cover art, artist, album, and a stream-format readout.
//
// playbackURL is the proxy URL the renderer will fetch the stream from.
func (d *DLNAPlayer) buildMediaItem(playbackURL string, meta mediaprovider.MediaItemMetadata) avtransport.MediaItem {
	item := avtransport.MediaItem{
		URL:         playbackURL,
		Title:       meta.Name,
		ContentType: meta.MIMEType,
		Seekable:    true,
		Duration:    meta.Duration,
		Artist:      strings.Join(meta.Artists, ", "),
		Album:       meta.Album,
		TrackNumber: meta.TrackNumber,
		Size:        meta.Size,
		// DIDL-Lite res@bitrate is bytes/sec; meta.BitRate is kbps from
		// the server. Convert: kbps * 1000 / 8 = bytes/sec.
		Bitrate:         meta.BitRate * 125,
		SampleFrequency: meta.SampleRate,
		BitsPerSample:   meta.BitDepth,
		NrAudioChannels: meta.ChannelCount,
	}
	if meta.CoverArtID != "" && d.coverArtPathFn != nil {
		if path, err := d.coverArtPathFn(meta.CoverArtID); err == nil {
			artKey := d.addURLToProxy(path)
			item.AlbumArtURI = d.urlForItem(artKey)
		}
	}
	return item
}

func (d *DLNAPlayer) SetVolume(vol int) error {
	if d.destroyed {
		return nil
	}
	ctx, cancel := context.WithCancel(context.Background())
	d.cancelRequest = cancel
	defer cancel()
	return d.renderControl.SetVolume(ctx, vol)
}

func (d *DLNAPlayer) GetVolume() int {
	if d.destroyed {
		return 0
	}
	ctx, cancel := context.WithCancel(context.Background())
	d.cancelRequest = cancel
	defer cancel()
	vol, _ := d.renderControl.GetVolume(ctx)
	return vol
}

func (d *DLNAPlayer) PlayFile(urlstr string, meta mediaprovider.MediaItemMetadata, startTime float64) error {
	if d.destroyed {
		return nil
	}

	d.ensureSetupProxy()

	d.metaLock.Lock()
	d.curTrackMeta = meta
	d.metaLock.Unlock()
	key := d.addURLToProxy(urlstr)

	media := d.buildMediaItem(d.urlForItem(key), meta)

	if err := d.playAVTransportMedia(&media); err != nil {
		return err
	}
	d.pendingPlayStart = true
	if startTime > 0 {
		d.awaitPlaybackStart()
		if !d.destroyed {
			d.sendSeekCmd(startTime)
		}
		d.pendingPlayStart = false
	} else {
		go func() {
			d.awaitPlaybackStart()
			if !d.destroyed {
				d.syncPlaybackTime()
			}
			d.pendingPlayStart = false
		}()
	}
	d.state = playing
	remainingDur := meta.Duration - time.Duration(startTime)*time.Second
	d.setTrackChangeTimer(remainingDur)
	d.stopwatch.Reset()
	d.stopwatch.Start()
	d.lastStartTime = int(startTime)
	d.InvokeOnPlaying()
	d.InvokeOnTrackChange()
	if startTime > 0 {
		d.InvokeOnSeek()
	}

	return nil
}

func (d *DLNAPlayer) playAVTransportMedia(media *avtransport.MediaItem) error {
	ctx, cancel := context.WithCancel(context.Background())
	d.cancelRequest = cancel
	defer cancel()

	err := d.avTransport.SetAVTransportMedia(ctx, media)
	if err != nil {
		return err
	}
	if err := d.avTransport.Play(ctx); err != nil {
		return err
	}
	return nil
}

// awaitPlaybackStart waits for the renderer to finish loading the media it
// was handed. A seek sent while the device is still transitioning is
// silently dropped - Sonos answers it 200 OK and keeps playing from the
// start of the track - so commands must wait for the transition to end.
func (d *DLNAPlayer) awaitPlaybackStart() {
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		time.Sleep(200 * time.Millisecond)
		if d.destroyed {
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		info, err := d.avTransport.GetTransportInfo(ctx)
		cancel()
		if err == nil && info.State != transitioning {
			return
		}
	}
}

func (d *DLNAPlayer) SetNextFile(url string, meta mediaprovider.MediaItemMetadata) error {
	if d.destroyed {
		return nil
	}

	var media *avtransport.MediaItem
	d.metaLock.Lock()
	d.nextTrackMeta = meta
	d.metaLock.Unlock()
	if url != "" {
		d.ensureSetupProxy()

		key := d.addURLToProxy(url)
		item := d.buildMediaItem(d.urlForItem(key), meta)
		media = &item
	} else {
		// empty media item to signify erasing next track in device queue
		media = &avtransport.MediaItem{}
	}

	ctx, cancel := context.WithCancel(context.Background())
	d.cancelRequest = cancel
	defer cancel()
	err := d.avTransport.SetNextAVTransportMedia(ctx, media)

	d.metaLock.Lock()
	// Clearing the queue has nothing to fall back to, and succeeding
	// here must drop any item a previous failure left behind, so that
	// the next track change does not start playing it.
	if err != nil && url != "" {
		d.failedToSetNext = true
		d.unsetNextMediaItem = media
	} else {
		d.failedToSetNext = false
		d.unsetNextMediaItem = nil
	}
	d.metaLock.Unlock()
	return err
}

func (d *DLNAPlayer) Continue() error {
	if d.destroyed || d.state == playing {
		return nil
	}

	ctx, cancel := context.WithCancel(context.Background())
	d.cancelRequest = cancel
	defer cancel()

	if d.pendingSeek {
		d.pendingSeek = false
		err := d.avTransport.Seek(ctx, int(d.pendingSeekSecs))
		if err != nil {
			return err
		}
	}

	if err := d.avTransport.Play(ctx); err != nil {
		return err
	}
	d.metaLock.Lock()
	nextTrackChange := d.curTrackMeta.Duration - d.curPlayPos()
	d.metaLock.Unlock()
	d.state = playing
	d.setTrackChangeTimer(nextTrackChange)
	d.stopwatch.Start()
	d.InvokeOnPlaying()
	return nil
}

func (d *DLNAPlayer) Pause() error {
	if d.destroyed || d.state != playing {
		return nil
	}

	ctx, cancel := context.WithCancel(context.Background())
	d.cancelRequest = cancel
	defer cancel()
	if err := d.avTransport.Pause(ctx); err != nil {
		return err
	}
	d.setTrackChangeTimer(0)
	d.stopwatch.Stop()
	d.state = paused
	d.InvokeOnPaused()
	return nil
}

func (d *DLNAPlayer) Stop(force bool) error {
	if d.destroyed {
		return nil
	}
	if force && d.cancelRequest != nil {
		d.cancelRequest()
	}

	switch d.state {
	case stopped:
		return nil
	case playing:
		var ctx context.Context
		var cancel context.CancelFunc
		if force {
			ctx, cancel = context.WithTimeout(context.Background(), 2*time.Second)
		} else {
			ctx, cancel = context.WithCancel(context.Background())
		}
		d.cancelRequest = cancel
		defer cancel()

		if err := d.avTransport.Pause(ctx); err != nil {
			return err
		}
		fallthrough
	case paused:
		d.setTrackChangeTimer(0)
		d.stopwatch.Reset()
		d.lastStartTime = 0
		d.state = stopped
		d.InvokeOnStopped()
		return nil
	default:
		return errors.New("invalid player state")
	}
}

func (d *DLNAPlayer) SeekSeconds(secs float64) error {
	if d.destroyed {
		return nil
	}

	if d.state == paused {
		d.pendingSeek = true
		d.pendingSeekSecs = secs
	} else {
		if err := d.sendSeekCmd(secs); err != nil {
			return err
		}
	}

	d.lastStartTime = int(secs)
	d.stopwatch.Reset()

	if d.state == playing {
		d.metaLock.Lock()
		nextTrackChange := d.curTrackMeta.Duration - time.Duration(secs)*time.Second
		d.metaLock.Unlock()
		d.setTrackChangeTimer(nextTrackChange)
		d.stopwatch.Start()
	}

	d.InvokeOnSeek()

	go func() {
		time.Sleep(4 * time.Second)
		if !d.destroyed {
			d.syncPlaybackTime()
		}
	}()
	return nil
}

func (d *DLNAPlayer) sendSeekCmd(secs float64) error {
	d.seeking = true
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := d.avTransport.Seek(ctx, int(secs)); err != nil {
		d.seeking = false
		return err
	}
	d.seeking = false
	return nil
}

func (d *DLNAPlayer) IsSeeking() bool {
	return d.seeking
}

func (d *DLNAPlayer) GetStatus() player.Status {
	state := player.Stopped
	if d.state == playing {
		state = player.Playing
	} else if d.state == paused {
		state = player.Paused
	}

	var timePos float64
	if !d.pendingPlayStart {
		timePos = d.curPlayPos().Seconds()
	}
	return player.Status{
		State:    state,
		TimePos:  timePos,
		Duration: d.curTrackMeta.Duration.Seconds(),
	}
}

func (d *DLNAPlayer) curPlayPos() time.Duration {
	return time.Duration(d.lastStartTime)*time.Second + d.stopwatch.Elapsed()
}

func (d *DLNAPlayer) Destroy() {
	d.destroyed = true
	d.setTrackChangeTimer(0)
	if d.cancelRequest != nil {
		d.cancelRequest()
	}

	if d.proxyServer != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		go d.proxyServer.Shutdown(ctx)
		d.proxyServer = nil
	}
}

func (d *DLNAPlayer) syncPlaybackTime() {
	start := time.Now()
	if pos, err := d.avTransport.GetPositionInfo(context.Background()); err == nil {
		d.lastStartTime = int(pos.RelTime.Seconds() + (time.Since(start) / 2).Seconds())
		d.stopwatch.Reset()
		if d.state == playing {
			d.stopwatch.Start()
		}
		d.setTrackChangeTimer(d.curTrackMeta.Duration - time.Duration(d.lastStartTime)*time.Second)
		d.InvokeOnSeek()
	}
}

func (d *DLNAPlayer) ensureSetupProxy() error {
	if d.proxyActive.Swap(true) {
		return nil // already active
	}

	var err error
	d.localIP, err = util.GetLocalIP()
	if err != nil {
		return err
	}

	listener, err := net.Listen("tcp", ":0")
	if err != nil {
		return err
	}
	d.proxyPort = listener.Addr().(*net.TCPAddr).Port

	d.proxyServer = &http.Server{
		Handler: http.HandlerFunc(d.handleRequest),
	}

	go d.proxyServer.Serve(listener)
	return nil
}

func (d *DLNAPlayer) setTrackChangeTimer(dur time.Duration) {
	if d.timerActive.Swap(true) {
		// was active
		d.resetChan <- dur
		return
	}
	if dur == 0 {
		d.timerActive.Store(false)
		return
	}

	d.timer = time.NewTimer(dur)
	go func() {
		for {
			select {
			case dur := <-d.resetChan:
				if dur == 0 {
					d.timerActive.Store(false)
					if !d.timer.Stop() {
						select {
						case <-d.timer.C:
						default:
						}
					}
					d.timer = nil
					return
				}
				// reset the timer
				if !d.timer.Stop() {
					select {
					case <-d.timer.C:
					default:
					}
				}
				d.timer.Reset(dur)
			case <-d.timer.C:
				d.timerActive.Store(false)
				d.timer = nil
				d.handleOnTrackChange()
				return
			}
		}
	}()
}

func (d *DLNAPlayer) handleOnTrackChange() {
	stopping := false
	d.metaLock.Lock()
	if d.nextTrackMeta.ID == "" {
		stopping = true
	}
	d.curTrackMeta = d.nextTrackMeta
	d.nextTrackMeta = mediaprovider.MediaItemMetadata{}
	nextTrackChange := d.curTrackMeta.Duration
	d.metaLock.Unlock()

	if stopping {
		// The renderer has to be told to stop, not just left to run off
		// the end of the stream: a Sonos player that reaches the end of
		// one resumes whatever session it was playing beforehand.
		d.Stop(false)
	} else {
		d.metaLock.Lock()
		if d.failedToSetNext {
			d.failedToSetNext = false
			media := d.unsetNextMediaItem
			d.unsetNextMediaItem = nil
			d.metaLock.Unlock()
			d.playAVTransportMedia(media)
		} else {
			d.metaLock.Unlock()
		}

		d.lastStartTime = 0
		d.stopwatch.Reset()
		d.stopwatch.Start()
		d.setTrackChangeTimer(nextTrackChange)
		d.InvokeOnTrackChange()

		go func() {
			time.Sleep(5 * time.Second)
			if !d.destroyed {
				d.syncPlaybackTime()
			}
		}()
	}
}

func (d *DLNAPlayer) urlForItem(key string) string {
	return fmt.Sprintf("http://%s:%d/%s", d.localIP, d.proxyPort, key)
}

func (d *DLNAPlayer) handleRequest(w http.ResponseWriter, r *http.Request) {
	key := strings.TrimPrefix(r.URL.Path, "/")
	url, _ := d.lookupProxyURL(key)

	if url == "" {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte("404"))
		return
	}

	// if the url is a filepath for a local cached file, serve it
	if info, err := os.Stat(url); err == nil && info.Size() > 0 {
		http.ServeFile(w, r, url)
		return
	}

	// Otherwise, proxy request to the music server
	proxyReq, err := http.NewRequest(r.Method, url, r.Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Copy headers from the original request to the new request
	proxyReq.Header = r.Header

	// Create an HTTP client and send the request
	client := &http.Client{}
	resp, err := client.Do(proxyReq)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	// Copy headers from the response to the writer
	for name, values := range resp.Header {
		for _, value := range values {
			w.Header().Add(name, value)
		}
	}

	// Set the status code
	w.WriteHeader(resp.StatusCode)

	// Copy the response body to the writer
	_, err = io.Copy(w, resp.Body)
	if err != nil {
		fmt.Fprintln(os.Stderr, "Error copying response body:", err)
	}
}

func (d *DLNAPlayer) addURLToProxy(url string) string {
	hash := md5.Sum([]byte(url))
	key := base64.StdEncoding.EncodeToString(hash[:])
	d.proxyURLLock.Lock()
	defer d.proxyURLLock.Unlock()
	d._updateProxyURL(key, url)
	return key
}

// lookupProxyURL finds a URL by key and updates its position to most recently used
func (d *DLNAPlayer) lookupProxyURL(key string) (string, bool) {
	d.proxyURLLock.Lock()
	defer d.proxyURLLock.Unlock()

	for i := range len(d.proxyURLs) {
		if d.proxyURLs[i].key == key {
			url := d.proxyURLs[i].url
			// Move accessed entry to the most recent position
			d._updateProxyURL(key, url)
			return url, true
		}
	}

	return "", false
}

func (d *DLNAPlayer) _updateProxyURL(key, url string) {
	// Check if the key already exists, and if so, move it to the most recently used position
	for i := range len(d.proxyURLs) {
		if d.proxyURLs[i].key == key {
			if i < len(d.proxyURLs)-1 {
				// Shift elements to the left from found position to the end
				copy(d.proxyURLs[i:], d.proxyURLs[i+1:])
			}
			// Place updated entry at the last position
			d.proxyURLs[len(d.proxyURLs)-1] = proxyMapEntry{key: key, url: url}
			return
		}
	}

	// Shift all elements left to make room for the new entry at the end
	copy(d.proxyURLs[:], d.proxyURLs[1:])
	// Insert new element at the most recent position
	d.proxyURLs[len(d.proxyURLs)-1] = proxyMapEntry{key: key, url: url}
}

// lengthedBodyTransport buffers request bodies of unknown length so that
// they are sent with a Content-Length instead of chunked encoding.
// go-upnpcast builds its SOAP bodies from readers, and Sonos players
// refuse to answer chunked control requests.
type lengthedBodyTransport struct {
	http.RoundTripper
}

func (t lengthedBodyTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	// A body of unknown length carries no ContentLength, which for a
	// non-empty body is what makes net/http fall back to chunking.
	if req.Body == nil || req.ContentLength > 0 {
		return t.RoundTripper.RoundTrip(req)
	}

	body, err := io.ReadAll(req.Body)
	req.Body.Close()
	if err != nil {
		return nil, err
	}

	req = req.Clone(req.Context())
	req.ContentLength = int64(len(body))
	req.Body = io.NopCloser(bytes.NewReader(body))
	req.GetBody = func() (io.ReadCloser, error) {
		return io.NopCloser(bytes.NewReader(body)), nil
	}
	return t.RoundTripper.RoundTrip(req)
}

// httpClientHandler wraps an http.Client to implement services.RequestHandler
type httpClientHandler struct {
	client *http.Client
}

func (h httpClientHandler) Do(req *http.Request) (*http.Response, error) {
	return h.client.Do(req)
}

type retryLogger struct{}

func (retryLogger) Error(msg string, keysAndValues ...any) {
	log.Println(msg, keysAndValues)
}

func (retryLogger) Info(msg string, keysAndValues ...any) {
	log.Println(msg, keysAndValues)
}

func (retryLogger) Warn(msg string, keysAndValues ...any) {
	log.Println(msg, keysAndValues)
}

func (retryLogger) Debug(msg string, keysAndValues ...any) {
	// log only retries, not every request
	if strings.Contains(msg, "retrying request") {
		log.Println(msg, keysAndValues)
	}
}
