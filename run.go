package eggql

// run.go provides the eggql.MustRun() function to quickly create a GraphQL HTTP handler

import (
	"net/http"

	"github.com/andrewwphillips/eggql/internal/handler"
	"github.com/andrewwphillips/eggql/internal/schema"
)

// MustRun creates an http handler that handles GraphQL requests.
// It is a variadic function so can take any number of parameters but to be useful
// you need to supply at least one parameter - a struct used as the root query resolver.
// The parameters are optional but should be supplied in this order:
//
//		map[string][]string = all the enums that are used in the resolvers
//		*struct = pointer to struct used to generate the GraphQL query (may be nil)
//		*struct = pointer to struct used to generate the GraphQL mutation (may be nil)
//		*struct = pointer to struct used to generate the GraphQL subscription
//	    func(*options) = zero or more options closure(s) as returned by eggql.FuncCache, etc
//
// Notes
// 1) If the 1st parameter is not a map (ie, no enums required) then it is assumed it is the query
// 2) The next (1 to 3) parameters must be pointers to structs (or nil) for query/mutation/subscription
// 3) It is pointless not to have at least one (non-nil) struct ptr
// 4) If you only require a mutation you must still provide the previous nil parameter (query)
// 5) If only a subscription still provide the query and mutations parameters (as nil)
// (The *types* of the query/mutation/subscription structs, including metadata from field "egg:" tag
// strings, are used to generate a GraphQL schema, whereas the actual *values* of these parameters
// are the GraphQL "resolvers" used to obtain query results.)
// 6) Zero or more options can follow the last *struct parameter
func MustRun(params ...interface{}) http.Handler {
	var enums map[string][]string
	var qms [3][]interface{}

	schemaParams := make([]interface{}, 0, 4) // parameters to schema.MustBuild
	p := params
	// Check for enums
	if len(p) > 0 {
		if e, ok := p[0].(map[string][]string); ok {
			enums = e
			schemaParams = append(schemaParams, p[0]) // schema also needs enums
			p = p[1:]
		}
	}
	// Get query (and mutation/subscription if provided)
	for i := 0; i < 3 && len(p) > 0; i++ {
		if _, ok := p[0].(func(*options)); ok {
			break
		}
		qms[i] = []interface{}{p[0]}
		schemaParams = append(schemaParams, p[0])
		p = p[1:]
	}
	// Get any options from the rest of the parameters (if any)
	var allOptions options
	for _, param := range p {
		if option, ok := param.(func(*options)); !ok {
			panic("unexpected parameter type in MustRun - expected an option")
		} else {
			option(&allOptions)
		}
	}

	return handler.New(
		[]string{schema.MustBuild(schemaParams...)},
		enums,
		qms,
		handler.FuncCache(allOptions.funcCache),
		handler.NoIntrospection(allOptions.noIntrospection),
		handler.NoConcurrency(allOptions.noConcurrency),
		handler.NilResolverAllowed(allOptions.nilResolver),
		handler.InitialTimeout(allOptions.initialTimeout),
		handler.PingFrequency(allOptions.pingFrequency),
		handler.PongTimeout(allOptions.pongTimeout),
	)
}
