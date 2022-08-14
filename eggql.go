package eggql

// eggql.go provides the gql type for generating a GraphQL HTTP handler or schema

import (
	"net/http"

	"github.com/andrewwphillips/eggql/internal/handler"
	"github.com/andrewwphillips/eggql/internal/schema"
)

type (
	gql struct {
		enums map[string][]string
		qms   [3]interface{}
	}
)

// New creates a new instance with from zero to 3 parameters representing the
// query, mutation, and subscription types (though these may also be added or replaced
// later using the SetQuery, SetMutation, and SetSubscription methods).
func New(q ...interface{}) gql {
	r := gql{}
	for i := 0; i < 3; i++ {
		if len(q) > i {
			r.qms[i] = q[i]
		}
	}
	return r
}

// SetEnums adds or replaces enums used in generating the schema
func (h *gql) SetEnums(enums map[string][]string) {
	h.enums = enums
}

// AddEnum adds one enum to the map of enums used in generating the schema.
// You can call AddEnum repeatedly instead of using SetEnums.
func (h *gql) AddEnum(name string, values []string) {
	if h.enums == nil {
		h.enums = make(map[string][]string)
	}
	h.enums[name] = values
}

// SetQuery adds or replaces the struct representing the root query type
func (h *gql) SetQuery(query interface{}) {
	h.qms[0] = query
}

// SetMutation adds or replaces the struct representing the root mutation type
func (h *gql) SetMutation(mutation interface{}) {
	h.qms[1] = mutation
}

// SetSubscription adds or replaces the struct representing the subscription type
func (h *gql) SetSubscription(subscription interface{}) {
	h.qms[2] = subscription
}

// GetSchema builds and returns the GraphQL schema
func (h *gql) GetSchema() (string, error) {
	s, err := schema.Build(h.enums, h.qms[:]...)
	if err != nil {
		return "", err
	}
	return s, nil
}

// GetHandler builds the schema and returns the HTTP handler that handles GraphQL queries
func (h *gql) GetHandler() (http.Handler, error) {
	s, err := schema.Build(h.enums, h.qms[:]...)
	if err != nil {
		return nil, err
	}
	return handler.New([]string{s}, h.enums, [3][]interface{}{{h.qms[0]}, {h.qms[1]}, {h.qms[2]}}), nil
}
