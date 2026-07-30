// AssistClaw camera capture — streams JPEG frames as newline-delimited JSON to stdout.
// The Go layer reads this stream via stdin/stdout IPC.
//
// Protocol:
//   {"type":"frame","timestamp":1708123456789,"width":1280,"height":720,"data":"<base64 JPEG>"}
//   {"type":"error","message":"..."}
//   {"type":"ready"}

#include <opencv2/opencv.hpp>
#include <base64/base64.h>
#include <chrono>
#include <csignal>
#include <cstdlib>
#include <iostream>
#include <nlohmann/json.hpp>
#include <thread>

using json = nlohmann::json;

static volatile sig_atomic_t g_running = 1;

void handleSignal(int) { g_running = 0; }

int64_t nowMs() {
    return std::chrono::duration_cast<std::chrono::milliseconds>(
        std::chrono::system_clock::now().time_since_epoch()).count();
}

// Base64 encode (minimal implementation — replace with a real library in prod)
static const std::string b64chars = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/";

std::string base64Encode(const std::vector<uchar>& bytes) {
    std::string ret;
    int i = 0;
    unsigned char char3[3], char4[4];
    for (auto b : bytes) {
        char3[i++] = b;
        if (i == 3) {
            char4[0] = (char3[0] & 0xfc) >> 2;
            char4[1] = ((char3[0] & 0x03) << 4) + ((char3[1] & 0xf0) >> 4);
            char4[2] = ((char3[1] & 0x0f) << 2) + ((char3[2] & 0xc0) >> 6);
            char4[3] = char3[2] & 0x3f;
            for (int k = 0; k < 4; k++) ret += b64chars[char4[k]];
            i = 0;
        }
    }
    if (i) {
        for (int j = i; j < 3; j++) char3[j] = '\0';
        char4[0] = (char3[0] & 0xfc) >> 2;
        char4[1] = ((char3[0] & 0x03) << 4) + ((char3[1] & 0xf0) >> 4);
        char4[2] = ((char3[1] & 0x0f) << 2) + ((char3[2] & 0xc0) >> 6);
        for (int k = 0; k < i + 1; k++) ret += b64chars[char4[k]];
        while (i++ < 3) ret += '=';
    }
    return ret;
}

int main(int argc, char* argv[]) {
    // Parse args: --device 0 --width 1280 --height 720 --fps 15 --quality 80
    int deviceIndex = 0;
    int width = 1280, height = 720, fps = 15, quality = 80;

    for (int i = 1; i < argc - 1; i++) {
        std::string arg(argv[i]);
        if (arg == "--device")  deviceIndex = std::atoi(argv[++i]);
        else if (arg == "--width")   width   = std::atoi(argv[++i]);
        else if (arg == "--height")  height  = std::atoi(argv[++i]);
        else if (arg == "--fps")     fps     = std::atoi(argv[++i]);
        else if (arg == "--quality") quality = std::atoi(argv[++i]);
    }

    std::signal(SIGINT,  handleSignal);
    std::signal(SIGTERM, handleSignal);

    cv::VideoCapture cap(deviceIndex, cv::CAP_ANY);
    if (!cap.isOpened()) {
        json err = {{"type", "error"}, {"message", "failed to open camera device " + std::to_string(deviceIndex)}};
        std::cout << err.dump() << "\n" << std::flush;
        return 1;
    }

    cap.set(cv::CAP_PROP_FRAME_WIDTH,  width);
    cap.set(cv::CAP_PROP_FRAME_HEIGHT, height);
    cap.set(cv::CAP_PROP_FPS,          fps);

    // Report actual camera settings
    json ready = {
        {"type",   "ready"},
        {"width",  (int)cap.get(cv::CAP_PROP_FRAME_WIDTH)},
        {"height", (int)cap.get(cv::CAP_PROP_FRAME_HEIGHT)},
        {"fps",    (int)cap.get(cv::CAP_PROP_FPS)},
    };
    std::cout << ready.dump() << "\n" << std::flush;

    const auto interval = std::chrono::milliseconds(1000 / std::max(fps, 1));
    cv::Mat frame;
    std::vector<uchar> buf;
    std::vector<int> encParams = {cv::IMWRITE_JPEG_QUALITY, quality};

    int flushCounter = 0;
    while (g_running) {
        auto t0 = std::chrono::steady_clock::now();
        if (!cap.read(frame) || frame.empty()) {
            json err = {{"type", "error"}, {"message", "failed to read frame"}};
            std::cout << err.dump() << "\n";
            std::this_thread::sleep_for(std::chrono::milliseconds(100));
            continue;
        }

        buf.clear();
        cv::imencode(".jpg", frame, buf, encParams);

        json f = {
            {"type", "frame"},
            {"timestamp", nowMs()},
            {"width",  frame.cols},
            {"height", frame.rows},
            {"data",   base64Encode(buf)},
        };
        std::cout << f.dump() << "\n";
        if (++flushCounter % 5 == 0) {
            std::cout << std::flush;
        }

        // Rate-limit to target FPS
        auto elapsed = std::chrono::steady_clock::now() - t0;
        if (elapsed < interval) {
            std::this_thread::sleep_for(interval - elapsed);
        }
    }

    cap.release();
    return 0;
}
