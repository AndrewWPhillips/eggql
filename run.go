package eggql

// run.go provides the MustRun function for quickly creating a GraphQL http handler

import (
	"net/http"

	"github.com/andrewwphillips/eggql/internal/handler"
	"github.com/andrewwphillips/eggql/internal/schema"
)

// MustRun creates an http handler that handles GraphQL requests.
// It is a variadic function so can take any number of parameters but to be useful
// you need to supply at least one parameter - a struct used as the root query resolver.
// The parameters are optional but should be supplied in this order:
//  map[string][]string = all the enums that are used in the resolvers
//  *struct = pointer to struct used to generate the GraphQL query (may be nil)
//  *struct = pointer to struct used to generate the GraphQL mutation (may be nil)
//  *struct = pointer to struct used to generate the GraphQL subscription
// Note that for the 3 (query/mutation/subscription) struct pointers you must provide the
// previous value(s) even if nil - eg if you just want to provide a mutation struct then
// the parameter preceding it (ie the query) must be nil.
// (The types of the structs, including metadata from field tag strings, are used
// to generate a GraphQL schema, whereas the actual value of these parameters are the
// GraphQL "resolvers" used to obtain query results.)
func MustRun(params ...interface{}) http.Handler {
	var enums map[string][]string
	var qms [3][]interface{}

	q := params
	if len(q) > 0 {
		if e, ok := q[0].(map[string][]string); ok {
			enums = e
			q = q[1:]
		}
	}
	for i, v := range q {
		qms[i] = []interface{}{v}
	}
	return handler.New([]string{schema.MustBuild(params...)}, enums, qms)
}
