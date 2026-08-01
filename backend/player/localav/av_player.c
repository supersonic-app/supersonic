// av_player.c — FFmpeg + miniaudio audio player
// One translation unit: defines miniaudio implementation here.

// Use FFmpeg for all decoding; disable miniaudio decoders
#define MA_NO_DECODING
#define MA_NO_ENCODING
#define MA_IMPLEMENTATION
#include "miniaudio.h"

#include "av_player.h"

#include <libavcodec/avcodec.h>
#include <libavformat/avformat.h>
#include <libavfilter/avfilter.h>
#include <libavfilter/buffersink.h>
#include <libavfilter/buffersrc.h>
#include <libavutil/opt.h>
#include <libavutil/samplefmt.h>
#include <libavutil/channel_layout.h>
#include <libavutil/dict.h>
#include <libswresample/swresample.h>

#include <stdlib.h>
#include <string.h>
#include <stdio.h>
#include <math.h>
#include <stdatomic.h>

// --------------------------------------------------------------------------
// Internal types
// --------------------------------------------------------------------------

typedef struct decoder {
    AVFormatContext *fmt_ctx;
    AVCodecContext  *codec_ctx;
    AVFilterGraph   *filter_graph;
    AVFilterContext *buffersrc_ctx;
    AVFilterContext *buffersink_ctx;
    AVPacket        *pkt;
    AVFrame         *frame;
    AVFrame         *filt_frame;
    AVFrame         *pending_frame; // filtered frame that didn't fit in ring — retry next step
    int              audio_stream_idx;
    double           duration;      // seconds; 0 if unknown
    // ReplayGain tag values read from file metadata (defaults: gain=0.0, peak=1.0)
    double           rg_track_gain_db;
    double           rg_album_gain_db;
    float            rg_track_peak;
    float            rg_album_peak;
    char             url[2048];
} decoder_t;

// EQ bank: one set of up to AVPLAYER_MAX_EQ_BANDS ma_peak2 biquad filters
typedef struct {
    ma_peak2  filters[AVPLAYER_MAX_EQ_BANDS];
    int       num_bands;
    int       initialized;
    float     preamp;  // linear gain (1.0 = 0 dB)
} eq_bank_t;

// SPSC ring buffer — producer: decode goroutine, consumer: miniaudio callback
typedef struct {
    float          *buf;         // [AVPLAYER_RING_FRAMES * AVPLAYER_CHANNELS]
    int             cap;         // in frames = AVPLAYER_RING_FRAMES
    atomic_int      write_idx;   // next frame to write (mod cap)
    atomic_int      read_idx;    // next frame to read  (mod cap)
    atomic_int      fill;        // frames available to read
    _Atomic long long frames_written_total; // cumulative frames written since last av_player_open
} ring_buf_t;

// Bitrate history ring: maps ring-write frame positions to per-packet bitrates,
// so that GetMediaInfo can return the bitrate matching the currently playing audio
// rather than the most recently decoded packet (which is ~ring-buffer-duration ahead).
// Producer: decode goroutine (sole writer). Consumer: Go thread via av_player_get_media_info.
#define BITRATE_HISTORY_SIZE 512  // covers ~4s at typical packet rates

typedef struct {
    long long frame_pos;  // ring.frames_written_total at time of capture
    int       bitrate;    // bits per second
} bitrate_entry_t;

typedef struct {
    bitrate_entry_t entries[BITRATE_HISTORY_SIZE];
    _Atomic int     write_idx;  // next slot to write (mod BITRATE_HISTORY_SIZE)
} bitrate_history_t;

struct av_player {
    // Current decoder
    decoder_t       *dec;
    // Pre-fetched next decoder (for gapless)
    decoder_t       *next_dec;

    ring_buf_t       ring;

    ma_device        device;
    ma_context       ma_ctx;
    int              ma_ctx_init;  // bool: ma_ctx was initialised
    int              device_init;  // bool: device was initialised

    atomic_int       state;        // AVPLAYER_STATE_*
    atomic_int       seek_pending; // 1 = a seek is in progress
    double           seek_target;  // seconds (guarded by seek_pending)

    // Time tracking (updated by decode loop from packet/frame pts)
    _Atomic double   time_pos;
    _Atomic double   duration;

    float            volume;       // 0.0–1.0 (applied in miniaudio callback)

    // Peaks (updated each filtered frame; read from Go at ~60 Hz)
    _Atomic double   l_peak;
    _Atomic double   r_peak;
    _Atomic double   l_rms;
    _Atomic double   r_rms;
    int              peaks_enabled; // bool

    // Per-packet bitrate for VBR (updated in decode loop)
    _Atomic int      cur_bitrate;

    // Media info (written once when track opens)
    av_media_info_t  media_info;

    // EOF flag set by decode loop; cleared when ring drains
    atomic_int       eof_reached;      // 1 = decoder exhausted
    atomic_int       next_consumed;    // 1 = next track has been swapped in

    // Gapless track-change delay: AVPLAYER_DECODE_NEXT_READY is held back until
    // frames_played_total reaches track_change_threshold, meaning the new track's
    // audio has actually started playing out of the ring buffer.
    _Atomic long long frames_played_total;   // incremented in ma_data_callback
    atomic_int        pending_track_change;  // 1 = waiting to fire NEXT_READY
    long long         track_change_threshold; // non-atomic: only accessed by decode goroutine
    // Position clock: position = position_offset + (frames_played_total - position_clock_ref) / rate
    // Reset on av_player_open, av_player_seek, and when NEXT_READY fires.
    _Atomic double    position_offset;    // seconds at last clock reset
    _Atomic long long position_clock_ref; // frames_played_total at last clock reset

    // Protects pointer swaps of dec/next_dec between the decode goroutine
    // (gapless swap in av_player_decode_step) and the playback manager
    // goroutine (av_player_open_next).  Never held during slow operations
    // like decoder_free — only during pointer reads/writes.
    ma_mutex         decoder_lock;

    // Protects the active decoder's filter graph (filter_graph, buffersrc_ctx,
    // buffersink_ctx) against concurrent rebuilds from av_player_set_filters /
    // av_player_set_peaks_enabled while av_player_decode_step is using those
    // pointers.  Held briefly around each individual filter API call.
    ma_mutex         filter_lock;

    // EQ double-buffer: updated from Go thread, read in audio callback.
    // eq_active_idx (0 or 1) selects which bank the callback reads.
    // The other bank is safe to write to between swaps.
    eq_bank_t        eq_banks[2];
    atomic_int       eq_active_idx;

    // Bitrate history for display sync (see bitrate_history_t above)
    bitrate_history_t bitrate_hist;

