#include "pw_wrapper.h"
#include <spa/param/audio/format-utils.h>
#include <spa/param/latency-utils.h>
#include <pipewire/pipewire.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>

// Go function
extern void process_channel_go(float *in, float *out, int samples, int sample_rate, int channel_index);
extern void log_from_c(char *msg);

// State listener callback
static void on_state_changed(void *data, enum pw_filter_state old, enum pw_filter_state state, const char *error) {
    char msg[256];
    snprintf(msg, sizeof(msg), "State change: %s -> %s", 
        pw_filter_state_as_string(old), 
        pw_filter_state_as_string(state));
    log_from_c(msg);
    
    if (error) {
        snprintf(msg, sizeof(msg), "Error: %s", error);
        log_from_c(msg);
    }
}

static void on_add_buffer(void *data, void *port_data, struct pw_buffer *buffer) {
    struct port_data *port = port_data;

    if (!port || port->direction != PW_DIRECTION_OUTPUT || !buffer) {
        return;
    }

    // Queue output buffers as soon as PipeWire creates them.
    pw_filter_queue_buffer(port_data, buffer);
}

// Callback function for processing audio
static void on_process(void *userdata, struct spa_io_position *position) {
    struct pw_filter_data *data = userdata;
    uint32_t n_samples;
    uint32_t sample_rate = 48000;
    static int process_cnt = 0;
    process_cnt++;

    if (position != NULL) {
        n_samples = position->clock.duration;
        if (position->clock.rate.denom > 0)
             sample_rate = position->clock.rate.denom;
    } else {
        return;
    }
    
    if (process_cnt < 20 || process_cnt % 100 == 0) {
        char msg[128];
        snprintf(msg, sizeof(msg), "Process %d: samples=%u rate=%u", process_cnt, n_samples, sample_rate);
        log_from_c(msg);
    }

    // Process each channel
    for (int i = 0; i < data->channels; i++) {
        struct pw_buffer *in_buf = pw_filter_dequeue_buffer(data->in_ports[i]);
        struct pw_buffer *out_buf = pw_filter_dequeue_buffer(data->out_ports[i]);
        
        if (process_cnt < 20) {
            char msg[128];
            snprintf(msg, sizeof(msg), "  CH%d: in=%p out=%p", i, in_buf, out_buf);
            log_from_c(msg);
        }

        if (out_buf == NULL) {
            if (process_cnt < 50 && process_cnt % 10 == 0) {
                 char msg[128];
                 snprintf(msg, sizeof(msg), "WARNING: CH%d Output buffer is NULL (Unconnected?)", i);
                 log_from_c(msg);
            }
            if (in_buf) pw_filter_queue_buffer(data->in_ports[i], in_buf);
            continue;
        }

        float *out = pw_filter_get_dsp_buffer(data->out_ports[i], n_samples);
        if (out == NULL) {
             pw_filter_queue_buffer(data->out_ports[i], out_buf);
             if (in_buf) pw_filter_queue_buffer(data->in_ports[i], in_buf);
             continue;
        }

        float *in = NULL;
        if (in_buf) {
            in = pw_filter_get_dsp_buffer(data->in_ports[i], n_samples);
        }

        if (in) {
            process_channel_go(in, out, n_samples, (int)sample_rate, i);
        } else {
            memset(out, 0, n_samples * sizeof(float));
            process_channel_go(out, out, n_samples, (int)sample_rate, i);
        }

        if (in_buf) pw_filter_queue_buffer(data->in_ports[i], in_buf);
        pw_filter_queue_buffer(data->out_ports[i], out_buf);
    }
}

static const struct pw_filter_events filter_events = {
    PW_VERSION_FILTER_EVENTS,
    .process = on_process,
    .state_changed = on_state_changed,
    .add_buffer = on_add_buffer,
};

// Helper to get channel name/position
static void get_channel_config(int i, int total, char *name, size_t max_len, uint32_t *pos) {
    if (total == 2) {
        if (i == 0) {
            snprintf(name, max_len, "FL");
            *pos = SPA_AUDIO_CHANNEL_FL;
        } else {
            snprintf(name, max_len, "FR");
            *pos = SPA_AUDIO_CHANNEL_FR;
        }
    } else if (total == 1) {
        snprintf(name, max_len, "MONO");
        *pos = SPA_AUDIO_CHANNEL_MONO;
    } else {
        snprintf(name, max_len, "CH%d", i+1);
        *pos = SPA_AUDIO_CHANNEL_MONO;
    }
}

struct pw_filter_data* create_pipewire_filter(struct pw_main_loop *loop, int channels) {
    if (!loop) return NULL;

    struct pw_filter_data *data = calloc(1, sizeof(struct pw_filter_data));
    data->loop = loop;
    data->channels = channels;

    data->context = pw_context_new(pw_main_loop_get_loop(loop), NULL, 0);
    if (!data->context) { free(data); return NULL; }

    data->core = pw_context_connect(data->context, NULL, 0);
    if (!data->core) {
        pw_context_destroy(data->context);
        free(data);
        return NULL;
    }

