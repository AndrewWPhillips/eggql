// Package handler implements an HTTP handler to process GraphQL queries (and
// mutations/subscriptions) given an instance of a query struct (and optionally
// mutation and subscription structs) and a corresponding GraphQL schema.
// The schema is typically generated (by the schema package) using the same struct(s).
package handler

// handler.go implements handler.New() (to create a new handler) and implements the
// returned handler's ServeHTTP method (hence implements http.Handler interface)

import (
	"encoding/json"
	"log"
	"net/http"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/vektah/gqlparser/v2"
	"github.com/vektah/gqlparser/v2/ast"
	"github.com/vektah/gqlparser/v2/gqlerror"
)

type (
	// ResolverLookupTables - store info on the all resolvers of all query structs
	// The key to the outer map is the type of the struct type containing the resolver
	// The key of the inner map is the name of the resolver (based on the field name or metadata)
	// The info currently has 2 parts
	//  - index of the resolver field within the struct
	//  - a cache of the resolver values returned so far
	ResolverLookupTables map[reflect.Type]map[string]ResolverData

	// ResolverData stores info related to a resolver (field of q query struct)
	//  - index of the resolver field  (to avoid a linear search of the fields to find the resolver by name)
	//  - cache of values of the resolver
	ResolverData struct {
		Index int // index of the resolver field in the parent struct
		// ResolverCache contains cached values of the resolver or is nil if the reeolver does not allow caching
		// Note: the map is created (or set to nil) before handling of queries so reading the map itself is safe
		// to do concurrently but modifying its contents (adding entries, etc) must be protected with the mutex
		Cache ResolverCache // cached values of this resolver
	}

	// CacheKey allows us to uniquely identify a cached value for a resolver
	CacheKey struct {
		fieldValue reflect.Value // the Go data (struct field) holding the resolver value
		// TODO: allow for private cache by also including a connection (string?) in the key, also check if we need args in the key
	}
	// ResolverCache contains a map (see CacheKey above) and a mutex to protect concurrent access to it
	ResolverCache struct {
		Mtx   *sync.Mutex                // protects concurrent access of the following map
		Saved map[CacheKey]reflect.Value // cached values of the resolver
	}

	// Handler stores the invariants (schema and structs) used in the GraphQL requests
	Handler struct {
		schema       *ast.Schema
		enums        map[string][]string       // each enum is a slice of strings
		enumsReverse map[string]map[string]int // allows reverse lookup - int value given enum value (string)

		// resolverLookup provides a lookup map for every struct used in a query/mutation/subscription.
		// At the top level we have a map where each key is the type of the struct and the value is the lookup map
		// For each lookup map the key is the resolver name (string) and the value is info about the resolver
		//  - index of the field in the struct that is used to resolve the query
		//  - cache of previously seen values for this resolver
		resolverLookup ResolverLookupTables

		// qData, mData and subscriptionData provide the resolvers for queries, mutations and subscriptions
		// respectively.  Note that each typically has only one element except that qData may also have
		// introspection data (as returned by NewIntrospectionData) but they could have more elements if
		// multiple schemas are combined (stitched).
		qData            []interface{}
		mData            []interface{}
		subscriptionData []interface{}

		// resolver options
		funcCache       bool // In the absence of cache directives results of resolver functions are cached (forever)
		noIntrospection bool // Disallows introspection queries
		noConcurrency   bool // Disables concurrent processing of queries (though mutations are never processed concurrently)
		nilResolver     bool // If a resolver is a nil func then the resolver returns null instead of an error

		// websocket options
		initialTimeout time.Duration // how long to wait for connection_init after the WS is opened
		pingFrequency  time.Duration // how often to send a ping (ka in old protocol) message to the client
		pongTimeout    time.Duration // how long to wait for a pong after sending a ping
	}
)

// New creates a new handler with the given schema(s) and query/mutation/subscription struct(s)
// Parameters:
//
//		schemaStrings - a slice of strings containing the GraphQL schema(s) - typically only 1
//		enums - a map of enum names to a slice of strings containing the enum values for all the schemas
//		qms - a slice of query/mutation/subscription structs where:
//		  qms[0] - query struct(s)
//		  qms[1] - mutation struct(s)
//		  qms[2] - subscription struct(s)
//		options - zero or more options returned by calls to:
//	      handler.FuncCache
//	      handler.NoIntrospection
//	      handler.NoConcurrency
//	      handler.NilResolver
//		  handler.InitialTimeout
//		  handler.PingFrequency
//		  handler.PongTimeout
func New(schemaStrings []string, enums map[string][]string, qms [3][]interface{}, options ...func(*Handler),
) http.Handler {
	h := &Handler{}
	h.SetOptions(options...)

	// Build the list of source (text) schemas - typically just one (but LoadSchemas can handle more than one)
	var sources []*ast.Source
	for i, str := range schemaStrings {
		sources = append(sources, &ast.Source{Name: "schema " + strconv.Itoa(i+1), Input: str})
	}

	// Generate the "binary" schema from the "source" schema(s)
	var pgqlError *gqlerror.Error
	h.schema, pgqlError = gqlparser.LoadSchema(sources...)
	if pgqlError != nil {
		log.Fatalf("eggql.handler.New - error making schema error %s\n", error(pgqlError))
	}

	h.enums, h.enumsReverse = makeEnumTables(enums)

	h.qData = qms[0]
	h.mData = qms[1]
	h.subscriptionData = qms[2]

	if !h.noIntrospection {
		// Add data for introspection
		h.qData = append(h.qData, NewIntrospectionData(h.schema))
		for enumName, list := range IntroEnums {
			enum := make([]string, 0, len(list))
			enumInt := make(map[string]int, len(list))
			for i, v := range list {
				enum = append(enum, v)
				enumInt[v] = i
			}
			h.enums[enumName] = enum
			h.enumsReverse[enumName] = enumInt
		}
	}

	h.makeResolverTables()

	return h
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

	// Decode the GET or POST request (JSON)
	g := gqlRequest{Handler: h}
	if r.Method == http.MethodGet {
		// if it's a GET we assume the GraphQL query is passed as a "query" query parameter
		values := r.URL.Query()
		// find the query parameter with name "query" which contains the GraphQL query (or mutation or subscription)
		if len(values["query"]) != 1 {
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte(`{"data": null,"errors": [{"message": "Error: query parameter is required"}]}`))
			return
		}
		g.Query = values["query"][0]
		// get GraphQL variables from "variables" query parameter
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
		// for POST requests we assume the GraphQL query (+ optionally variables) are JSON encoded in the request body
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
