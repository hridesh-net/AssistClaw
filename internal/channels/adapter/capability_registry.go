package adapter

import (
	"strings"
	"sync"
)

var capabilityRegistry = map[string]ChannelCapabilities{
	"telegram": {
		Threading:          true,
		Attachments:        true,
		DirectMessages:     true,
		GroupMessages:      true,
		Mentions:           true,
		Voice:              false,
		Reactions:          true,
		Edits:              true,
		MaxMessageLength:   4096,
		InteractiveButtons: true,
	},
	"discord": {
		Threading:          true,
		Attachments:        true,
		DirectMessages:     true,
		GroupMessages:      true,
		Mentions:           true,
		Voice:              true,
		Reactions:          true,
		Edits:              true,
		MaxMessageLength:   2000,
		InteractiveButtons: true,
	},
	"slack": {
		Threading:          true,
		Attachments:        true,
		DirectMessages:     true,
		GroupMessages:      true,
		Mentions:           true,
		Voice:              false,
		Reactions:          true,
		Edits:              true,
		MaxMessageLength:   40000,
		InteractiveButtons: true,
	},
	"whatsapp": {
		Threading:        false,
		Attachments:      true,
		DirectMessages:   true,
		GroupMessages:    true,
		Mentions:         true,
		Voice:            true,
		Reactions:        true,
		Edits:            false,
		MaxMessageLength: 4000,
	},
}

var capabilityRegistryMu sync.RWMutex

// CapabilitiesFor returns known capabilities for a built-in channel type.
func CapabilitiesFor(channelType string) (ChannelCapabilities, bool) {
	key := strings.ToLower(strings.TrimSpace(channelType))
	capabilityRegistryMu.RLock()
	c, ok := capabilityRegistry[key]
	capabilityRegistryMu.RUnlock()
	return c, ok
}

// RegisterCapabilities allows extensions/new channels to publish their capabilities.
func RegisterCapabilities(channelType string, caps ChannelCapabilities) {
	channelType = strings.ToLower(strings.TrimSpace(channelType))
	if channelType == "" {
		return
	}
	capabilityRegistryMu.Lock()
	capabilityRegistry[channelType] = caps
	capabilityRegistryMu.Unlock()
}

// BuiltinCaps returns capabilities for channelType from the global registry.
// Built-in [Identity] implementations should delegate [Identity.Capabilities] here
// so the matrix is defined only in capability_registry.go (DRY).
func BuiltinCaps(channelType string) (ChannelCapabilities, bool) {
	return CapabilitiesFor(channelType)
}
