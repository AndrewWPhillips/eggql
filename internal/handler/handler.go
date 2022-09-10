// Package handler implements an HTTP handler to process GraphQL queries (and
// mutations/subscriptions) given an instance of a query struct (and optionally
// mutation and subscription structs) and a corresponding GraphQL schema.
// The schema is typically generated (by the schema package) from the same struct(s).
package handler

// handler.go implements handler.New() to create a new handler, and it's ServeHTTP method

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"strconv"
	"strings"

	"github.com/vektah/gqlparser/v2"
	"github.com/vektah/gqlparser/v2/ast"
	"github.com/vektah/gqlparser/v2/gqlerror"
)

type (
	// Handler stores the invariants (schema and structs) used in the GraphQL requests
	Handler struct {
		schema       *ast.Schema
		enums        map[string][]string       // each enum is a slice of strings
		enumsReverse map[string]map[string]int // allows reverse lookup - int value given enum value (string)

		// qData, mData and subscriptionData provide the resolvers for queries, mutations and subscriptions
		// respectively.  Note that each typically has only one element except that qData may also have
		// introspection data (as returned by NewIntrospectionData) but they could have more elements if
		// multiple schemas are combined (stitched).
		qData            []interface{}
		mData            []interface{}
		subscriptionData []interface{}

		// The following are for websockets
		subscriptionCancel map[string]context.CancelFunc // allows cancel of subscription(s) when connection is closed
	}
)

// New creates a new handler with the given schema(s) and query/mutation/subscription struct(s)
// Parameters:
//   schemaStrings - a slice of strings containing the GraphQL schemas (typically only 1)
//   enums - a map of enum names to a slice of strings containing the enum values for all the schemas
//   qms - a slice of query/mutation/subscription structs where:
//     qms[0] - query struct(s)
//     qms[1] - mutation struct(s)
//     qms[2] - subscription struct(s)
func New(schemaStrings []string, enums map[string][]string, qms [3][]interface{}) http.Handler {
	var sources []*ast.Source

	for i, str := range schemaStrings {
		sources = append(sources, &ast.Source{Name: "schema " + strconv.Itoa(i+1), Input: str})
	}

	r := &Handler{}
	var pgqlError *gqlerror.Error
	r.schema, pgqlError = gqlparser.LoadSchema(sources...)
	if pgqlError != nil {
		log.Fatalf("eggql.handler.New - error making schema error %s\n", error(pgqlError))
	}

	r.enums = make(map[string][]string, len(enums))
	r.enumsReverse = make(map[string]map[string]int, len(enums))
	for enumName, list := range enums {
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

	r.qData = qms[0]
	r.mData = qms[1]
	r.subscriptionData = qms[2]

	if AllowIntrospection {
		// Add data for introspection
		r.qData = append(r.qData, NewIntrospectionData(r.schema))
		for enumName, list := range IntroEnums {
			enum := make([]string, 0, len(list))
			enumInt := make(map[string]int, len(list))
			for i, v := range list {
				enum = append(enum, v)
				enumInt[v] = i
			}
			r.enums[enumName] = enum
			r.enumsReverse[enumName] = enumInt
		}
	}
	return r
}

// ServerHTTP receives a GraphQL query as an HTTP request, executes the
// query (or mutation) and generates an HTTP response or error message
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method == "OPTIONS" {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		w.Header().Set("Access-Control-Allow-Methods", "POST, GET, OPTIONS")
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if r.Header.Get("Upgrade") == "websocket" {
		// Call websocket handler
		h.serveWS(w, r)
		return
	}
	w.Header().Set("Content-Type", "application/graphql+json")
	if r.Method != http.MethodGet && r.Method != http.MethodPost {
		http.Error(w, "GraphQL queries must use GET or POST", http.StatusMethodNotAllowed)
		return
	}

	// Decode the request (JSON)
	g := gqlRequest{h: h}
	if r.Method == http.MethodGet {
		values := r.URL.Query()
		// find the query parameter with name "query" which contains the GraphQL query (or mutation or subscription)
		if len(values["query"]) != 1 {
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte(`{"data": null,"errors": [{"message": "Error: query parameter is required"}]}`))
			return
		}
		g.Query = values["query"][0]
		if len(values["variables"]) > 0 {
			vars := values["variables"][0]
			if len(vars) > 1 && vars[0] == '"' && vars[len(vars)-1] == '"' {
				vars = vars[1 : len(vars)-1] // remove quotes if present
			}
			decoder := json.NewDecoder(strings.NewReader(vars))
			decoder.UseNumber() // allows us to distinguish ints from floats (see FixNumberVariables() below)
			if err := decoder.Decode(&g.Variables); err != nil {
				w.WriteHeader(http.StatusBadRequest)
				w.Write([]byte(`{"data": null,"errors": [{"message": "Error decoding JSON variables:` + err.Error() + `"}]}`))
				return
			}
		}
	} else {
		decoder := json.NewDecoder(r.Body)
		decoder.DisallowUnknownFields() // quickly find if a field name has been misspelt
		decoder.UseNumber()             // allows us to distinguish ints from floats (see FixNumberVariables() below)
		if err := decoder.Decode(&g); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte(`{"data": null,"errors": [{"message": "Error decoding JSON request:` + err.Error() + `"}]}`))
			return
		}
	}

	// Since variables are sent as JSON (which does not distinguish int/float) we need to decide
	FixNumberVariables(g.Variables)

	// Execute it and write the result or error to the HTTP response
	if buf, err := json.Marshal(g.ExecuteHTTP(r.Context())); err != nil {
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
