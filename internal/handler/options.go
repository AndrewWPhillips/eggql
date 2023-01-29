package handler

// options.go handles setting of handler options

// The use of closures for options makes it simple for the caller to add any desired options, but the mechanism
// (which they need not understand) is not simple: The handler.New() function takes as its last (variadic) parameter
// a slice of closures each with the signature func(*Handler).  The option functions below (CacheOn, etc) return
// such a closure which captures any options (parameters passed to the option function) so that the handler can be
// modified when the closure is run.  So for example in this call:
//
//   handler.New([]string{schema}, nil, [3][]interface{{query}, nil, nil}, handler.CacheOn())
//
// handler.CacheOn() is called and the generated closure is returned then passed as the last parameter to
// handler.New().  The SetOptions() method is called within handler.New() which executes all the options
// closures which in the above case will call the closure returned from handler.CacheOn() which sets the
// cacheOn field of the handler to true.
//
// A pitfall is that if the same option function is used more than once then only the last use has any effect.

import (
	"time"
)

const (
	defaultInitialTimeout = 10 * time.Second // how long to wait for connection_init after the WS is opened
	defaultPingFrequency  = 20 * time.Second // how often to send a ping (ka in old protocol) message to the client
	defaultPongTimeout    = 5 * time.Second  // how long to wait for a pong after sending a ping
)

// SetOptions takes a slice of handler options (closures) and executes them
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

// FuncCache turns on caching forever for the results of function resolvers, but not data (non-func) resolver fields
// Values are cached indefinitely - but this can be set using the maxAge argument of @cacheControl directive.
// This setting is overridden if a field uses the @cacheControl directive to enable caching or "no_cache" to disable it.
func FuncCache(on bool) func(*Handler) {
	return func(h *Handler) {
		h.funcCache = on
	}
}

// NoIntrospection turns off all introspection queries
func NoIntrospection(on bool) func(*Handler) {
	return func(h *Handler) {
		h.noIntrospection = on
	}
}

// NoConcurrency turns off concurrent execution of queries
func NoConcurrency(on bool) func(*Handler) {
	return func(h *Handler) {
		h.noConcurrency = on
	}
}

// NilResolverAllowed allows func resolvers to be nil, whence they return a null value (rather than return an error)
func NilResolverAllowed(on bool) func(*Handler) {
	return func(h *Handler) {
		h.nilResolver = on
	}
}

// InitialTimeout sets the length time to wait from when the websocket is opened until the
// "connection_init" message is received. If the message is not received from the client
// within the time limit then an error message is returned to the client and the WS is closed.
func InitialTimeout(timeout time.Duration) func(*Handler) {
	return func(h *Handler) {
		h.initialTimeout = timeout // timeout value is "captured" and returned as part of the func
	}
}

// PingFrequency says how often to send a "ping" message (if the client connects with new
// protocol) or a "ka" (keep alive) message (old protocol)
func PingFrequency(freq time.Duration) func(*Handler) {
	return func(h *Handler) {
		h.pingFrequency = freq
	}
}

// PongTimeout set the length time to wait for a "pong" message from the client after
// a "ping" message is sent. If the message is not received from the client
// within the time limit then an error message is returned to the client and the WS is closed.
func PongTimeout(timeout time.Duration) func(*Handler) {
	return func(h *Handler) {
		h.pongTimeout = timeout
	}
}
