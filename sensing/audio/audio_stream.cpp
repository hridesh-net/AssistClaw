// AssistClaw audio stream — pipes PCM audio frames as newline-delimited JSON to stdout.
// The Go layer reads this stream via stdin/stdout IPC.
//
// Protocol:
//   {"type":"ready","sample_rate":16000,"channels":1,"format":"pcm_s16le"}
//   {"type":"audio","timestamp":1708123456789,"samples":512,"data":"<base64 PCM>"}
//   {"type":"error","message":"..."}

#include <portaudio.h>
#include <csignal>
#include <cstdlib>
#include <cstring>
#include <iostream>
#include <sstream>
#include <string>
#include <vector>

static volatile sig_atomic_t g_running = 1;
void handleSignal(int) { g_running = 0; }

// Minimal base64 encode (same as camera version — consolidate into shared lib in prod)
static const std::string B64 = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/";
std::string b64Encode(const void* data, size_t len) {
    const auto* in = reinterpret_cast<const unsigned char*>(data);
    std::string out;
    out.reserve((len + 2) / 3 * 4);
    for (size_t i = 0; i < len; i += 3) {
        unsigned int val = in[i] << 16;
        if (i + 1 < len) val |= in[i+1] << 8;
        if (i + 2 < len) val |= in[i+2];
        out += B64[(val >> 18) & 0x3f];
        out += B64[(val >> 12) & 0x3f];
        out += (i + 1 < len) ? B64[(val >> 6) & 0x3f] : '=';
        out += (i + 2 < len) ? B64[val & 0x3f] : '=';
    }
    return out;
}

int64_t nowMs() {
    struct timespec ts;
    clock_gettime(CLOCK_REALTIME, &ts);
    return (int64_t)ts.tv_sec * 1000 + ts.tv_nsec / 1000000;
}

// Callback buffer
struct StreamState {
    std::vector<int16_t> buffer;
    bool ready = false;
};

static int audioCallback(
    const void* inputBuffer,
    void* /*outputBuffer*/,
    unsigned long framesPerBuffer,
    const PaStreamCallbackTimeInfo* /*timeInfo*/,
    PaStreamCallbackFlags /*statusFlags*/,
    void* userData)
{
    auto* state = reinterpret_cast<StreamState*>(userData);
    if (!inputBuffer || !g_running) return paComplete;

    const int16_t* in = reinterpret_cast<const int16_t*>(inputBuffer);
    state->buffer.assign(in, in + framesPerBuffer);
    state->ready = true;
    return paContinue;
}

int main(int argc, char* argv[]) {
    int sampleRate = 16000;
    int channels   = 1;
    int framesPerBuf = 512;

    for (int i = 1; i < argc - 1; i++) {
        std::string arg(argv[i]);
        if (arg == "--sample-rate")  sampleRate   = std::atoi(argv[++i]);
        else if (arg == "--channels") channels    = std::atoi(argv[++i]);
        else if (arg == "--frames")  framesPerBuf = std::atoi(argv[++i]);
    }

    std::signal(SIGINT,  handleSignal);
    std::signal(SIGTERM, handleSignal);

    PaError err = Pa_Initialize();
    if (err != paNoError) {
        std::cout << "{\"type\":\"error\",\"message\":\"Pa_Initialize: " << Pa_GetErrorText(err) << "\"}\n" << std::flush;
        return 1;
    }

    StreamState state;
    PaStream* stream = nullptr;
    err = Pa_OpenDefaultStream(
        &stream,
        channels, 0,          // input channels, no output
        paInt16,
        sampleRate,
        framesPerBuf,
        audioCallback,
        &state);

    if (err != paNoError) {
        std::cout << "{\"type\":\"error\",\"message\":\"" << Pa_GetErrorText(err) << "\"}\n" << std::flush;
        Pa_Terminate();
        return 1;
    }

    Pa_StartStream(stream);

    // Ready event
    std::cout << "{\"type\":\"ready\""
              << ",\"sample_rate\":" << sampleRate
              << ",\"channels\":"    << channels
              << ",\"format\":\"pcm_s16le\"}\n" << std::flush;

    int flushCounter = 0;
    while (g_running) {
        Pa_Sleep(10); // 10ms poll
        if (!state.ready) continue;
        state.ready = false;

        std::string encoded = b64Encode(state.buffer.data(), state.buffer.size() * sizeof(int16_t));
        std::cout << "{\"type\":\"audio\""
                  << ",\"timestamp\":" << nowMs()
                  << ",\"samples\":"   << state.buffer.size()
                  << ",\"data\":\""    << encoded << "\"}\n";
        if (++flushCounter % 10 == 0) {
            std::cout << std::flush;
        }
    }

    Pa_StopStream(stream);
    Pa_CloseStream(stream);
    Pa_Terminate();
    return 0;
}
