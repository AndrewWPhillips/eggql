// Package eggql allows you to easily build a GraphQl server in Go.
// (EGGQL might be an acronym for Easy Go Graph Query Language.)

// It's easy because you don't have to worry about schemas and
// getting run-time panics due to inconsistencies between your
// GraphQL schema and the corresponding Go structures.  It effectively
// builds a GraphQL schema for you based on your data structures.
// For example, here is the code for a complete GraphQL server:

//package main
//
//import (
//    "github.com/andrewwphillips/eggql"
//)
//func main() {
//	http.Handle("/graphql", eggql.New(struct{ Message string }{Message: "hello, world"}))
//	http.ListenAndServe(":80", nil)
//}

// This creates a GraphQl object called "message", that can be used in a query like this:
// {
//    message
// }

// which will return this JSON:
// {
//    "data": {
//      "message": "hello, world"
//    }
// }

// Of course, you can do much more sophisticated stuff, usually very easily, such
// as mutations with query parameters.  In fact, you can do most normal GraphQL
// stuff (apart from subscriptions and a few other things - see TO-DO list below)
// To create bullet-proof servers, resolver functions also (optionally) support a
// context parameter (of context.Context type) and error returns (of error type).

// See the README.md file for more details on using the package.

package eggql

// TODO:
// object param with an enum field
// GraphQL parallel execution of queries (but not mutations)
// allow map to return a list where key = a field (default name ID)
// allow slice/array to return list where ID = index
// add ID type
// ext. types
// add Date (extension) type
// null handling and non-nullability
// unions
// subscriptions
// more systematic error handling
// caching of requests
// dataloader
// finish introspection (__type.interfaces, input types, directives)
