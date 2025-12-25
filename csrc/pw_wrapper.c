#include "pw_wrapper.h"
#include <spa/param/audio/format-utils.h>
#include <spa/param/latency-utils.h>
#include <pipewire/pipewire.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>

extern void process_audio_go(float *in, float *out, int samples); // Go function (handled via cgo)

// Callback function for processing audio, calls Go for DSP
static void on_process(void *userdata, struct spa_io_position *position) {
    struct pw_filter_data *data = userdata;
    struct pw_buffer *in_buf, *out_buf;
    float *in, *out;
    uint32_t n_samples;

    if (position == NULL)
        return;

    // Get number of samples to process
    n_samples = position->clock.duration;

    // Get input buffer
    in_buf = pw_filter_dequeue_buffer(data->in_port->port);
    if (in_buf == NULL) {
        fprintf(stderr, "Failed to dequeue input buffer\n");
        return;
    }

    // Get output buffer
    out_buf = pw_filter_dequeue_buffer(data->out_port->port);
    if (out_buf == NULL) {
        fprintf(stderr, "Failed to dequeue output buffer\n");
        pw_filter_queue_buffer(data->in_port->port, in_buf);
        return;
    }

    // Get DSP buffer pointers (32-bit float)
    in = pw_filter_get_dsp_buffer(data->in_port->port, n_samples);
    out = pw_filter_get_dsp_buffer(data->out_port->port, n_samples);

    if (in == NULL || out == NULL) {
        fprintf(stderr, "Failed to get DSP buffers\n");
        pw_filter_queue_buffer(data->in_port->port, in_buf);
        pw_filter_queue_buffer(data->out_port->port, out_buf);
        return;
    }

    // Call Go DSP function to process audio
    // n_samples is total samples (for stereo: n_samples includes both channels)
    process_audio_go(in, out, n_samples * data->channels);

    // Queue the processed buffers back
    pw_filter_queue_buffer(data->in_port->port, in_buf);
    pw_filter_queue_buffer(data->out_port->port, out_buf);
}

// Define filter events
static const struct pw_filter_events filter_events = {
    PW_VERSION_FILTER_EVENTS,
    .process = on_process,
};

// Function to create a PipeWire filter with both input and output ports
struct pw_filter_data* create_pipewire_filter(struct pw_main_loop *loop, int channels, int sample_rate) {
    if (!loop) {
        fprintf(stderr, "ERROR: PipeWire main loop is NULL\n");
        return NULL;
    }

    // Allocate structure to hold all PipeWire resources
    struct pw_filter_data *data = calloc(1, sizeof(struct pw_filter_data));
    if (!data) {
        fprintf(stderr, "ERROR: Failed to allocate filter data\n");
        return NULL;
    }

    data->loop = loop;
    data->channels = channels;

    // Initialize PipeWire
    pw_init(NULL, NULL);

    data->context = pw_context_new(pw_main_loop_get_loop(loop), NULL, 0);
    if (!data->context) {
        fprintf(stderr, "ERROR: Failed to create PipeWire context\n");
        free(data);
        return NULL;
    }

    // Connect to PipeWire core
    data->core = pw_context_connect(data->context, NULL, 0);
    if (!data->core) {
        fprintf(stderr, "ERROR: Failed to connect to PipeWire core\n");
        pw_context_destroy(data->context);
        free(data);
        return NULL;
    }

    // Create PipeWire filter properties
    struct pw_properties *props = pw_properties_new(
        PW_KEY_MEDIA_TYPE, "Audio",
        PW_KEY_MEDIA_CATEGORY, "Filter",
        PW_KEY_MEDIA_ROLE, "DSP",
        PW_KEY_NODE_NAME, "pw-comp",
        PW_KEY_NODE_DESCRIPTION, "Audio Compressor Filter",
        NULL
    );
    if (!props) {
        fprintf(stderr, "ERROR: Failed to create properties\n");
        pw_core_disconnect(data->core);
        pw_context_destroy(data->context);
        free(data);
        return NULL;
    }

