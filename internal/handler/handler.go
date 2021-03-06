// Package handler implements an HTTP handler to process GraphQL queries (and
// mutations/subscriptions) given an instance of a query struct (and optionally
// mutation and subscription structs) and a corresponding GraphQL schema.
// The schema is typically generated (by the schema package) from the same struct(s).
package handler

// handler.go implements handler.New() to create a new handler, and it's ServeHTTP method

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/vektah/gqlparser/v2"
	"github.com/vektah/gqlparser/v2/ast"
)

type (
	// Handler stores the invariants (schema and structs) used in the GraphQL requests
	Handler struct {
		schema       *ast.Schema
		enums        map[string][]string       // each enum is a slice of strings
		enumsReverse map[string]map[string]int // allows reverse lookup - int value given enum value (string)
		qData        interface{}
		mData        interface{}
		//subscriptionData interface{}
	}
)

// New is the main handler function that returns an HTTP handler given a schema (and enums(s)) PLUS
// corresponding instances of query and optionally mutation and subscription structs.
func New(schemaString string, qms ...interface{}) http.Handler {
	schema, pgqlError := gqlparser.LoadSchema(&ast.Source{
		Name:  "schema",
		Input: schemaString,
	})
	if pgqlError != nil {
		panic("eggql.handler.New - error making schema: " + pgqlError.Message)
	}

	r := &Handler{
		schema: schema,
	}
	if rawEnums, ok := qms[0].(map[string][]string); ok {
		r.enums = make(map[string][]string, len(rawEnums))
		r.enumsReverse = make(map[string]map[string]int, len(rawEnums))
		for enumName, list := range rawEnums {
			enum := make([]string, 0, len(list))
			enumInt := make(map[string]int, len(list))
			for i, v := range list {
				v = strings.SplitN(v, "#", 2)[0] // remove description
				v = strings.SplitN(v, "@", 2)[0] // remove directive(s)
				v = strings.TrimRight(v, " ")    // remove trailing spaces
				enum = append(enum, v)
				enumInt[v] = i
			}
			name := strings.TrimRight(strings.SplitN(enumName, "#", 2)[0], " ")
			r.enums[name] = enum
			r.enumsReverse[name] = enumInt
		}

		// Skip the enums, to get the query, mutation, subscription
		qms = qms[1:]
	}
	r.qData = qms[0]
	if len(qms) > 1 {
		r.mData = qms[1]
	}
	// TODO: implement subscriptions
	//if len(qms) > 2 {
	//	r.subscriptionData = qms[2]
	//}

	return r
}

// ServerHTTP receives a GraphQL query as an HTTP request, executes the
// query (or mutation) and generates an HTTP response or error message
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method == "OPTIONS" {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
		w.WriteHeader(http.StatusNoContent)
		return
	}
	w.Header().Set("Content-Type", "application/graphql+json")
	if r.Method != http.MethodGet && r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	// Decode the request (JSON)
	g := gqlRequest{h: h}
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields() // quickly find if a field name has been misspelt
	decoder.UseNumber()             // allows us to distinguish ints from floats (see FixNumberVariables() below)
	if err := decoder.Decode(&g); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"data": null,"errors": [{"message": "Error decoding JSON request:` + err.Error() + `"}]}`))
		return
	}

	// Since variables are sent as JSON (which does not distinguish int/float) we need to decide
	FixNumberVariables(g.Variables)

	// Execute it and write the result or error
	// TODO work out how to let caller provide their own context or at least a timeout option
	if buf, err := json.Marshal(g.Execute(r.Context())); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"data": null,"errors": [{"message": "Error encoding JSON response:` + err.Error() + `"}]}`))
	} else {
		w.Write(buf)
	}
}

// FixNumberVariables goes through the structure created by the JSON decoder, converting any json.Number values to
// either an int64 or a float64.  This assumes that all the JSON numbers were decoded into a json.Number type, rather
// than int/float, by use of the json.Decode.UseNumber() method.
func FixNumberVariables(m map[string]interface{}) {
	for key, val := range m {
		switch v := val.(type) {
		case json.Number:
			if i, err := v.Int64(); err == nil {
				m[key] = i
				continue
			}
			if f, err := v.Float64(); err == nil {
				m[key] = f
				continue
			}
			// TODO check if we can ever get here due to an error in JSON variables (NOT a bug) & add error return
			panic("JSON number decode error") // it must be an int or float

		case map[string]interface{}:
			FixNumberVariables(v) // recursively handle nested numbers

			// TODO check if we need to handle JSON lists which decode as []interface{}
		}
	}
}
