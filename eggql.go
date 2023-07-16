package eggql

// eggql.go provides the gql type for generating a GraphQL HTTP handler (and/or schema)

// Using eggql.New() and calling the methods of the returned type is more flexible
// but more involved than simply calling eggql.MustRun (see run.go).

// For example, you can obtain the generated schema as a string using the GetSchema method.

// Call eggql.New() to obtain an instance of the type, then call its methods
// (SetEnums, Add, etc), if needed, then call the GetHandler() method to generate
// the HTTP handler or call GetSchema() to obtain the schema as a string.

// You can also set options such as websocket timeouts and ping frequency for subscriptions.

import (
	"net/http"
	"time"

	"github.com/andrewwphillips/eggql/internal/handler"
	"github.com/andrewwphillips/eggql/internal/schema"
)

type (
	// gql is an internal type, so it is not possible to modify the struct fields
	// outside the eggql package, but you can obtain one by calling eggql.New()
	// then call its public methods.
	gql struct {
		enums   map[string][]string
		qms     [][3]interface{} // each slice element represents a schema (with a root query, mutation and subscription)
		options []func(*handler.Handler)
	}
)

// New creates a new instance with from zero to 3 parameters representing the
// query, mutation, and subscription types of a schema.  Further schemas can
// be added (to enable schema stitching) by using the Add method.
// If no parameters are provided then the returned instance will be empty and
// will be of no use unless you subsequently call Add() at least once.
func New(q ...interface{}) gql {
	g := gql{}
	if len(q) > 0 { // leave g.qms slice empty if no params were supplied
		g.Add(q...)
	}
	return g
}

// Add allows adding of another query, mutation and/or subscription.
// Up to 3 parameters can be given, but they must be structs and passed in that
// order (query, mutation, subscription) but any may be nil (eg use nil for the
// query if you only want to add a mutation).
func (g *gql) Add(q ...interface{}) {
	var schemaQMS [3]interface{}
	for i := 0; i < 3; i++ {
		if len(q) > i {
			schemaQMS[i] = q[i]
		}
	}
	g.qms = append(g.qms, schemaQMS)
}

// Len returns the number of query/mutation/subscriptions sets that have been added.
func (g *gql) Len() int {
	return len(g.qms)
}

// SetEnums adds or replaces all enums used in generating the schema
func (g *gql) SetEnums(enums map[string][]string) {
	g.enums = enums
}

// AddEnum adds one enum to the map of enums used in generating the schema.
// You can call AddEnum repeatedly to add multiple enums, instead of using SetEnums.
func (g *gql) AddEnum(name string, values []string) {
	if g.enums == nil {
		g.enums = make(map[string][]string)
	}
	g.enums[name] = values
}

// GetSchema builds and returns the GraphQL schema
func (g *gql) GetSchema() (string, error) {
	var schemaString string
	for _, schemaQMS := range g.qms {
		s, err := schema.Build(g.enums, schemaQMS[:]...)
		if err != nil {
			return "", err
		}
		schemaString += s // should we do more than concatenate the schemas?
	}
	return schemaString, nil
}

// GetHandler uses the previously added Query, Enums, options, etc to build the
// schema and return the HTTP handler
func (g *gql) GetHandler() (http.Handler, error) {
	var schemaStrings []string
	var schemaQMS [3][]interface{}
	for _, qms := range g.qms {
		s, err := schema.Build(g.enums, qms[:]...)
		if err != nil {
			return nil, err
		}
		if len(schemaStrings) == 0 {
			schemaStrings = append(schemaStrings, s)
		} else {
			schemaStrings = append(schemaStrings, "extend "+s)
		}
		if qms[0] != nil {
			schemaQMS[0] = append(schemaQMS[0], qms[0])
		}
		if qms[1] != nil {
			schemaQMS[1] = append(schemaQMS[1], qms[1])
		}
		if qms[2] != nil {
			schemaQMS[2] = append(schemaQMS[2], qms[2])
		}
	}
	return handler.New(schemaStrings, g.enums, schemaQMS, g.options...), nil
}

func (g *gql) SetInitialTimeout(timeout time.Duration) {
	g.options = append(g.options, handler.InitialTimeout(timeout))
}

func (g *gql) SetPingFrequency(freq time.Duration) {
	g.options = append(g.options, handler.PingFrequency(freq))
}

func (g *gql) SetPongTimeout(timeout time.Duration) {
	g.options = append(g.options, handler.PongTimeout(timeout))
}
