package eggql

// eggql.go provides the gql type for generating a GraphQL HTTP handler or schema

import (
	"net/http"
	"time"

	"github.com/andrewwphillips/eggql/internal/handler"
	"github.com/andrewwphillips/eggql/internal/schema"
)

type (
	gql struct {
		enums   map[string][]string
		qms     [][3]interface{} // each slice element represents a schema (with a root query, mutation and subscription)
		options []func(*handler.Handler)
	}
)

// New creates a new instance with from zero to 3 parameters representing the
// query, mutation, and subscription types of a schema.  Further schemas can
// be added (to enable schema stitching) by calling using the Add method.
func New(q ...interface{}) gql {
	g := gql{}
	if len(q) > 0 {
		g.Add(q...)
	}
	return g
}

func (g *gql) Add(q ...interface{}) {
	var schemaQMS [3]interface{}
	for i := 0; i < 3; i++ {
		if len(q) > i {
			schemaQMS[i] = q[i]
		}
	}
	g.qms = append(g.qms, schemaQMS)
}

// SetEnums adds or replaces enums used in generating the schema
func (g *gql) SetEnums(enums map[string][]string) {
	g.enums = enums
}

// AddEnum adds one enum to the map of enums used in generating the schema.
// You can call AddEnum repeatedly instead of using SetEnums.
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
