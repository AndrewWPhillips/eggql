package handler

import (
	"time"
)

// options.go handles setting of handler options

const (
	defaultInitialTimeout = 10 * time.Second // how long to wait for connection_init after the WS is opened
	defaultPingFrequency  = 20 * time.Second // how often to send a ping (ka in old protocol) message to the client
	defaultPongTimeout    = 5 * time.Second  // how long to wait for a pong after sending a ping
)

func (h *Handler) SetOptions(options ...func(*Handler)) {
	for _, option := range options {
		option(h)
	}

	// Set any options that still have their unset (zero) value
	if h.initialTimeout == 0 {
		h.initialTimeout = defaultInitialTimeout
	}
	if h.pingFrequency == 0 {
		h.pingFrequency = defaultPingFrequency
	}
	if h.pongTimeout == 0 {
		h.pongTimeout = defaultPongTimeout
	}
}

// InitialTimeout set the length time to wait from when the websocket is opened until the
// "connection_init" message is received. If the message is not received from the client
// within the time limit then an error message is returned to the client and the WS is closed.
func InitialTimeout(timeout time.Duration) func(h *Handler) {
	return func(h *Handler) {
		h.initialTimeout = timeout // timeout value is "captured" and returned as part of the func
	}
}

// PingFrequency says how often to send a "ping" message (if the client connects with new
// protocol) or a "ka" (keep alive) message (old protocol)
func PingFrequency(freq time.Duration) func(h *Handler) {
	return func(h *Handler) {
		h.pingFrequency = freq
	}
}

// PongTimeout set the length time to wait for a "pong" message from the client after
// a "ping" message is sent. If the message is not received from the client
// within the time limit then an error message is returned to the client and the WS is closed.
func PongTimeout(timeout time.Duration) func(h *Handler) {
	return func(h *Handler) {
		h.pongTimeout = timeout
	}
}