    // ReplayGain output-stage gain (applied in miniaudio callback as a linear multiplier).
    // rg_gain is the active gain; a pending switch fires when frames_played_total
    // reaches rg_switch_threshold (set at gapless track boundary).
    _Atomic float      rg_gain;             // current linear gain (1.0 = no adjustment)
    _Atomic float      rg_gain_pending;     // gain for the next track
    _Atomic long long  rg_switch_threshold; // switch when frames_played_total >= this
    atomic_int         rg_switch_pending;   // 1 = a switch is waiting
    // Settings written from Go; read by decode loop at gapless swap.
    int    rg_mode;        // 0=off, 1=track, 2=album
    int    rg_prevent_clip;
    double rg_preamp_db;   // additional dB offset (from ReplayGainOptions.PreampGain)

    // ICY metadata: last seen StreamTitle, for change detection.
    // Only accessed from the decode goroutine and av_player_open (after decode stops).
    char   last_icy_title[512];
};

// --------------------------------------------------------------------------
// Ring buffer helpers
// --------------------------------------------------------------------------

static void ring_init(ring_buf_t *r) {
    r->cap = AVPLAYER_RING_FRAMES;
    r->buf = (float *)malloc(r->cap * AVPLAYER_CHANNELS * sizeof(float));
    atomic_store(&r->write_idx, 0);
    atomic_store(&r->read_idx,  0);
    atomic_store(&r->fill,      0);
    atomic_store_explicit((_Atomic long long *)&r->frames_written_total, 0LL, memory_order_relaxed);
}

static void ring_free(ring_buf_t *r) {
    free(r->buf);
    r->buf = NULL;
}

static void ring_clear(ring_buf_t *r) {
    atomic_store(&r->write_idx, 0);
    atomic_store(&r->read_idx,  0);
    atomic_store(&r->fill,      0);
}

// Returns free frames available for writing
static int ring_space(const ring_buf_t *r) {
    return r->cap - atomic_load(&r->fill);
}

// Returns frames available to read
static int ring_avail(const ring_buf_t *r) {
    return atomic_load(&r->fill);
}

// Write exactly n_frames of interleaved f32 stereo.
// Returns n_frames on success, 0 if there is not enough space (all-or-nothing).
// Never clips or drops samples — the caller must retry with the same data.
static int ring_write(ring_buf_t *r, const float *src, int n_frames) {
    if (ring_space(r) < n_frames) return 0;

    int wi  = atomic_load(&r->write_idx);
    int ch  = AVPLAYER_CHANNELS;
    int cap = r->cap;

    // How many frames fit before the wrap-around point?
    int first  = cap - wi;             // frames from wi to end of buffer
    if (first > n_frames) first = n_frames;
    int second = n_frames - first;     // frames that wrap to start of buffer

    memcpy(&r->buf[wi * ch], src, first * ch * sizeof(float));
    if (second > 0) {
        memcpy(&r->buf[0], src + first * ch, second * ch * sizeof(float));
    }

    atomic_store(&r->write_idx, (wi + n_frames) % cap);
    atomic_fetch_add(&r->fill, n_frames);
    atomic_fetch_add_explicit((_Atomic long long *)&r->frames_written_total,
                              (long long)n_frames, memory_order_relaxed);
    return n_frames;
}

// Read up to n_frames into dst.  Returns number of frames actually read.
static int ring_read(ring_buf_t *r, float *dst, int n_frames) {
    int avail   = ring_avail(r);
    int to_read = (n_frames < avail) ? n_frames : avail;
    if (to_read == 0) return 0;

    int ri  = atomic_load(&r->read_idx);
    int ch  = AVPLAYER_CHANNELS;
    int cap = r->cap;

    int first  = cap - ri;
    if (first > to_read) first = to_read;
    int second = to_read - first;

    memcpy(dst, &r->buf[ri * ch], first * ch * sizeof(float));
    if (second > 0) {
        memcpy(dst + first * ch, &r->buf[0], second * ch * sizeof(float));
    }

    atomic_store(&r->read_idx, (ri + to_read) % cap);
    atomic_fetch_sub(&r->fill, to_read);
    return to_read;
}

// --------------------------------------------------------------------------
// Decoder helpers
// --------------------------------------------------------------------------

static decoder_t *decoder_alloc(void) {
    decoder_t *d = (decoder_t *)calloc(1, sizeof(decoder_t));
    if (!d) return NULL;
    d->pkt        = av_packet_alloc();
    d->frame      = av_frame_alloc();
    d->filt_frame = av_frame_alloc();
    if (!d->pkt || !d->frame || !d->filt_frame) {
        av_packet_free(&d->pkt);
        av_frame_free(&d->frame);
        av_frame_free(&d->filt_frame);
        free(d);
        return NULL;
    }
    return d;
}

static void decoder_free(decoder_t *d) {
    if (!d) return;
    if (d->filter_graph) avfilter_graph_free(&d->filter_graph);
    if (d->codec_ctx)    avcodec_free_context(&d->codec_ctx);
    if (d->fmt_ctx)      avformat_close_input(&d->fmt_ctx);
    av_packet_free(&d->pkt);
    av_frame_free(&d->frame);
    av_frame_free(&d->filt_frame);
    av_frame_free(&d->pending_frame);  // safe if NULL
    free(d);
}

// Build filter graph: abuffer → aresample → aformat → abuffersink
// ReplayGain and EQ are both applied in the miniaudio callback.
static int decoder_build_filter_graph(decoder_t *d)
{
    if (d->filter_graph) {
        avfilter_graph_free(&d->filter_graph);
        d->filter_graph     = NULL;
        d->buffersrc_ctx    = NULL;
        d->buffersink_ctx   = NULL;
    }

    AVFilterGraph *graph = avfilter_graph_alloc();
    if (!graph) return AVERROR(ENOMEM);

    // --- abuffer source ---
    char src_args[256];
    AVRational tb = d->fmt_ctx->streams[d->audio_stream_idx]->time_base;
    snprintf(src_args, sizeof(src_args),
             "sample_rate=%d:sample_fmt=%s:channel_layout=0x%" PRIx64 ":time_base=%d/%d",
             d->codec_ctx->sample_rate,
             av_get_sample_fmt_name(d->codec_ctx->sample_fmt),
             (uint64_t)d->codec_ctx->ch_layout.u.mask,
             tb.num, tb.den);

    AVFilterContext *src_ctx = NULL;
    int ret = avfilter_graph_create_filter(&src_ctx,
                                           avfilter_get_by_name("abuffer"),
                                           "in", src_args, NULL, graph);
    if (ret < 0) goto fail;

    // --- abuffersink (fixed output: f32 interleaved, stereo, 48 kHz) ---
    AVFilterContext *sink_ctx = NULL;
    ret = avfilter_graph_create_filter(&sink_ctx,
                                       avfilter_get_by_name("abuffersink"),
                                       "out", NULL, NULL, graph);
    if (ret < 0) goto fail;

    // Output format constraints are enforced via the aformat filter in the chain
    // below rather than via deprecated av_opt_set_int_list on the sink.

    // --- Build filter chain string ---
    // ReplayGain and EQ are applied in the miniaudio callback; only resample + format here.
    const char *chain = "aresample=" AVPLAYER_SAMPLE_RATE_STR
                        ",aformat=sample_fmts=flt:channel_layouts=stereo";

    // Link: in → chain → out
    AVFilterInOut *outputs = avfilter_inout_alloc();
    AVFilterInOut *inputs  = avfilter_inout_alloc();
    if (!outputs || !inputs) {
        avfilter_inout_free(&outputs);
        avfilter_inout_free(&inputs);
        ret = AVERROR(ENOMEM);
        goto fail;
    }
    outputs->name       = av_strdup("in");
    outputs->filter_ctx = src_ctx;
    outputs->pad_idx    = 0;
    outputs->next       = NULL;

    inputs->name        = av_strdup("out");
    inputs->filter_ctx  = sink_ctx;
    inputs->pad_idx     = 0;
    inputs->next        = NULL;

    ret = avfilter_graph_parse_ptr(graph, chain, &inputs, &outputs, NULL);
    avfilter_inout_free(&outputs);
    avfilter_inout_free(&inputs);
    if (ret < 0) goto fail;

    ret = avfilter_graph_config(graph, NULL);
    if (ret < 0) goto fail;

    d->filter_graph   = graph;
    d->buffersrc_ctx  = src_ctx;
    d->buffersink_ctx = sink_ctx;
    return 0;

fail:
    avfilter_graph_free(&graph);
    return ret;
}

