package client

// SystemProxySession represents an active system-level proxy configuration.
// Disable must restore the previous state.
type SystemProxySession interface {
	Disable() error
}