    struct pw_properties *props = pw_properties_new(
        PW_KEY_MEDIA_TYPE, "Audio",
        PW_KEY_MEDIA_CATEGORY, "Filter",
        PW_KEY_MEDIA_ROLE, "DSP",
        PW_KEY_NODE_NAME, "pw-comp",
        PW_KEY_NODE_DESCRIPTION, "Audio Compressor Filter",
        NULL
    );

    data->filter = pw_filter_new(data->core, "pw-comp-filter", props);
    if (!data->filter) {
        pw_core_disconnect(data->core);
        pw_context_destroy(data->context);
        free(data);
        return NULL;
    }

    pw_filter_add_listener(data->filter, &data->filter_listener, &filter_events, data);

    data->in_ports = calloc(channels, sizeof(struct port_data*));
    data->out_ports = calloc(channels, sizeof(struct port_data*));

    uint8_t buffer[1024];

    for (int i = 0; i < channels; i++) {
        char ch_name[32];
        uint32_t ch_pos;
        get_channel_config(i, channels, ch_name, sizeof(ch_name), &ch_pos);

        struct spa_pod_builder b = SPA_POD_BUILDER_INIT(buffer, sizeof(buffer));
        const struct spa_pod *params[1];

        // Format: 1 channel, F32 ONLY (Simplified), Rate Range, MONO Position
        uint32_t positions[1] = { ch_pos };
        
        params[0] = spa_pod_builder_add_object(&b,
            SPA_TYPE_OBJECT_Format, SPA_PARAM_EnumFormat,
            SPA_FORMAT_mediaType, SPA_POD_Id(SPA_MEDIA_TYPE_audio),
            SPA_FORMAT_mediaSubtype, SPA_POD_Id(SPA_MEDIA_SUBTYPE_raw),
            SPA_FORMAT_AUDIO_format, SPA_POD_Id(SPA_AUDIO_FORMAT_F32), // Strictly F32 Interleaved (1 ch = same as planar)
            SPA_FORMAT_AUDIO_rate, SPA_POD_CHOICE_RANGE_Int(48000, 1, 384000),
            SPA_FORMAT_AUDIO_channels, SPA_POD_Int(1),
            SPA_FORMAT_AUDIO_position, SPA_POD_Array(sizeof(uint32_t), SPA_TYPE_Id, 1, positions),
            0);

        char port_name[64];
        
        snprintf(port_name, sizeof(port_name), "input_%s", ch_name);
        data->in_ports[i] = pw_filter_add_port(data->filter,
            PW_DIRECTION_INPUT,
            PW_FILTER_PORT_FLAG_MAP_BUFFERS,
            sizeof(struct port_data),
            pw_properties_new(
                PW_KEY_PORT_NAME, port_name,
                PW_KEY_FORMAT_DSP, "32 bit float mono audio",
                PW_KEY_MEDIA_TYPE, "Audio",
                NULL),
            params, 1);

        if (!data->in_ports[i]) {
            destroy_pipewire_filter(data);
            return NULL;
        }

        data->in_ports[i]->direction = PW_DIRECTION_INPUT;
        data->in_ports[i]->channel = i;

        snprintf(port_name, sizeof(port_name), "output_%s", ch_name);
        data->out_ports[i] = pw_filter_add_port(data->filter,
            PW_DIRECTION_OUTPUT,
            PW_FILTER_PORT_FLAG_MAP_BUFFERS,
            sizeof(struct port_data),
            pw_properties_new(
                PW_KEY_PORT_NAME, port_name,
                PW_KEY_FORMAT_DSP, "32 bit float mono audio",
                PW_KEY_MEDIA_TYPE, "Audio",
                NULL),
            params, 1);

        if (!data->out_ports[i]) {
            destroy_pipewire_filter(data);
            return NULL;
        }

        data->out_ports[i]->direction = PW_DIRECTION_OUTPUT;
        data->out_ports[i]->channel = i;
    }

    struct spa_pod_builder b_lat = SPA_POD_BUILDER_INIT(buffer, sizeof(buffer));
    const struct spa_pod *connect_params[1];
    connect_params[0] = spa_process_latency_build(&b_lat,
        SPA_PARAM_ProcessLatency,
        &SPA_PROCESS_LATENCY_INFO_INIT(.ns = 1024 * SPA_NSEC_PER_SEC / 48000)); // ~21ms

    if (pw_filter_connect(data->filter, PW_FILTER_FLAG_RT_PROCESS, connect_params, 1) < 0) {
        char err_msg[] = "Failed to connect filter";
        log_from_c(err_msg);
        fprintf(stderr, "ERROR: %s\n", err_msg);
        destroy_pipewire_filter(data);
        return NULL;
    }

    return data;
}

void destroy_pipewire_filter(struct pw_filter_data* data) {
    if (!data) return;
    if (data->filter) pw_filter_destroy(data->filter);
    if (data->core) pw_core_disconnect(data->core);
    if (data->context) pw_context_destroy(data->context);
    
    if (data->in_ports) free(data->in_ports);
    if (data->out_ports) free(data->out_ports);
    free(data);
}