// Look up a metadata key in container-level and stream-level dicts.
// Tries uppercase first, then lowercase.  Returns the entry or NULL.
static AVDictionaryEntry *rg_dict_get(AVFormatContext *fmt_ctx, int stream_idx,
                                       const char *key_upper, const char *key_lower)
{
    AVDictionary *dicts[2] = {
        fmt_ctx->metadata,
        fmt_ctx->streams[stream_idx]->metadata
    };
    for (int i = 0; i < 2; i++) {
        if (!dicts[i]) continue;
        AVDictionaryEntry *e = av_dict_get(dicts[i], key_upper, NULL, 0);
        if (!e) e = av_dict_get(dicts[i], key_lower, NULL, 0);
        if (e) return e;
    }
    return NULL;
}

// Read all four ReplayGain tags from d->fmt_ctx into d->rg_* fields.
// Defaults: gain = 0.0 dB, peak = 1.0 (linear).
static void decoder_read_rg_tags(decoder_t *d) {
    d->rg_track_gain_db = 0.0;
    d->rg_album_gain_db = 0.0;
    d->rg_track_peak    = 1.0f;
    d->rg_album_peak    = 1.0f;

    AVDictionaryEntry *e;
    // Gain values: "−6.23 dB" — atof stops at the non-numeric ' ' or 'd'
    e = rg_dict_get(d->fmt_ctx, d->audio_stream_idx,
                    "REPLAYGAIN_TRACK_GAIN", "replaygain_track_gain");
    if (e) d->rg_track_gain_db = atof(e->value);

    e = rg_dict_get(d->fmt_ctx, d->audio_stream_idx,
                    "REPLAYGAIN_ALBUM_GAIN", "replaygain_album_gain");
    if (e) d->rg_album_gain_db = atof(e->value);

    // Peak values: linear 0..1 (e.g. "0.988312")
    e = rg_dict_get(d->fmt_ctx, d->audio_stream_idx,
                    "REPLAYGAIN_TRACK_PEAK", "replaygain_track_peak");
    if (e) d->rg_track_peak = (float)atof(e->value);

    e = rg_dict_get(d->fmt_ctx, d->audio_stream_idx,
                    "REPLAYGAIN_ALBUM_PEAK", "replaygain_album_peak");
    if (e) d->rg_album_peak = (float)atof(e->value);
}

// Compute the linear RG gain multiplier for the given decoder and settings.
// rg_mode: 0=off, 1=track, 2=album (falls back to track if album tag absent).
static float compute_rg_gain(const decoder_t *d, int rg_mode, int prevent_clip, double preamp_db) {
    if (!d || rg_mode == 0) return 1.0f;

    double gain_db;
    float  peak;
    if (rg_mode == 2 && d->rg_album_gain_db != 0.0) {
        gain_db = d->rg_album_gain_db;
        peak    = d->rg_album_peak;
    } else {
        gain_db = d->rg_track_gain_db;
        peak    = d->rg_track_peak;
    }

    gain_db += preamp_db;
    float linear = (float)pow(10.0, gain_db / 20.0);

    if (prevent_clip && peak > 0.0f) {
        float max_gain = 1.0f / peak;
        if (linear > max_gain) linear = max_gain;
    }
    return linear;
}

// Open a URL; populate dec->fmt_ctx, codec_ctx, audio_stream_idx, duration.
static int decoder_open(decoder_t *d, const char *url) {
    // Tune for network streams (generous probe)
    AVDictionary *opts = NULL;
    av_dict_set(&opts, "timeout", "10000000", 0);    // 10s connect timeout
    av_dict_set(&opts, "reconnect", "1", 0);
    av_dict_set(&opts, "reconnect_streamed", "1", 0);
    av_dict_set(&opts, "reconnect_delay_max", "5", 0);
    av_dict_set(&opts, "user_agent", "supersonic/1.0", 0);

    int ret = avformat_open_input(&d->fmt_ctx, url, NULL, &opts);
    av_dict_free(&opts);
    if (ret < 0) return ret;

    ret = avformat_find_stream_info(d->fmt_ctx, NULL);
    if (ret < 0) { avformat_close_input(&d->fmt_ctx); return ret; }

    const AVCodec *codec = NULL;
    int stream_idx = av_find_best_stream(d->fmt_ctx, AVMEDIA_TYPE_AUDIO, -1, -1, &codec, 0);
    if (stream_idx < 0) {
        avformat_close_input(&d->fmt_ctx);
        return stream_idx;
    }
    d->audio_stream_idx = stream_idx;

    d->codec_ctx = avcodec_alloc_context3(codec);
    if (!d->codec_ctx) { avformat_close_input(&d->fmt_ctx); return AVERROR(ENOMEM); }

    ret = avcodec_parameters_to_context(d->codec_ctx, d->fmt_ctx->streams[stream_idx]->codecpar);
    if (ret < 0) goto fail;

    ret = avcodec_open2(d->codec_ctx, codec, NULL);
    if (ret < 0) goto fail;

    // Ensure we have a valid channel layout
    if (!d->codec_ctx->ch_layout.nb_channels) {
        av_channel_layout_default(&d->codec_ctx->ch_layout, 2);
    }
    if (!d->codec_ctx->ch_layout.u.mask) {
        // Convert from nb_channels if mask is 0
        AVChannelLayout tmp = AV_CHANNEL_LAYOUT_STEREO;
        if (d->codec_ctx->ch_layout.nb_channels == 1) {
            tmp = (AVChannelLayout)AV_CHANNEL_LAYOUT_MONO;
        }
        av_channel_layout_copy(&d->codec_ctx->ch_layout, &tmp);
    }

    double dur_secs = 0.0;
    if (d->fmt_ctx->duration > 0) {
        dur_secs = (double)d->fmt_ctx->duration / AV_TIME_BASE;
    }
    d->duration = dur_secs;

    strncpy(d->url, url, sizeof(d->url) - 1);
    decoder_read_rg_tags(d);
    return 0;

fail:
    avcodec_free_context(&d->codec_ctx);
    avformat_close_input(&d->fmt_ctx);
    return ret;
}

