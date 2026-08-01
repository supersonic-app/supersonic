// av_waveform.c — FFmpeg-based audio analysis for waveform image generation.
// Decodes audio, computes peak/RMS per chunk, writes results incrementally
// to shared memory that Go polls for progressive rendering.
// Does NOT include miniaudio (only av_player.c defines MA_IMPLEMENTATION).

#include "av_player.h"

#include <libavcodec/avcodec.h>
#include <libavformat/avformat.h>
#include <libavutil/channel_layout.h>
#include <libavutil/mem.h>
#include <libavutil/opt.h>
#include <libswresample/swresample.h>
#include <math.h>
#include <stdatomic.h>
#include <stdint.h>
#include <stdio.h>
#include <string.h>

#define ANALYZE_SAMPLE_RATE 22050
#define ANALYZE_CHANNELS    1
#define WAVEFORM_BINS       1024

// Feed `got` resampled s16 mono samples into the chunk accumulators.
// Finalizes chunks as they fill, writing to out_peak/out_rms and
// advancing *cur_chunk with an atomic release store to *progress.
static void process_samples(const int16_t *buf, int got, int samples_per_chunk,
                            int *cur_chunk, double *chunk_peak,
                            double *chunk_sum_sq, int *chunk_count,
                            uint8_t *out_peak, uint8_t *out_rms,
                            _Atomic int *progress) {
    for (int i = 0; i < got && *cur_chunk < WAVEFORM_BINS; i++) {
        double sample = (double)buf[i] / 32768.0;
        double abs_sample = fabs(sample);
        if (abs_sample > *chunk_peak) *chunk_peak = abs_sample;
        *chunk_sum_sq += sample * sample;
        (*chunk_count)++;

        if (*chunk_count >= samples_per_chunk) {
            double rms = sqrt(*chunk_sum_sq / *chunk_count);
            if (*chunk_peak > 1.0) *chunk_peak = 1.0;
            if (rms > 1.0) rms = 1.0;
            out_peak[*cur_chunk] = (uint8_t)(*chunk_peak * 255.0);
            out_rms[*cur_chunk]  = (uint8_t)(rms * 255.0);
            (*cur_chunk)++;
            atomic_store_explicit(progress, *cur_chunk, memory_order_release);
            *chunk_peak   = 0.0;
            *chunk_sum_sq = 0.0;
            *chunk_count  = 0;
        }
    }
}

