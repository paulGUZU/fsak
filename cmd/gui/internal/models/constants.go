package models

import "time"

// UI Constants
const (
	DefaultWindowWidth  = 400
	DefaultWindowHeight = 640
	MinWindowWidth      = 360
	MinWindowHeight     = 480

	ProfileManagerWidth  = 480
	ProfileManagerHeight = 720
)

// Connection Constants
const (
	ConnectionTimeout       = 4 * time.Second
	ConnectionRetryTimeout  = 20 * time.Second
	TunStartupTimeout       = 800 * time.Millisecond
	TunStopTimeout          = 4 * time.Second
	RunnerWatchPollInterval = 100 * time.Millisecond
)

// Buffer Constants
const (
	MaxLogBuffer        = 8192
	UploadPipelineLimit = 4
	MinUploadChunkSize  = 16 * 1024
	MaxUploadChunkSize  = 512 * 1024
)

// Mode Labels
const (
	ModeLabelProxy = "Proxy (current system)"
	ModeLabelTUN   = "TUN (system-wide)"
)

// App Info
const (
	AppID          = "com.paulguzu.fsak.client.gui"
	AppName        = "FSAK VPN Client"
	AppDisplayName = "FSAK"
	AppVersion     = "1.0.0"
)

// Storage
const (
	ProfilesFileName = "client_profiles.json"
	ConfigDirName    = "fsak"
)

// TUN Helper
const (
	TunHelperArg = "--fsak-tun-helper"
	TunDevice    = "utun233"
)