// --------------------------------------------------------------------------
// miniaudio callback
// --------------------------------------------------------------------------

static void ma_data_callback(ma_device *device, void *output, const void *input, ma_uint32 frame_count) {
    (void)input;
    av_player_t *p = (av_player_t *)device->pUserData;
    float *out = (float *)output;

    if (atomic_load(&p->state) != AVPLAYER_STATE_PLAYING) {
        // Silence output when paused or stopped
        memset(out, 0, frame_count * AVPLAYER_CHANNELS * sizeof(float));
        return;
    }

    int got = ring_read(&p->ring, out, (int)frame_count);
    if (got > 0)
        atomic_fetch_add_explicit((_Atomic long long *)&p->frames_played_total,
                                  (long long)got, memory_order_relaxed);

    // Compute peaks from pre-volume PCM (represents the demuxed/decoded signal).
    if (p->peaks_enabled && got > 0) {
        float l_max = 0.0f, r_max = 0.0f;
        float l_sq = 0.0f, r_sq = 0.0f;
        for (int i = 0; i < got; i++) {
            float l = out[i * AVPLAYER_CHANNELS + 0];
            float r = out[i * AVPLAYER_CHANNELS + 1];
            float al = l < 0.0f ? -l : l;
            float ar = r < 0.0f ? -r : r;
            if (al > l_max) l_max = al;
            if (ar > r_max) r_max = ar;
            l_sq += l * l;
            r_sq += r * r;
        }
        double lp = (l_max > 0.0f) ? 20.0 * log10((double)l_max) : -INFINITY;
        double rp = (r_max > 0.0f) ? 20.0 * log10((double)r_max) : -INFINITY;
        double lr = (l_sq > 0.0f) ? 10.0 * log10((double)(l_sq / got)) : -INFINITY;
        double rr = (r_sq > 0.0f) ? 10.0 * log10((double)(r_sq / got)) : -INFINITY;
        atomic_store_explicit((_Atomic double *)&p->l_peak, lp, memory_order_relaxed);
        atomic_store_explicit((_Atomic double *)&p->r_peak, rp, memory_order_relaxed);
        atomic_store_explicit((_Atomic double *)&p->l_rms,  lr, memory_order_relaxed);
        atomic_store_explicit((_Atomic double *)&p->r_rms,  rr, memory_order_relaxed);
    } else if (got == 0) {
        atomic_store_explicit((_Atomic double *)&p->l_peak, -INFINITY, memory_order_relaxed);
        atomic_store_explicit((_Atomic double *)&p->r_peak, -INFINITY, memory_order_relaxed);
        atomic_store_explicit((_Atomic double *)&p->l_rms,  -INFINITY, memory_order_relaxed);
        atomic_store_explicit((_Atomic double *)&p->r_rms,  -INFINITY, memory_order_relaxed);
    }

    // Apply EQ preamp + bands (after peaks, before volume — peak meter reflects raw decoded signal)
    if (got > 0) {
        int eq_idx = atomic_load_explicit(&p->eq_active_idx, memory_order_acquire);
        eq_bank_t *eq = &p->eq_banks[eq_idx];
        if (eq->preamp != 1.0f && eq->preamp > 0.0f) {
            for (int i = 0; i < got * AVPLAYER_CHANNELS; i++)
                out[i] *= eq->preamp;
        }
        if (eq->initialized && eq->num_bands > 0) {
            for (int b = 0; b < eq->num_bands; b++) {
                ma_peak2_process_pcm_frames(&eq->filters[b], out, out, (ma_uint64)got);
            }
        }
    }

    // Switch RG gain at track boundary if pending
    if (atomic_load_explicit(&p->rg_switch_pending, memory_order_acquire)) {
        long long fp = atomic_load_explicit(
            (_Atomic long long *)&p->frames_played_total, memory_order_relaxed);
        if (fp >= atomic_load_explicit(&p->rg_switch_threshold, memory_order_relaxed)) {
            atomic_store_explicit(&p->rg_gain,
                atomic_load_explicit(&p->rg_gain_pending, memory_order_relaxed),
                memory_order_relaxed);
            atomic_store_explicit(&p->rg_switch_pending, 0, memory_order_relaxed);
        }
    }

    // Apply ReplayGain
    if (got > 0) {
        float rg = atomic_load_explicit(&p->rg_gain, memory_order_relaxed);
        if (rg != 1.0f && rg > 0.0f) {
            for (int i = 0; i < got * AVPLAYER_CHANNELS; i++)
                out[i] *= rg;
        }
    }

    // Apply volume
    float vol = p->volume;
    if (vol < 0.9999f || vol > 1.0001f) {
        for (int i = 0; i < got * AVPLAYER_CHANNELS; i++) {
            out[i] *= vol;
        }
    }
    // Fill remainder with silence
    if (got < (int)frame_count) {
        memset(out + got * AVPLAYER_CHANNELS, 0,
               (frame_count - got) * AVPLAYER_CHANNELS * sizeof(float));
    }
}

// --------------------------------------------------------------------------
// Device management helpers
// --------------------------------------------------------------------------

static int player_init_device(av_player_t *p, const char *device_name, int exclusive) {
    if (p->device_init) {
        ma_device_uninit(&p->device);
        p->device_init = 0;
    }

    ma_device_config cfg = ma_device_config_init(ma_device_type_playback);
    cfg.playback.format   = ma_format_f32;
    cfg.playback.channels = AVPLAYER_CHANNELS;
    cfg.sampleRate        = AVPLAYER_SAMPLE_RATE;
    cfg.dataCallback      = ma_data_callback;
    cfg.pUserData         = p;
    cfg.periodSizeInMilliseconds = 10;

    if (exclusive) {
        cfg.playback.shareMode = ma_share_mode_exclusive;
    }

    if (device_name && device_name[0] != '\0') {
        // Find device by name
        ma_device_info *playback_infos;
        ma_uint32        playback_count;
        if (ma_context_get_devices(&p->ma_ctx,
                                    &playback_infos, &playback_count,
                                    NULL, NULL) == MA_SUCCESS) {
            for (ma_uint32 i = 0; i < playback_count; i++) {
                if (strcmp(playback_infos[i].name, device_name) == 0) {
                    cfg.playback.pDeviceID = &playback_infos[i].id;
                    break;
                }
            }
        }
    }

    if (ma_device_init(&p->ma_ctx, &cfg, &p->device) != MA_SUCCESS) {
        return -1;
    }
    p->device_init = 1;

    if (ma_device_start(&p->device) != MA_SUCCESS) {
        ma_device_uninit(&p->device);
        p->device_init = 0;
        return -1;
    }
    return 0;
}

// --------------------------------------------------------------------------
// Public API implementation
// --------------------------------------------------------------------------

