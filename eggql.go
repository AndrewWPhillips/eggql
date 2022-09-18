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
		qms     [3]interface{}
		options []func(*handler.Handler)
	}
)

// New creates a new instance with from zero to 3 parameters representing the
// query, mutation, and subscription types (though these may also be added or replaced
// later using the SetQuery, SetMutation, and SetSubscription methods).
func New(q ...interface{}) gql {
	g := gql{}
	for i := 0; i < 3; i++ {
		if len(q) > i {
			g.qms[i] = q[i]
		}
	}
	return g
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

// SetQuery adds or replaces the struct representing the root query type
func (g *gql) SetQuery(query interface{}) {
	g.qms[0] = query
}

// SetMutation adds or replaces the struct representing the root mutation type
func (g *gql) SetMutation(mutation interface{}) {
	g.qms[1] = mutation
}

// SetSubscription adds or replaces the struct representing the subscription type
func (g *gql) SetSubscription(subscription interface{}) {
	g.qms[2] = subscription
}

// GetSchema builds and returns the GraphQL schema
func (g *gql) GetSchema() (string, error) {
	s, err := schema.Build(g.enums, g.qms[:]...)
	if err != nil {
		return "", err
	}
	return s, nil
}

// GetHandler builds the schema and returns the HTTP handler that handles GraphQL queries
func (g *gql) GetHandler() (http.Handler, error) {
	s, err := schema.Build(g.enums, g.qms[:]...)
	if err != nil {
		return nil, err
	}
	return handler.New([]string{s}, g.enums, [3][]interface{}{{g.qms[0]}, {g.qms[1]}, {g.qms[2]}}, g.options...), nil
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
