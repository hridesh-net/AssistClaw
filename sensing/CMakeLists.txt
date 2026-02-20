cmake_minimum_required(VERSION 3.20)
project(assistclaw-sensing VERSION 1.0.0 LANGUAGES CXX)

set(CMAKE_CXX_STANDARD 20)
set(CMAKE_CXX_STANDARD_REQUIRED ON)
set(CMAKE_CXX_EXTENSIONS OFF)

# ─────────────────────────────────────────────
# Options
# ─────────────────────────────────────────────
option(SENSING_CAMERA "Build camera capture binary" ON)
option(SENSING_AUDIO  "Build audio stream binary"   ON)
option(SENSING_GPIO   "Build GPIO control binary"   OFF)

# ─────────────────────────────────────────────
# Find optional dependencies
# ─────────────────────────────────────────────
if(SENSING_CAMERA)
  find_package(OpenCV QUIET COMPONENTS core highgui imgproc videoio)
  if(NOT OpenCV_FOUND)
    message(WARNING "OpenCV not found — camera capture will not be built")
    set(SENSING_CAMERA OFF)
  endif()
endif()

if(SENSING_AUDIO)
  find_package(PkgConfig QUIET)
  if(PkgConfig_FOUND)
    pkg_check_modules(PORTAUDIO portaudio-2.0)
  endif()
  if(NOT PORTAUDIO_FOUND)
    message(WARNING "PortAudio not found — audio stream will not be built")
    set(SENSING_AUDIO OFF)
  endif()
endif()

# ─────────────────────────────────────────────
# Compiler flags
# ─────────────────────────────────────────────
add_compile_options(-Wall -Wextra -Wpedantic -O2)
if(CMAKE_CXX_COMPILER_ID MATCHES "GNU|Clang")
  add_compile_options(-fstack-protector-strong)
endif()

# ─────────────────────────────────────────────
# Camera capture binary
# ─────────────────────────────────────────────
if(SENSING_CAMERA)
  add_executable(assistclaw-camera camera/camera_capture.cpp)
  target_link_libraries(assistclaw-camera PRIVATE ${OpenCV_LIBS})
  target_include_directories(assistclaw-camera PRIVATE ${OpenCV_INCLUDE_DIRS})
  install(TARGETS assistclaw-camera DESTINATION bin)
  message(STATUS "Camera capture: enabled (OpenCV ${OpenCV_VERSION})")
endif()

# ─────────────────────────────────────────────
# Audio stream binary
# ─────────────────────────────────────────────
if(SENSING_AUDIO)
  add_executable(assistclaw-audio audio/audio_stream.cpp)
  target_link_libraries(assistclaw-audio PRIVATE ${PORTAUDIO_LIBRARIES})
  target_compile_options(assistclaw-audio PRIVATE ${PORTAUDIO_CFLAGS_OTHER})
  target_include_directories(assistclaw-audio PRIVATE ${PORTAUDIO_INCLUDE_DIRS})
  install(TARGETS assistclaw-audio DESTINATION bin)
  message(STATUS "Audio stream: enabled (PortAudio)")
endif()

# ─────────────────────────────────────────────
# GPIO binary (Raspberry Pi / LuckFox)
# ─────────────────────────────────────────────
if(SENSING_GPIO)
  find_library(PIGPIO_LIB pigpio)
  if(PIGPIO_LIB)
    add_executable(assistclaw-gpio gpio/gpio_ctrl.cpp)
    target_link_libraries(assistclaw-gpio PRIVATE ${PIGPIO_LIB} pthread)
    install(TARGETS assistclaw-gpio DESTINATION bin)
    message(STATUS "GPIO control: enabled (pigpio)")
  else()
    message(WARNING "pigpio not found — GPIO binary will not be built")
  endif()
endif()