av_player_t *av_player_create(void) {
    av_player_t *p = (av_player_t *)calloc(1, sizeof(av_player_t));
    if (!p) return NULL;
    ring_init(&p->ring);
    p->volume = 1.0f;
    atomic_store(&p->state, AVPLAYER_STATE_STOPPED);
    atomic_store(&p->l_peak, -INFINITY);
    atomic_store(&p->r_peak, -INFINITY);
    atomic_store(&p->l_rms,  -INFINITY);
    atomic_store(&p->r_rms,  -INFINITY);
    ma_mutex_init(&p->decoder_lock);
    ma_mutex_init(&p->filter_lock);
    return p;
}

int av_player_init(av_player_t *p, const char *device_name, int exclusive) {
    if (ma_context_init(NULL, 0, NULL, &p->ma_ctx) != MA_SUCCESS) {
        return -1;
    }
    p->ma_ctx_init = 1;
    return player_init_device(p, device_name, exclusive);
}

void av_player_destroy(av_player_t *p) {
    if (!p) return;
    atomic_store(&p->state, AVPLAYER_STATE_STOPPED);
    if (p->device_init) {
        ma_device_uninit(&p->device);
    }
    if (p->ma_ctx_init) {
        ma_context_uninit(&p->ma_ctx);
    }
    decoder_free(p->dec);
    decoder_free(p->next_dec);
    ring_free(&p->ring);
    ma_mutex_uninit(&p->decoder_lock);
    ma_mutex_uninit(&p->filter_lock);
    // Clean up EQ banks
    for (int b = 0; b < 2; b++) {
        if (p->eq_banks[b].initialized) {
            for (int i = 0; i < p->eq_banks[b].num_bands; i++)
                ma_peak2_uninit(&p->eq_banks[b].filters[i], NULL);
        }
    }
    free(p);
}

int av_player_open(av_player_t *p, const char *url, double start_time)
{
    // Stop current playback
    atomic_store(&p->state, AVPLAYER_STATE_STOPPED);
    ring_clear(&p->ring);
    atomic_store(&p->eof_reached, 0);
    atomic_store(&p->next_consumed, 0);
    atomic_store(&p->pending_track_change, 0);
    atomic_store_explicit((_Atomic long long *)&p->frames_played_total, 0LL, memory_order_relaxed);
    atomic_store_explicit((_Atomic long long *)&p->ring.frames_written_total, 0LL, memory_order_relaxed);
    atomic_store_explicit((_Atomic double *)&p->position_offset, 0.0, memory_order_relaxed);
    atomic_store_explicit((_Atomic long long *)&p->position_clock_ref, 0LL, memory_order_relaxed);
    memset(&p->bitrate_hist, 0, sizeof(p->bitrate_hist));

    // Acquire decoder_lock to prevent racing with av_player_open_next which
    // also reads/frees next_dec under this lock.
    ma_mutex_lock(&p->decoder_lock);
    decoder_t *old_next = p->next_dec;
    p->next_dec = NULL;
    ma_mutex_unlock(&p->decoder_lock);
    decoder_free(old_next);

    decoder_free(p->dec);
    p->dec = NULL;

    p->last_icy_title[0] = '\0';

    decoder_t *d = decoder_alloc();
    if (!d) return AVERROR(ENOMEM);

    int ret = decoder_open(d, url);
    if (ret < 0) { decoder_free(d); return ret; }

    ret = decoder_build_filter_graph(d);
    if (ret < 0) { decoder_free(d); return ret; }

    // Populate media info
    memset(&p->media_info, 0, sizeof(p->media_info));
    strncpy(p->media_info.codec,
            d->codec_ctx->codec->name,
            sizeof(p->media_info.codec) - 1);
    p->media_info.sample_rate = d->codec_ctx->sample_rate;
    p->media_info.channels    = d->codec_ctx->ch_layout.nb_channels;

    atomic_store_explicit((_Atomic double *)&p->duration, d->duration, memory_order_relaxed);
    atomic_store_explicit((_Atomic double *)&p->time_pos, 0.0, memory_order_relaxed);

    p->dec = d;

    // Compute and apply RG gain from the newly opened track's tags.
    atomic_store(&p->rg_switch_pending, 0);
    atomic_store_explicit(&p->rg_gain,
        compute_rg_gain(d, p->rg_mode, p->rg_prevent_clip, p->rg_preamp_db),
        memory_order_relaxed);

    if (start_time > 0.0) {
        av_player_seek(p, start_time);
    }

    atomic_store(&p->state, AVPLAYER_STATE_PLAYING);
    return 0;
}

int av_player_open_next(av_player_t *p, const char *url)
{
    // Atomically take ownership of the old next decoder so that the decode
    // goroutine cannot simultaneously free it during a gapless swap.
    ma_mutex_lock(&p->decoder_lock);
    decoder_t *old_next = p->next_dec;
    p->next_dec = NULL;
    ma_mutex_unlock(&p->decoder_lock);

    decoder_free(old_next);  // free outside the lock — can be slow
    atomic_store(&p->next_consumed, 0);

    if (!url || url[0] == '\0') return 0;

    decoder_t *d = decoder_alloc();
    if (!d) return AVERROR(ENOMEM);

    int ret = decoder_open(d, url);
    if (ret < 0) { decoder_free(d); return ret; }

    ret = decoder_build_filter_graph(d);
    if (ret < 0) { decoder_free(d); return ret; }

    // Publish the new decoder atomically.
    ma_mutex_lock(&p->decoder_lock);
    p->next_dec = d;
    ma_mutex_unlock(&p->decoder_lock);
    return 0;
}

void av_player_stop(av_player_t *p) {
    atomic_store(&p->state, AVPLAYER_STATE_STOPPED);
    ring_clear(&p->ring);
    atomic_store(&p->eof_reached, 0);
    atomic_store(&p->pending_track_change, 0);
    decoder_free(p->dec);   p->dec      = NULL;

    ma_mutex_lock(&p->decoder_lock);
    decoder_t *old_next = p->next_dec;
    p->next_dec = NULL;
    ma_mutex_unlock(&p->decoder_lock);
    decoder_free(old_next);
    atomic_store_explicit((_Atomic double *)&p->time_pos, 0.0, memory_order_relaxed);
    atomic_store_explicit((_Atomic double *)&p->duration,  0.0, memory_order_relaxed);
}

void av_player_pause(av_player_t *p) {
    if (atomic_load(&p->state) == AVPLAYER_STATE_PLAYING) {
        atomic_store(&p->state, AVPLAYER_STATE_PAUSED);
    }
}

void av_player_resume(av_player_t *p) {
    if (atomic_load(&p->state) == AVPLAYER_STATE_PAUSED) {
        atomic_store(&p->state, AVPLAYER_STATE_PLAYING);
    }
}

