package eggql

// options.go handles options that can be used to control the GraphQL server.
// These options are just passed on to the handler. (See internal/handler/options.go
// for details on how closures are used to handle options.)

import (
	"time"
)

type options struct {
	// handler options
	funcCache, noIntrospection, noConcurrency, nilResolver bool
	initialTimeout, pingFrequency, pongTimeout             time.Duration
}

// FuncCache setting the parameter to true means all *function* resolver results are cached, whereas false
// means no resolvers are cached (in the absence of any cache directives or caching options).
// Non-func resolvers are *not* cached even with this setting turned (since they are in memory anyway)
// but you can still cache them with the @cacheControl directive.  Note that even when this option is on
// you can still disable caching using the "no_cache" option in the field's egg: tag string.
// To limit the length of time a value is cached use the maxAge argument of the @cacheControl directive.
func FuncCache(on bool) func(*options) {
	return func(opt *options) {
		opt.funcCache = on
	}
}

// NoIntrospection controls whether introspection queries are permitted
func NoIntrospection(on bool) func(*options) {
	return func(opt *options) {
		opt.noIntrospection = on
	}
}

// NoConcurrency controls whether concurrent excution of queries (but not mutations) is permitted
func NoConcurrency(on bool) func(*options) {
	return func(opt *options) {
		opt.noConcurrency = on
	}
}

// NilResolver controls whether nil resolvers are allowed - if not a nil resolver function results in an error
func NilResolver(on bool) func(*options) {
	return func(opt *options) {
		opt.nilResolver = on
	}
}

// InitialTimeout sets the length time to wait from when the websocket is opened until the
// "connection_init" message is received. If the message is not received from the client
// within the time limit then an error message is returned to the client and the WS is closed.
func InitialTimeout(timeout time.Duration) func(*options) {
	return func(opt *options) {
		opt.initialTimeout = timeout
	}
}

// PingFrequency says how often to send a "ping" message (if the client connects with new
// GraphQL websocket protocol) or a "ka" (keep alive) message (old protocol)
func PingFrequency(freq time.Duration) func(*options) {
	return func(opt *options) {
		opt.pingFrequency = freq
	}
}

// PongTimeout set the length time to wait for a "pong" message from the client after
// a "ping" message is sent. If the message is not received from the client
// within the time limit then an error message is returned to the client and the WS is closed.
func PongTimeout(timeout time.Duration) func(*options) {
	return func(opt *options) {
		opt.pongTimeout = timeout
	}
}