    // Create PipeWire filter
    data->filter = pw_filter_new(data->core, "pw-comp-filter", props);
    if (!data->filter) {
        fprintf(stderr, "ERROR: Failed to create PipeWire filter\n");
        pw_properties_free(props);
        pw_core_disconnect(data->core);
        pw_context_destroy(data->context);
        free(data);
        return NULL;
    }

    // Register filter events
    pw_filter_add_listener(data->filter, &data->filter_listener, &filter_events, data);

    // Build port format string (mono or stereo)
    char format_str[128];
    snprintf(format_str, sizeof(format_str), "32 bit float %s audio",
             channels == 1 ? "mono" : "stereo");

    // Add input port
    data->in_port = calloc(1, sizeof(struct port_data));
    if (!data->in_port) {
        fprintf(stderr, "ERROR: Failed to allocate input port data\n");
        destroy_pipewire_filter(data);
        return NULL;
    }

    data->in_port->port = pw_filter_add_port(data->filter,
        PW_DIRECTION_INPUT,
        PW_FILTER_PORT_FLAG_MAP_BUFFERS,
        sizeof(struct port_data),
        pw_properties_new(
            PW_KEY_FORMAT_DSP, format_str,
            PW_KEY_PORT_NAME, "input",
            NULL),
        NULL, 0);

    if (!data->in_port->port) {
        fprintf(stderr, "ERROR: Failed to add input port\n");
        destroy_pipewire_filter(data);
        return NULL;
    }

    // Add output port
    data->out_port = calloc(1, sizeof(struct port_data));
    if (!data->out_port) {
        fprintf(stderr, "ERROR: Failed to allocate output port data\n");
        destroy_pipewire_filter(data);
        return NULL;
    }

    data->out_port->port = pw_filter_add_port(data->filter,
        PW_DIRECTION_OUTPUT,
        PW_FILTER_PORT_FLAG_MAP_BUFFERS,
        sizeof(struct port_data),
        pw_properties_new(
            PW_KEY_FORMAT_DSP, format_str,
            PW_KEY_PORT_NAME, "output",
            NULL),
        NULL, 0);

    if (!data->out_port->port) {
        fprintf(stderr, "ERROR: Failed to add output port\n");
        destroy_pipewire_filter(data);
        return NULL;
    }

    // Build parameters for the filter (latency info)
    uint8_t buffer[1024];
    struct spa_pod_builder b = SPA_POD_BUILDER_INIT(buffer, sizeof(buffer));
    const struct spa_pod *params[1];

    // Set a 10ms processing latency
    params[0] = spa_process_latency_build(&b,
        SPA_PARAM_ProcessLatency,
        &SPA_PROCESS_LATENCY_INFO_INIT(
            .ns = 10 * SPA_NSEC_PER_MSEC
        ));

    // Connect the filter with real-time processing
    if (pw_filter_connect(data->filter,
            PW_FILTER_FLAG_RT_PROCESS,
            params, 1) < 0) {
        fprintf(stderr, "ERROR: Failed to connect filter\n");
        destroy_pipewire_filter(data);
        return NULL;
    }

    printf("PipeWire filter created successfully\n");
    printf("  Channels: %d\n", channels);
    printf("  Sample Rate: %d Hz\n", sample_rate);
    printf("  Format: 32-bit float DSP\n");
    printf("  Input port: input\n");
    printf("  Output port: output\n");
    printf("\nUse 'pw-link' or 'qpwgraph' to connect audio:\n");
    printf("  pw-link <source>:output pw-comp:input\n");
    printf("  pw-link pw-comp:output <sink>:input\n");

    return data;
}

// Cleanup function to properly destroy all PipeWire resources
void destroy_pipewire_filter(struct pw_filter_data* data) {
    if (!data) {
        return;
    }

    if (data->filter) {
        pw_filter_destroy(data->filter);
    }

    if (data->core) {
        pw_core_disconnect(data->core);
    }

    if (data->context) {
        pw_context_destroy(data->context);
    }

    if (data->in_port) {
        free(data->in_port);
    }

    if (data->out_port) {
        free(data->out_port);
    }

    free(data);
}