int av_player_seek(av_player_t *p, double seconds) {
    if (!p->dec) return -1;
    AVFormatContext *fc = p->dec->fmt_ctx;
    int stream_idx = p->dec->audio_stream_idx;

    int64_t ts = (int64_t)(seconds * AV_TIME_BASE);
    int ret = avformat_seek_file(fc, -1, INT64_MIN, ts, INT64_MAX, 0);
    if (ret < 0) return ret;

    avcodec_flush_buffers(p->dec->codec_ctx);

    // Flush filter graph by draining and sending NULL frame
    (void)av_buffersrc_add_frame_flags(p->dec->buffersrc_ctx, NULL, AV_BUFFERSRC_FLAG_PUSH);
    av_frame_unref(p->dec->filt_frame);
    while (av_buffersink_get_frame(p->dec->buffersink_ctx, p->dec->filt_frame) >= 0) {
        av_frame_unref(p->dec->filt_frame);
    }
    // Reinit the filter graph to avoid corruption
    decoder_build_filter_graph(p->dec);

    // Discard any pending frame from before the seek.
    av_frame_free(&p->dec->pending_frame);

    ring_clear(&p->ring);
    atomic_store(&p->eof_reached, 0);
    atomic_store(&p->pending_track_change, 0);
    // Reset position clock to the seek target.
    long long played_now = atomic_load_explicit(
        (_Atomic long long *)&p->frames_played_total, memory_order_relaxed);
    atomic_store_explicit((_Atomic double *)&p->position_offset, seconds, memory_order_relaxed);
    atomic_store_explicit((_Atomic long long *)&p->position_clock_ref, played_now, memory_order_relaxed);
    // Realign frames_written_total to frames_played_total so post-seek bitrate
    // history entries (frame_pos >= played_now) are immediately visible to the
    // lookup in av_player_get_media_info without waiting for the gap to drain.
    atomic_store_explicit((_Atomic long long *)&p->ring.frames_written_total,
                          played_now, memory_order_relaxed);
    memset(&p->bitrate_hist, 0, sizeof(p->bitrate_hist));
    atomic_store(&p->rg_switch_pending, 0);
    return 0;
}

void av_player_set_volume(av_player_t *p, float volume) {
    p->volume = volume;
}

void av_player_set_replay_gain(av_player_t *p, int mode, int prevent_clip, double preamp_db) {
    p->rg_mode         = mode;
    p->rg_prevent_clip = prevent_clip;
    p->rg_preamp_db    = preamp_db;
    // Clear any pending track-boundary switch; mode change applies immediately.
    atomic_store(&p->rg_switch_pending, 0);
    atomic_store_explicit(&p->rg_gain,
        compute_rg_gain(p->dec, mode, prevent_clip, preamp_db),
        memory_order_release);
}

void av_player_set_eq(av_player_t *p, const av_eq_band_t *bands, int num_bands, double preamp_db) {
    if (num_bands > AVPLAYER_MAX_EQ_BANDS)
        num_bands = AVPLAYER_MAX_EQ_BANDS;

    int active = atomic_load_explicit(&p->eq_active_idx, memory_order_relaxed);
    int target = 1 - active;
    eq_bank_t *bank = &p->eq_banks[target];

    // Uninit stale filters in the target bank (deferred cleanup from previous swap)
    if (bank->initialized) {
        for (int i = 0; i < bank->num_bands; i++)
            ma_peak2_uninit(&bank->filters[i], NULL);
        bank->initialized = 0;
        bank->num_bands = 0;
    }

    bank->preamp = (preamp_db != 0.0) ? (float)pow(10.0, preamp_db / 20.0) : 1.0f;

    if (num_bands == 0 || !bands) {
        bank->num_bands = 0;
        atomic_store_explicit(&p->eq_active_idx, target, memory_order_release);
        return;
    }

    for (int i = 0; i < num_bands; i++) {
        ma_peak2_config cfg = ma_peak2_config_init(
            ma_format_f32, AVPLAYER_CHANNELS, AVPLAYER_SAMPLE_RATE,
            bands[i].gain_db, bands[i].q, bands[i].frequency);
        if (ma_peak2_init(&cfg, NULL, &bank->filters[i]) != MA_SUCCESS) {
            // Roll back filters already initialised in this bank
            for (int j = 0; j < i; j++)
                ma_peak2_uninit(&bank->filters[j], NULL);
            bank->num_bands = 0;
            return;  // keep old EQ active
        }
    }
    bank->num_bands = num_bands;
    bank->initialized = 1;

    atomic_store_explicit(&p->eq_active_idx, target, memory_order_release);
}

int av_player_get_state(av_player_t *p) {
    return atomic_load(&p->state);
}

double av_player_get_position(av_player_t *p) {
    double offset = atomic_load_explicit((_Atomic double *)&p->position_offset, memory_order_relaxed);
    long long ref  = atomic_load_explicit((_Atomic long long *)&p->position_clock_ref, memory_order_relaxed);
    long long played = atomic_load_explicit((_Atomic long long *)&p->frames_played_total, memory_order_relaxed);
    double result = offset + (double)(played - ref) / AVPLAYER_SAMPLE_RATE;
    return result < 0.0 ? 0.0 : result;
}

double av_player_get_duration(av_player_t *p) {
    return atomic_load_explicit((_Atomic double *)&p->duration, memory_order_relaxed);
}

int av_player_buffered_frames(av_player_t *p) {
    return ring_avail(&p->ring);
}

void av_player_get_peaks(av_player_t *p,
                         double *l_peak, double *r_peak,
                         double *l_rms,  double *r_rms)
{
    *l_peak = atomic_load_explicit((_Atomic double *)&p->l_peak, memory_order_relaxed);
    *r_peak = atomic_load_explicit((_Atomic double *)&p->r_peak, memory_order_relaxed);
    *l_rms  = atomic_load_explicit((_Atomic double *)&p->l_rms,  memory_order_relaxed);
    *r_rms  = atomic_load_explicit((_Atomic double *)&p->r_rms,  memory_order_relaxed);
}

void av_player_set_peaks_enabled(av_player_t *p, int enabled) {
    p->peaks_enabled = enabled;
    if (!enabled) {
        atomic_store_explicit((_Atomic double *)&p->l_peak, -INFINITY, memory_order_relaxed);
        atomic_store_explicit((_Atomic double *)&p->r_peak, -INFINITY, memory_order_relaxed);
        atomic_store_explicit((_Atomic double *)&p->l_rms,  -INFINITY, memory_order_relaxed);
        atomic_store_explicit((_Atomic double *)&p->r_rms,  -INFINITY, memory_order_relaxed);
    }
}

// Check whether the ICY StreamTitle in the current decoder's metadata has changed
// since the last call.  If it has, copies the new title into buf (up to buflen-1
// bytes, NUL-terminated) and returns 1.  Returns 0 if unchanged or unavailable.
// Only call from the decode goroutine.
int av_player_check_icy_title(av_player_t *p, char *buf, int buflen) {
    if (buflen <= 0) return 0;
    const char *title = "";
    if (p->dec && p->dec->fmt_ctx) {
        AVDictionaryEntry *e = av_dict_get(p->dec->fmt_ctx->metadata, "StreamTitle", NULL, 0);
        if (e && e->value) title = e->value;
    }
    if (strcmp(title, p->last_icy_title) != 0) {
        strncpy(p->last_icy_title, title, sizeof(p->last_icy_title) - 1);
        p->last_icy_title[sizeof(p->last_icy_title) - 1] = '\0';
        strncpy(buf, title, buflen - 1);
        buf[buflen - 1] = '\0';
        return 1;
    }
    return 0;
}

