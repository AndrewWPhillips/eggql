package eggql

// run.go provides the MustRun function for quickly creating a GraphQL http handler

import (
	"net/http"

	"github.com/andrewwphillips/eggql/internal/handler"
	"github.com/andrewwphillips/eggql/internal/schema"
)

// MustRun creates an http handler that handles GraphQL requests.  It takes up to 4
// parameters. The 1st (optional) parameter is a map of string slices that contains
// all the GraphQL enums that are used with the queries. The next 3 (opt.) parameters are
// Go structs that are used to generate the GraphQL query, mutation, and subscription.
// The types of these parameters (as well as metadata from field tag strings) are used
// to generate a GraphQL schema , whereas the actual value of these parameters are the
// GraphQL "resolvers" used to obtain query results.
func MustRun(q ...interface{}) http.Handler {
	return handler.New(schema.MustBuild(q...), q...)
}
