#ifndef PW_WRAPPER_H
#define PW_WRAPPER_H

#include <pipewire/pipewire.h>
#include <spa/param/audio/format.h>
#include <spa/param/audio/raw.h>
#include <spa/param/format-utils.h>
#include <spa/param/latency-utils.h>
#include <spa/utils/type.h>
#include <spa/pod/pod.h>
#include <spa/pod/builder.h>
#include <spa/pod/parser.h>

extern void process_audio_go(float *in, float *out, int samples); // Go DSP function

// Structure to hold port-specific data
struct port_data {
    struct port *port;
};

// Structure to hold all PipeWire resources for filter lifecycle management
struct pw_filter_data {
    struct pw_main_loop *loop;
    struct pw_context *context;
    struct pw_core *core;
    struct pw_filter *filter;
    struct spa_hook filter_listener;
    struct port_data *in_port;
    struct port_data *out_port;
    int channels;
};

struct pw_filter_data* create_pipewire_filter(struct pw_main_loop *loop, int channels, int sample_rate);

void destroy_pipewire_filter(struct pw_filter_data* data);

#endif // PW_WRAPPER_H