void av_player_get_media_info(av_player_t *p, av_media_info_t *info) {
    *info = p->media_info;

    // Return the bitrate entry whose frame position is closest to (but not
    // exceeding) frames_played_total, so the displayed bitrate tracks the
    // audio currently being written to the sound card rather than the most
    // recently decoded packet (~ring-buffer-duration ahead).
    long long played = atomic_load_explicit(
        (_Atomic long long *)&p->frames_played_total, memory_order_relaxed);

    bitrate_history_t *h = &p->bitrate_hist;
    int wi = atomic_load_explicit(&h->write_idx, memory_order_acquire);

    int best_bitrate = atomic_load(&p->cur_bitrate); // fallback if history empty
    for (int i = 0; i < BITRATE_HISTORY_SIZE; i++) {
        int idx = (wi - 1 - i + BITRATE_HISTORY_SIZE) % BITRATE_HISTORY_SIZE;
        if (h->entries[idx].frame_pos > 0 && h->entries[idx].frame_pos <= played) {
            best_bitrate = h->entries[idx].bitrate;
            break;
        }
    }

    info->bitrate = best_bitrate;
}

int av_player_list_devices(av_device_info_t *devices, int max_devices) {
    // This function can be called before player init; create a temporary context
    ma_context ctx;
    if (ma_context_init(NULL, 0, NULL, &ctx) != MA_SUCCESS) return 0;

    ma_device_info *playback_infos;
    ma_uint32        playback_count;
    int              count = 0;

    if (ma_context_get_devices(&ctx, &playback_infos, &playback_count, NULL, NULL) == MA_SUCCESS) {
        for (ma_uint32 i = 0; i < playback_count && count < max_devices; i++) {
            strncpy(devices[count].name, playback_infos[i].name, 255);
            strncpy(devices[count].description, playback_infos[i].name, 255);
            count++;
        }
    }

    ma_context_uninit(&ctx);
    return count;
}

int av_player_set_device(av_player_t *p, const char *device_name) {
    int state = atomic_load(&p->state);
    // Stop the device, reinit with new device, restart
    ma_device_uninit(&p->device);
    p->device_init = 0;
    int ret = player_init_device(p, device_name, 0);
    // Restore play state (the audio callback will handle the ring buffer)
    (void)state;
    return ret;
}

int av_player_set_exclusive(av_player_t *p, int exclusive) {
    ma_device_uninit(&p->device);
    p->device_init = 0;
    return player_init_device(p, NULL, exclusive);
}

// --------------------------------------------------------------------------
// Decode step — called from Go goroutine in a tight loop
// --------------------------------------------------------------------------

// Attempt to write one filtered frame to the ring buffer.
//
// If d->pending_frame is set, we retry that frame first (it didn't fit last
// time).  Otherwise we pull the next frame from the filter sink.
//
// Returns:
//   0              — a frame was successfully written
//   AVERROR(EAGAIN) — ring full or no frame available from sink yet
//   other negative  — real error
static int drain_sink(av_player_t *p, decoder_t *d) {
    AVFrame *ff;

    if (d->pending_frame) {
        // Retry previously-stalled frame.
        ff = d->pending_frame;
    } else {
        // Pull next frame from filter graph — hold filter_lock so a concurrent
        // filter rebuild cannot free/replace buffersink_ctx mid-call.
        ff = d->filt_frame;
        ma_mutex_lock(&p->filter_lock);
        int ret = av_buffersink_get_frame(d->buffersink_ctx, ff);
        AVRational tb = (ff->pts != AV_NOPTS_VALUE)
                        ? av_buffersink_get_time_base(d->buffersink_ctx)
                        : (AVRational){0, 1};
        ma_mutex_unlock(&p->filter_lock);

        if (ret == AVERROR(EAGAIN) || ret == AVERROR_EOF) {
            return AVERROR(EAGAIN);  // nothing ready
        }
        if (ret < 0) return ret;

        // Update time position from this frame's pts.
        if (ff->pts != AV_NOPTS_VALUE) {
            double t = (double)ff->pts * av_q2d(tb);
            atomic_store_explicit((_Atomic double *)&p->time_pos, t, memory_order_relaxed);
        }
    }

    // Try to write to ring (all-or-nothing).
    int written = ring_write(&p->ring, (const float *)ff->data[0], ff->nb_samples);
    if (written == 0) {
        // Ring is full.  Save this frame to retry next decode step.
        if (!d->pending_frame) {
            // Move filt_frame into pending_frame (take ownership).
            d->pending_frame = av_frame_clone(ff);
            av_frame_unref(d->filt_frame);
        }
        // else: pending_frame already holds it, nothing to do.
        return AVERROR(EAGAIN);
    }

    // Frame written successfully; discard it.
    if (d->pending_frame) {
        av_frame_free(&d->pending_frame);  // sets to NULL
    } else {
        av_frame_unref(d->filt_frame);
    }
    return 0;
}