int av_analyze_waveform(const char *in_url, int64_t duration_ms,
                        uint8_t *out_peak, uint8_t *out_rms,
                        _Atomic int *progress, _Atomic int *cancel) {
    int ret;
    char errbuf[256];

    // ---- Input -------------------------------------------------------
    AVDictionary *opts = NULL;
    av_dict_set(&opts, "timeout",             "10000000", 0);
    av_dict_set(&opts, "reconnect",           "1",        0);
    av_dict_set(&opts, "reconnect_streamed",  "1",        0);
    av_dict_set(&opts, "reconnect_delay_max", "5",        0);
    av_dict_set(&opts, "user_agent",          "supersonic/1.0", 0);

    AVFormatContext *in_fmt = NULL;
    ret = avformat_open_input(&in_fmt, in_url, NULL, &opts);
    av_dict_free(&opts);
    if (ret < 0) {
        av_strerror(ret, errbuf, sizeof(errbuf));
        fprintf(stderr, "[av_waveform] avformat_open_input failed: %s\n", errbuf);
        return ret;
    }

    ret = avformat_find_stream_info(in_fmt, NULL);
    if (ret < 0) goto close_in;

    const AVCodec *codec = NULL;
    int stream_idx = av_find_best_stream(in_fmt, AVMEDIA_TYPE_AUDIO, -1, -1, &codec, 0);
    if (stream_idx < 0) { ret = stream_idx; goto close_in; }

    AVCodecContext *dec_ctx = avcodec_alloc_context3(codec);
    if (!dec_ctx) { ret = AVERROR(ENOMEM); goto close_in; }

    avcodec_parameters_to_context(dec_ctx, in_fmt->streams[stream_idx]->codecpar);

    // Normalise channel layout (mirrors decoder_open in av_player.c)
    if (!dec_ctx->ch_layout.nb_channels)
        av_channel_layout_default(&dec_ctx->ch_layout, 2);
    if (!dec_ctx->ch_layout.u.mask) {
        AVChannelLayout tmp = (dec_ctx->ch_layout.nb_channels == 1)
            ? (AVChannelLayout)AV_CHANNEL_LAYOUT_MONO
            : (AVChannelLayout)AV_CHANNEL_LAYOUT_STEREO;
        av_channel_layout_copy(&dec_ctx->ch_layout, &tmp);
    }

    ret = avcodec_open2(dec_ctx, codec, NULL);
    if (ret < 0) goto free_dec;

    // ---- Resampler (to mono s16 at analysis rate) --------------------
    AVChannelLayout out_ch_layout = AV_CHANNEL_LAYOUT_MONO;
    SwrContext *swr = NULL;
    ret = swr_alloc_set_opts2(&swr,
        &out_ch_layout,      AV_SAMPLE_FMT_S16, ANALYZE_SAMPLE_RATE,
        &dec_ctx->ch_layout, dec_ctx->sample_fmt, dec_ctx->sample_rate,
        0, NULL);
    if (ret < 0) goto free_dec;
    ret = swr_init(swr);
    if (ret < 0) goto free_swr;

    // ---- Decode / resample / analyze loop ----------------------------
    int64_t total_samples = (int64_t)ANALYZE_SAMPLE_RATE * duration_ms / 1000;
    int samples_per_chunk = (int)(total_samples / WAVEFORM_BINS);
    if (samples_per_chunk < 1) samples_per_chunk = 1;

    AVPacket *pkt   = av_packet_alloc();
    AVFrame  *frame = av_frame_alloc();

    int cur_chunk = 0;
    double chunk_peak = 0.0;
    double chunk_sum_sq = 0.0;
    int chunk_count = 0;

    while (av_read_frame(in_fmt, pkt) >= 0) {
        if (atomic_load_explicit(cancel, memory_order_acquire)) {
            av_packet_unref(pkt);
            ret = AVERROR_EXIT;
            goto done;
        }
        if (pkt->stream_index != stream_idx) { av_packet_unref(pkt); continue; }
        if (avcodec_send_packet(dec_ctx, pkt) < 0) { av_packet_unref(pkt); continue; }
        av_packet_unref(pkt);

        while (avcodec_receive_frame(dec_ctx, frame) == 0) {
            int out_samples = swr_get_out_samples(swr, frame->nb_samples);
            if (out_samples <= 0) { av_frame_unref(frame); continue; }

            int16_t *buf = (int16_t *)av_malloc(out_samples * sizeof(int16_t));
            if (!buf) { av_frame_unref(frame); continue; }

            int got = swr_convert(swr,
                (uint8_t **)&buf, out_samples,
                (const uint8_t **)frame->data, frame->nb_samples);
            av_frame_unref(frame);
            if (got <= 0) { av_free(buf); continue; }

            process_samples(buf, got, samples_per_chunk, &cur_chunk,
                            &chunk_peak, &chunk_sum_sq, &chunk_count,
                            out_peak, out_rms, progress);
            av_free(buf);
        }
    }

    // Flush decoder
    avcodec_send_packet(dec_ctx, NULL);
    while (avcodec_receive_frame(dec_ctx, frame) == 0) {
        int out_samples = swr_get_out_samples(swr, frame->nb_samples);
        if (out_samples <= 0) { av_frame_unref(frame); continue; }

        int16_t *buf = (int16_t *)av_malloc(out_samples * sizeof(int16_t));
        if (!buf) { av_frame_unref(frame); continue; }

        int got = swr_convert(swr,
            (uint8_t **)&buf, out_samples,
            (const uint8_t **)frame->data, frame->nb_samples);
        av_frame_unref(frame);
        if (got <= 0) { av_free(buf); continue; }

        process_samples(buf, got, samples_per_chunk, &cur_chunk,
                        &chunk_peak, &chunk_sum_sq, &chunk_count,
                        out_peak, out_rms, progress);
        av_free(buf);
    }

    // Flush resampler
    for (;;) {
        int out_samples = swr_get_out_samples(swr, 0);
        if (out_samples <= 0) break;
        int16_t *buf = (int16_t *)av_malloc(out_samples * sizeof(int16_t));
        if (!buf) break;
        int got = swr_convert(swr, (uint8_t **)&buf, out_samples, NULL, 0);
        if (got <= 0) { av_free(buf); break; }

        process_samples(buf, got, samples_per_chunk, &cur_chunk,
                        &chunk_peak, &chunk_sum_sq, &chunk_count,
                        out_peak, out_rms, progress);
        av_free(buf);
    }

    // Handle final partial chunk
    if (cur_chunk < WAVEFORM_BINS && chunk_count > 0) {
        double rms = sqrt(chunk_sum_sq / chunk_count);
        if (chunk_peak > 1.0) chunk_peak = 1.0;
        if (rms > 1.0) rms = 1.0;
        out_peak[cur_chunk] = (uint8_t)(chunk_peak * 255.0);
        out_rms[cur_chunk]  = (uint8_t)(rms * 255.0);
        cur_chunk++;
        atomic_store_explicit(progress, cur_chunk, memory_order_release);
    }

    ret = 0;

done:
    av_packet_free(&pkt);
    av_frame_free(&frame);

free_swr:
    swr_free(&swr);
free_dec:
    avcodec_free_context(&dec_ctx);
close_in:
    avformat_close_input(&in_fmt);
    return ret;
}
