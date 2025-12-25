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

extern void process_channel_go(float *in, float *out, int samples, int sample_rate, int channel_index);
extern void log_from_c(char *msg);

// Structure to hold port-specific data
struct port_data {
    enum pw_direction direction;
    int channel;
};

// Structure to hold all PipeWire resources for filter lifecycle management
struct pw_filter_data {
    struct pw_main_loop *loop;
    struct pw_context *context;
    struct pw_core *core;
    struct pw_filter *filter;
    struct spa_hook filter_listener;
    struct port_data **in_ports;  // Array of pointers to port_data
    struct port_data **out_ports; // Array of pointers to port_data
    int channels;
};

struct pw_filter_data* create_pipewire_filter(struct pw_main_loop *loop, int channels);

void destroy_pipewire_filter(struct pw_filter_data* data);

#endif // PW_WRAPPER_H