int av_player_decode_step(av_player_t *p) {
    // Check if a delayed track-change signal is ready to fire.
    if (atomic_load_explicit(&p->pending_track_change, memory_order_relaxed)) {
        long long played = atomic_load_explicit(
            (_Atomic long long *)&p->frames_played_total, memory_order_relaxed);
        if (played >= p->track_change_threshold) {
            atomic_store(&p->pending_track_change, 0);
            // Reset position clock so the new track starts from 0.
            atomic_store_explicit((_Atomic double *)&p->position_offset, 0.0, memory_order_relaxed);
            atomic_store_explicit((_Atomic long long *)&p->position_clock_ref, (long long)played, memory_order_relaxed);
            return AVPLAYER_DECODE_NEXT_READY;
        }
    }

    if (atomic_load(&p->state) == AVPLAYER_STATE_STOPPED) {
        return AVPLAYER_DECODE_STOPPED;
    }

    // If paused, check if ring buffer still has data (don't decode more)
    if (atomic_load(&p->state) == AVPLAYER_STATE_PAUSED) {
        return AVPLAYER_DECODE_RING_FULL;  // tell Go to sleep briefly
    }

    decoder_t *d = p->dec;
    if (!d) return AVPLAYER_DECODE_STOPPED;

    // If we have a pending frame (ring was full last time), try to flush it first.
    // If the ring is still too full, tell Go to sleep briefly.
    if (d->pending_frame) {
        int ret = drain_sink(p, d);
        if (ret == 0) return AVPLAYER_DECODE_OK;
        // pending_frame still set → ring still full
        return AVPLAYER_DECODE_RING_FULL;
    }

    // Also drain whatever the filter sink has ready (from the previous packet).
    {
        int ret = drain_sink(p, d);
        if (ret == 0) return AVPLAYER_DECODE_OK;
        // EAGAIN from sink means no frame ready — proceed to read a new packet.
        // But if the ring is nearly full, don't bother reading more right now.
        if (ring_space(&p->ring) < 512) {
            return AVPLAYER_DECODE_RING_FULL;
        }
    }

    // Check EOF already reached.  By this point pending_frame is NULL (the
    // check above returned early if it was set), so all old-track samples are
    // in the ring.  Swap to the next decoder immediately — before the ring
    // drains — so the ring stays filled and there is no audible gap.
    if (atomic_load(&p->eof_reached)) {
        // Gapless: atomically swap in next decoder while ring still has audio
        // data.  We hold the lock only for the pointer swap; the actual free
        // of the old decoder happens outside to keep the critical section short.
        decoder_t *next = NULL;
        decoder_t *old_dec = NULL;

        ma_mutex_lock(&p->decoder_lock);
        next = p->next_dec;
        if (next) {
            old_dec = p->dec;
            p->dec = next;
            p->next_dec = NULL;
        }
        ma_mutex_unlock(&p->decoder_lock);

        if (next) {
            decoder_free(old_dec);  // free outside the lock
            atomic_store(&p->eof_reached, 0);
            atomic_store(&p->next_consumed, 1);

            // Update duration / time for the new track
            atomic_store_explicit((_Atomic double *)&p->duration, p->dec->duration, memory_order_relaxed);
            atomic_store_explicit((_Atomic double *)&p->time_pos, 0.0, memory_order_relaxed);

            // Update media info
            memset(&p->media_info, 0, sizeof(p->media_info));
            strncpy(p->media_info.codec, p->dec->codec_ctx->codec->name, sizeof(p->media_info.codec) - 1);
            p->media_info.sample_rate = p->dec->codec_ctx->sample_rate;
            p->media_info.channels    = p->dec->codec_ctx->ch_layout.nb_channels;

            // Delay NEXT_READY until the new track's audio actually starts playing.
            // Use frames_played_total + ring.fill (not frames_written_total) so the
            // threshold is correct even if a prior seek broke the written-total
            // invariant via ring_clear.
            long long fp = atomic_load_explicit(
                (_Atomic long long *)&p->frames_played_total, memory_order_relaxed);
            int fill = atomic_load(&p->ring.fill);
            p->track_change_threshold = fp + (long long)fill;
            atomic_store(&p->pending_track_change, 1);
            // Schedule RG gain switch at the same boundary as the track change.
            atomic_store_explicit(&p->rg_gain_pending,
                compute_rg_gain(p->dec, p->rg_mode, p->rg_prevent_clip, p->rg_preamp_db),
                memory_order_relaxed);
            atomic_store_explicit(&p->rg_switch_threshold,
                (long long)p->track_change_threshold, memory_order_relaxed);
            atomic_store_explicit(&p->rg_switch_pending, 1, memory_order_release);
            // d is now stale (pointed at the freed old decoder); refresh it.
            d = p->dec;
        }
        // No next track: wait for ring to drain before signalling stopped.
        if (!next && ring_avail(&p->ring) == 0) {
            atomic_store(&p->state, AVPLAYER_STATE_STOPPED);
            return AVPLAYER_DECODE_STOPPED;
        }
        return AVPLAYER_DECODE_RING_FULL;
    }

    // Read and decode a packet
    int ret = av_read_frame(d->fmt_ctx, d->pkt);
    if (ret == AVERROR_EOF) {
        // Signal EOF to the decoder
        avcodec_send_packet(d->codec_ctx, NULL);
        atomic_store(&p->eof_reached, 1);

        // Drain decoder
        AVFrame *frame = d->frame;
        while (avcodec_receive_frame(d->codec_ctx, frame) >= 0) {
            ma_mutex_lock(&p->filter_lock);
            (void)av_buffersrc_add_frame_flags(d->buffersrc_ctx, frame, AV_BUFFERSRC_FLAG_PUSH);
            ma_mutex_unlock(&p->filter_lock);
            av_frame_unref(frame);
        }
        // Flush filter graph
        ma_mutex_lock(&p->filter_lock);
        (void)av_buffersrc_add_frame_flags(d->buffersrc_ctx, NULL, AV_BUFFERSRC_FLAG_PUSH);
        ma_mutex_unlock(&p->filter_lock);
        // Drain sink into ring
        while (drain_sink(p, d) == 0) {}

        return AVPLAYER_DECODE_EOF;
    }
    if (ret < 0) {
        return AVPLAYER_DECODE_ERROR;
    }

    if (d->pkt->stream_index != d->audio_stream_idx) {
        av_packet_unref(d->pkt);
        return AVPLAYER_DECODE_OK;
    }

    // Update instantaneous bitrate
    if (d->pkt->duration > 0) {
        AVRational tb = d->fmt_ctx->streams[d->audio_stream_idx]->time_base;
        double dur_secs = (double)d->pkt->duration * av_q2d(tb);
        if (dur_secs > 0) {
            int bps = (int)((double)d->pkt->size * 8.0 / dur_secs);
            atomic_store(&p->cur_bitrate, bps);
            p->media_info.bitrate = bps;

            // Push (frame_pos, bitrate) to history ring so GetMediaInfo can
            // return the bitrate matching the currently playing audio rather
            // than the most recently decoded packet.
            bitrate_history_t *h = &p->bitrate_hist;
            int wi = atomic_load_explicit(&h->write_idx, memory_order_relaxed);
            h->entries[wi].frame_pos = atomic_load_explicit(
                (_Atomic long long *)&p->ring.frames_written_total, memory_order_relaxed);
            h->entries[wi].bitrate = bps;
            atomic_store_explicit(&h->write_idx, (wi + 1) % BITRATE_HISTORY_SIZE,
                                  memory_order_release);
        }
    }

    ret = avcodec_send_packet(d->codec_ctx, d->pkt);
    av_packet_unref(d->pkt);
    if (ret < 0 && ret != AVERROR(EAGAIN)) return AVPLAYER_DECODE_ERROR;

    // Receive all decoded frames
    AVFrame *frame = d->frame;
    while ((ret = avcodec_receive_frame(d->codec_ctx, frame)) == 0) {
        ma_mutex_lock(&p->filter_lock);
        int r2 = av_buffersrc_add_frame_flags(d->buffersrc_ctx, frame, AV_BUFFERSRC_FLAG_PUSH);
        ma_mutex_unlock(&p->filter_lock);
        av_frame_unref(frame);
        if (r2 < 0) break;

        // Drain as many filtered frames as will fit in the ring right now.
        // Stop (leave pending_frame set) if the ring is full.
        while (drain_sink(p, d) == 0) {
            // Keep draining until sink is empty or ring is full.
        }
        // If we have a pending frame, the ring is full — stop reading more packets.
        if (d->pending_frame) break;
    }

    return AVPLAYER_DECODE_OK;
}
