package eggql

// eggql.go provides the gql type for generating a GraphQL HTTP handler or schema

import (
	"github.com/andrewwphillips/eggql/internal/handler"
	"github.com/andrewwphillips/eggql/internal/schema"
	"net/http"
)

type (
	gql struct {
		enums map[string][]string
		qms   [3]interface{}
	}
)

// New creates a new instance
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

// SetQuery adds or replaces the struct representing the root query type
func (h *gql) SetQuery(query interface{}) {
	h.qms[0] = query
}

// SetMutation adds or replaces the struct representing the root mutation type
func (h *gql) SetMutation(mutation interface{}) {
	h.qms[1] = mutation
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
	var all []interface{}
	if h.enums != nil {
		all = append(all, h.enums)
	}
	all = append(all, h.qms[:]...)
	return handler.New(s, all...), nil
}
