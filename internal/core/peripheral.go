package core

import "context"

// PeripheralCapability identifies a sensing/actuation capability a peripheral
// offers. Peripherals are the hardware seam for edge and wearable deployments:
// microphones, cameras, GPIO lines, and later custom wearable sensors all sit
// behind this one contract so the agent core never links a specific driver.
type PeripheralCapability string

const (
	PeripheralAudioIn  PeripheralCapability = "audio_in"  // microphone / wake-word source
	PeripheralAudioOut PeripheralCapability = "audio_out" // speaker / TTS sink
	PeripheralCamera   PeripheralCapability = "camera"    // still or video capture
	PeripheralGPIO     PeripheralCapability = "gpio"      // digital IO lines
	PeripheralSensor   PeripheralCapability = "sensor"    // generic analog/serial sensor
)

// PeripheralEvent is a single reading or event emitted by a peripheral (an audio
// frame, a captured image reference, a GPIO edge, a sensor sample).
type PeripheralEvent struct {
	Peripheral string               // emitting peripheral name
	Capability PeripheralCapability // which capability produced it
	Payload    map[string]any       // structured, capability-specific data
}

// PeripheralEventFunc receives events a peripheral emits while running.
type PeripheralEventFunc func(PeripheralEvent)

// Peripheral is a hardware (or virtual) device the agent can sense from or act
// on. Implementations own their own goroutine/lifecycle between Start and Stop.
// The C++ sensing binaries (camera/audio) and future GPIO/wearable bridges
// implement this contract.
type Peripheral interface {
	// Name returns a stable identifier for this peripheral instance.
	Name() string
	// Capabilities lists what this peripheral can do.
	Capabilities() []PeripheralCapability
	// Start begins operation, emitting events via emit until ctx is cancelled
	// or Stop is called. It must not block.
	Start(ctx context.Context, emit PeripheralEventFunc) error
	// Stop halts operation and releases resources.
	Stop() error
}
