#include "pw_wrapper.h"
#include <spa/param/audio/format-utils.h>
#include <spa/param/latency-utils.h>
#include <pipewire/pipewire.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>

// Go function
extern void process_channel_go(float *in, float *out, int samples, int sample_rate, int channel_index);

// State listener callback
static void on_state_changed(void *data, enum pw_filter_state old, enum pw_filter_state state, const char *error) {
    fprintf(stderr, "Filter State Change: %s -> %s\n", 
        pw_filter_state_as_string(old), 
        pw_filter_state_as_string(state));
    if (error) {
        fprintf(stderr, "Filter Error: %s\n", error);
    }
}

// Callback function for processing audio
static void on_process(void *userdata, struct spa_io_position *position) {
    struct pw_filter_data *data = userdata;
    uint32_t n_samples;
    uint32_t sample_rate = 48000;

    if (position != NULL) {
        n_samples = position->clock.duration;
        if (position->clock.rate.denom > 0)
             sample_rate = position->clock.rate.denom;
    } else {
        return;
    }

    // Process each channel
    for (int i = 0; i < data->channels; i++) {
        struct pw_buffer *in_buf = pw_filter_dequeue_buffer(data->in_ports[i]->port);
        struct pw_buffer *out_buf = pw_filter_dequeue_buffer(data->out_ports[i]->port);

        // If output port is not connected or has no buffer, we can't output anything.
        // We still need to recycle input if we got it.
        if (out_buf == NULL) {
            if (in_buf) pw_filter_queue_buffer(data->in_ports[i]->port, in_buf);
            continue;
        }

        float *out = pw_filter_get_dsp_buffer(data->out_ports[i]->port, n_samples);
        if (out == NULL) {
             // Should not happen if out_buf is valid
             pw_filter_queue_buffer(data->out_ports[i]->port, out_buf);
             if (in_buf) pw_filter_queue_buffer(data->in_ports[i]->port, in_buf);
             continue;
        }

        float *in = NULL;
        if (in_buf) {
            in = pw_filter_get_dsp_buffer(data->in_ports[i]->port, n_samples);
        }

        if (in) {
            // Normal processing: In -> Out
            process_channel_go(in, out, n_samples, (int)sample_rate, i);
        } else {
            // Missing input: Treat as silence.
            // We fill output with zeros, then process it in-place.
            // This ensures the compressor's internal state (envelopes) decay naturally
            // and meters show silence instead of freezing.
            memset(out, 0, n_samples * sizeof(float));
            process_channel_go(out, out, n_samples, (int)sample_rate, i);
        }

        if (in_buf) pw_filter_queue_buffer(data->in_ports[i]->port, in_buf);
        pw_filter_queue_buffer(data->out_ports[i]->port, out_buf);
    }
}

static const struct pw_filter_events filter_events = {
    PW_VERSION_FILTER_EVENTS,
    .process = on_process,
    .state_changed = on_state_changed,
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
        *pos = SPA_AUDIO_CHANNEL_UNKNOWN;
    }
}

struct pw_filter_data* create_pipewire_filter(struct pw_main_loop *loop, int channels) {
    if (!loop) return NULL;

    struct pw_filter_data *data = calloc(1, sizeof(struct pw_filter_data));
    data->loop = loop;
    data->channels = channels;

    pw_init(NULL, NULL);

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

    // Allocate port arrays
    data->in_ports = calloc(channels, sizeof(struct port_data*));
    data->out_ports = calloc(channels, sizeof(struct port_data*));

    uint8_t buffer[1024];

    // Create ports for each channel
    for (int i = 0; i < channels; i++) {
        char ch_name[32];
        uint32_t ch_pos;
        get_channel_config(i, channels, ch_name, sizeof(ch_name), &ch_pos);

        struct spa_pod_builder b = SPA_POD_BUILDER_INIT(buffer, sizeof(buffer));
        const struct spa_pod *params[1];

        // Format for THIS port: 1 channel, specific position, ANY rate
        uint32_t positions[1] = { ch_pos };
        
        params[0] = spa_pod_builder_add_object(&b,
            SPA_TYPE_OBJECT_Format, SPA_PARAM_EnumFormat,
            SPA_FORMAT_mediaType, SPA_POD_Id(SPA_MEDIA_TYPE_audio),
            SPA_FORMAT_mediaSubtype, SPA_POD_Id(SPA_MEDIA_SUBTYPE_raw),
            SPA_FORMAT_AUDIO_format, SPA_POD_Id(SPA_AUDIO_FORMAT_F32),
            SPA_FORMAT_AUDIO_rate, SPA_POD_Int(0), 
            SPA_FORMAT_AUDIO_channels, SPA_POD_Int(1),
            SPA_FORMAT_AUDIO_position, SPA_POD_Array(sizeof(uint32_t), SPA_TYPE_Id, 1, positions),
            0);

        char port_name[64];
        
        // Input Port
        data->in_ports[i] = calloc(1, sizeof(struct port_data));
        snprintf(port_name, sizeof(port_name), "input_%s", ch_name);
        
        data->in_ports[i]->port = pw_filter_add_port(data->filter,
            PW_DIRECTION_INPUT,
            PW_FILTER_PORT_FLAG_MAP_BUFFERS,
            sizeof(struct port_data),
            pw_properties_new(
                PW_KEY_PORT_NAME, port_name,
                PW_KEY_FORMAT_DSP, "32 bit float mono audio",
                PW_KEY_MEDIA_TYPE, "Audio",
                NULL),
            params, 1);

        // Output Port
        data->out_ports[i] = calloc(1, sizeof(struct port_data));
        snprintf(port_name, sizeof(port_name), "output_%s", ch_name);

        data->out_ports[i]->port = pw_filter_add_port(data->filter,
            PW_DIRECTION_OUTPUT,
            PW_FILTER_PORT_FLAG_MAP_BUFFERS,
            sizeof(struct port_data),
            pw_properties_new(
                PW_KEY_PORT_NAME, port_name,
                PW_KEY_FORMAT_DSP, "32 bit float mono audio",
                PW_KEY_MEDIA_TYPE, "Audio",
                NULL),
            params, 1);
    }

    // Connect
    struct spa_pod_builder b_lat = SPA_POD_BUILDER_INIT(buffer, sizeof(buffer));
    const struct spa_pod *connect_params[1];
    connect_params[0] = spa_process_latency_build(&b_lat,
        SPA_PARAM_ProcessLatency,
        &SPA_PROCESS_LATENCY_INFO_INIT(.ns = 10 * SPA_NSEC_PER_MSEC));

    if (pw_filter_connect(data->filter, PW_FILTER_FLAG_RT_PROCESS, connect_params, 1) < 0) {
        fprintf(stderr, "ERROR: Failed to connect filter\n");
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
    
    if (data->in_ports) {
        for (int i=0; i<data->channels; i++) free(data->in_ports[i]);
        free(data->in_ports);
    }
    if (data->out_ports) {
        for (int i=0; i<data->channels; i++) free(data->out_ports[i]);
        free(data->out_ports);
    }
    free(data);
}