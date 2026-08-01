#pragma once

#include <stdint.h>

// Decode-step return codes (av_player_decode_step)
#define AVPLAYER_DECODE_OK         0   // decoded and buffered a frame
#define AVPLAYER_DECODE_RING_FULL  1   // ring buffer full; caller should sleep briefly
#define AVPLAYER_DECODE_EOF        2   // current track exhausted (ring buffer still draining)
#define AVPLAYER_DECODE_NEXT_READY 3   // next track pre-opened and ready; swapped in
#define AVPLAYER_DECODE_STOPPED    4   // playback stopped (ring buffer drained after EOF)
#define AVPLAYER_DECODE_ERROR     -1   // unrecoverable error

// Player states
#define AVPLAYER_STATE_STOPPED  0
#define AVPLAYER_STATE_PLAYING  1
#define AVPLAYER_STATE_PAUSED   2

// Output sample rate and format (fixed for gapless compatibility across tracks)
#define AVPLAYER_SAMPLE_RATE        48000
#define AVPLAYER_SAMPLE_RATE_STR    "48000"
#define AVPLAYER_CHANNELS           2
// Ring buffer capacity in frames (~4 seconds)
#define AVPLAYER_RING_FRAMES    (AVPLAYER_SAMPLE_RATE * 4)
// Maximum number of parametric EQ bands
#define AVPLAYER_MAX_EQ_BANDS   15

typedef struct av_player av_player_t;

// One parametric EQ band passed from Go (numeric params, no FFmpeg strings)
typedef struct {
    double frequency;   // center frequency in Hz
    double gain_db;     // gain in dB (0 = no effect)
    double q;           // Q factor
} av_eq_band_t;

// Audio device info (for enumeration)
typedef struct {
    char name[256];
    char description[256];
} av_device_info_t;

// Media info populated after a track opens
typedef struct {
    char   codec[64];
    int    sample_rate;
    int    channels;
    int    bitrate;        // bits/sec; updated per-packet for VBR
} av_media_info_t;

// ---- Lifecycle -------------------------------------------------------

// Allocate a new player.  Must be followed by av_player_init().
av_player_t *av_player_create(void);

// Initialise the miniaudio device.
// device_name: NULL or "" for default device.
// exclusive: non-zero to request exclusive access.
// Returns 0 on success, non-zero on failure.
int av_player_init(av_player_t *p, const char *device_name, int exclusive);

// Destroy the player and free all resources.
void av_player_destroy(av_player_t *p);

// ---- Playback --------------------------------------------------------

// Open a URL and prepare for playback.
// Returns 0 on success.
int av_player_open(av_player_t *p, const char *url, double start_time);

// Pre-open the next URL for gapless playback.  Call while current track is
// playing; the decode loop will seamlessly switch when current track ends.
// Pass NULL/empty url to clear any queued next track.
int av_player_open_next(av_player_t *p, const char *url);

// Stop playback and discard any buffered audio.
void av_player_stop(av_player_t *p);

// Pause/resume (low-latency; does not stop the miniaudio device).
void av_player_pause(av_player_t *p);
void av_player_resume(av_player_t *p);

// Seek to an absolute time (seconds).  Returns 0 on success.
int av_player_seek(av_player_t *p, double seconds);

// Set playback volume (0.0–1.0).
void av_player_set_volume(av_player_t *p, float volume);

// ---- ReplayGain ------------------------------------------------------

// Set ReplayGain mode (0=off, 1=track, 2=album), prevent-clipping flag, and
// preamp offset in dB.  Reads tags from the current decoder's metadata and
// applies the computed gain immediately.  Also arms a pending gain switch at
// the next gapless track boundary so the new track's gain takes effect
// exactly when its audio starts playing.
void av_player_set_replay_gain(av_player_t *p, int mode, int prevent_clip, double preamp_db);

// Update EQ bands and preamp (lock-free swap; takes effect on next audio callback).
// preamp_db is applied after peak metering and before EQ bands.
// Pass NULL/0 bands to disable EQ bands (preamp still applies if non-zero).
void av_player_set_eq(av_player_t *p, const av_eq_band_t *bands, int num_bands, double preamp_db);

// ---- Status ----------------------------------------------------------

int    av_player_get_state(av_player_t *p);
double av_player_get_position(av_player_t *p);
double av_player_get_duration(av_player_t *p);

// Number of frames still buffered in the ring (proxy for "is audio draining")
int av_player_buffered_frames(av_player_t *p);

// ---- Peaks -----------------------------------------------------------

// Populate peak/RMS values from the last processed frame (dBFS, -inf..0).
// Values are invalid (0.0) when not playing.
void av_player_get_peaks(av_player_t *p,
                         double *l_peak, double *r_peak,
                         double *l_rms,  double *r_rms);

// Enable/disable peak measurement (astats filter in graph).
void av_player_set_peaks_enabled(av_player_t *p, int enabled);

// ---- Media info ------------------------------------------------------

void av_player_get_media_info(av_player_t *p, av_media_info_t *info);

// ---- ICY metadata ----------------------------------------------------

// Check whether the ICY StreamTitle has changed since the last call.
// If changed, copies the new title into buf (NUL-terminated, at most buflen-1 bytes)
// and returns 1.  Returns 0 if unchanged or no ICY metadata is available.
// Must be called from the decode goroutine only.
int av_player_check_icy_title(av_player_t *p, char *buf, int buflen);

// ---- Waveform analysis (no player instance required) -----------------

// Decode in_url and compute per-chunk peak/RMS values for waveform rendering.
// Writes 1024 bytes each to out_peak and out_rms arrays, updating *progress
// atomically as each chunk completes (Go polls this for progressive rendering).
// duration_ms is the expected track duration used to compute chunk sizes.
// Runs synchronously; does not require av_player_t or miniaudio.
// Returns 0 on success, negative AVERROR code on failure.
int av_analyze_waveform(const char *in_url, int64_t duration_ms,
                        uint8_t *out_peak, uint8_t *out_rms,
                        _Atomic int *progress, _Atomic int *cancel);

// ---- Device management -----------------------------------------------

// Fill devices[0..max_devices-1].  Returns the number of devices filled.
int av_player_list_devices(av_device_info_t *devices, int max_devices);

// Change the output device.  NULL or "" selects the system default.
// Returns 0 on success.
int av_player_set_device(av_player_t *p, const char *device_name);

// Set exclusive mode on/off.  Returns 0 on success.
int av_player_set_exclusive(av_player_t *p, int exclusive);

// ---- Decode loop (called from Go goroutine) --------------------------

// Perform one decode step: read → decode → filter → write ring buffer.
// See AVPLAYER_DECODE_* return codes above.
// This is the only function that should be called repeatedly in a loop.
int av_player_decode_step(av_player_t *p);
