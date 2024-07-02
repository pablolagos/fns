package fns

// Identifiers for HTTP/2 settings
const (
	SettingHeaderTableSize      uint16 = 0x1
	SettingEnablePush           uint16 = 0x2
	SettingMaxConcurrentStreams uint16 = 0x3
	SettingInitialWindowSize    uint16 = 0x4
	SettingMaxFrameSize         uint16 = 0x5
	SettingMaxHeaderListSize    uint16 = 0x6
)

// Default HTTP/2 settings values
const (
	DefaultHeaderTableSize      = 4096
	DefaultEnablePush           = 1
	DefaultMaxConcurrentStreams = 100
	DefaultInitialWindowSize    = 65535
	DefaultMaxFrameSize         = 16384
	DefaultMaxHeaderListSize    = 100
)

// Settings defines the structure for HTTP/2 settings
type Settings struct {
	headerTableSize      uint32
	enablePush           uint32
	maxConcurrentStreams uint32
	initialWindowSize    uint32
	maxFrameSize         uint32
	maxHeaderListSize    uint32
}

// NewSettings creates a Settings object with default values
func NewSettings() *Settings {
	return &Settings{
		headerTableSize:      DefaultHeaderTableSize,
		enablePush:           DefaultEnablePush,
		maxConcurrentStreams: DefaultMaxConcurrentStreams,
		initialWindowSize:    DefaultInitialWindowSize,
		maxFrameSize:         DefaultMaxFrameSize,
		maxHeaderListSize:    DefaultMaxHeaderListSize,
	}
}

// Values returns the settings as a map
func (s *Settings) Values() map[uint16]uint32 {
	return map[uint16]uint32{
		SettingHeaderTableSize:      s.headerTableSize,
		SettingEnablePush:           s.enablePush,
		SettingMaxConcurrentStreams: s.maxConcurrentStreams,
		SettingInitialWindowSize:    s.initialWindowSize,
		SettingMaxFrameSize:         s.maxFrameSize,
		SettingMaxHeaderListSize:    s.maxHeaderListSize,
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

// Count returns the number of settings
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
