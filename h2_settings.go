package fns

import (
	"encoding/binary"
	"errors"
)

// Identifiers for HTTP/2 serverSettings
const (
	SettingHeaderTableSize      uint16 = 0x1
	SettingEnablePush           uint16 = 0x2
	SettingMaxConcurrentStreams uint16 = 0x3
	SettingInitialWindowSize    uint16 = 0x4
	SettingMaxFrameSize         uint16 = 0x5
	SettingMaxHeaderListSize    uint16 = 0x6
)

// Default HTTP/2 serverSettings values as per RFC 9113
const (
	DefaultHeaderTableSize      = 4096
	DefaultEnablePush           = 1
	DefaultMaxConcurrentStreams = 0
	DefaultInitialWindowSize    = 65535
	DefaultMaxFrameSize         = 16384
	DefaultMaxHeaderListSize    = 0
)

// ProtocolDefaultSettings defines the default values for HTTP/2 serverSettings
var ProtocolDefaultSettings = [...]uint32{
	0, // Placeholder for index 0
	DefaultHeaderTableSize,
	DefaultEnablePush,
	DefaultMaxConcurrentStreams,
	DefaultInitialWindowSize,
	DefaultMaxFrameSize,
	DefaultMaxHeaderListSize,
}

// Settings defines the structure for HTTP/2 serverSettings
type Settings struct {
	headerTableSize      uint32
	enablePush           uint32
	maxConcurrentStreams uint32
	initialWindowSize    uint32
	maxFrameSize         uint32
	maxHeaderListSize    uint32
}

var ErrShortBuffer = errors.New("short buffer")

// NewSettings creates a Settings object with protocol default values
func NewSettings() Settings {
	return Settings{
		headerTableSize:      ProtocolDefaultSettings[SettingHeaderTableSize],
		enablePush:           ProtocolDefaultSettings[SettingEnablePush],
		maxConcurrentStreams: ProtocolDefaultSettings[SettingMaxConcurrentStreams],
		initialWindowSize:    ProtocolDefaultSettings[SettingInitialWindowSize],
		maxFrameSize:         ProtocolDefaultSettings[SettingMaxFrameSize],
		maxHeaderListSize:    ProtocolDefaultSettings[SettingMaxHeaderListSize],
	}
}

// Set sets a specific setting by its identifier
func (s *Settings) Set(id uint16, value uint32) {
	switch id {
	case SettingHeaderTableSize:
		s.headerTableSize = value
	case SettingEnablePush:
		s.enablePush = value
	case SettingMaxConcurrentStreams:
		s.maxConcurrentStreams = value
	case SettingInitialWindowSize:
		s.initialWindowSize = value
	case SettingMaxFrameSize:
		s.maxFrameSize = value
	case SettingMaxHeaderListSize:
		s.maxHeaderListSize = value
	}
}

// Count returns the number of serverSettings
func (s *Settings) Count() int {
	return 6
}

// Get returns the value of a specific setting by its identifier
func (s *Settings) Get(id uint16) uint32 {
	switch id {
	case SettingHeaderTableSize:
		return s.headerTableSize
	case SettingEnablePush:
		return s.enablePush
	case SettingMaxConcurrentStreams:
		return s.maxConcurrentStreams
	case SettingInitialWindowSize:
		return s.initialWindowSize
	case SettingMaxFrameSize:
		return s.maxFrameSize
	case SettingMaxHeaderListSize:
		return s.maxHeaderListSize
	default:
		return 0
	}
}

// PutParams puts the non-defaul serverSettings into the body of a frame
func (s *Settings) PutParams(body *[]byte) error {
	if cap(*body) < 36 {
		return ErrShortBuffer
	}

	*body = (*body)[:36] // Adjust slice to required length

	// Define serverSettings parameters
	settingsParams := []struct {
		id    uint16
		value uint32
	}{
		{SettingHeaderTableSize, s.headerTableSize},
		{SettingEnablePush, s.enablePush},
		{SettingMaxConcurrentStreams, s.maxConcurrentStreams},
		{SettingInitialWindowSize, s.initialWindowSize},
		{SettingMaxFrameSize, s.maxFrameSize},
		{SettingMaxHeaderListSize, s.maxHeaderListSize},
	}

	// Initialize an index
	index := 0

	// Put each setting into frame.Body one-by-one
	for _, param := range settingsParams {
		if int(param.id) <= len(ProtocolDefaultSettings) && ProtocolDefaultSettings[param.id] != param.value {
			binary.BigEndian.PutUint16((*body)[index:], param.id)
			index += 2
			binary.BigEndian.PutUint32((*body)[index:], param.value)
			index += 4
		}
	}

	// Adjust the length of the body slice
	*body = (*body)[:index]

	return nil
}
