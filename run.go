package eggql

// run.go provides the MustRun function for quickly creating a GraphQL http handler

import (
	"net/http"

	"github.com/andrewwphillips/eggql/internal/handler"
	"github.com/andrewwphillips/eggql/internal/schema"
)

// MustRun creates an http handler that handles GraphQL requests.
// It is a variadic function taking these parameters in this order:
//  enums (opt)  - map[string][]string = all the enums that are used in the resolvers
//  query        - *struct = pointer to a struct used to generate the GraphQL query
//  mutation     - *struct = pointer to struct used to generate the GraphQL mutation
//  subscription - *struct = pointer to struct used to generate the GraphQL subscription
// Notes
// 1) If the 1st parameter is not a map (ie, no enums required) then it is assumed it is the query
// 2) The next (1 to 3) parameters must be pointers to structs (or nil) for query/mutation/subscription
// 3) It is pointless not to have at least one (non-nil) struct ptr
// 4) If you only require a mutation you must still provide the previous query parameter (as nil)
// 5) If only a subscription still provide the query and mutations parameters (as nil)
// (The *types* of the query/mutation/subscription structs, including metadata from field "egg:" tag
// strings, are used to generate a GraphQL schema, whereas the actual *values* of these parameters
// are the GraphQL "resolvers" used to obtain query results.)
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
