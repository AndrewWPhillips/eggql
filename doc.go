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
// allow multiple schemas - stitching?
// subscriptions
// complexity limiting:
//   - add complexity(len, <int>, <arg>, <arg>) option
//     where len = length for field returning a list and the value *can* be precalculated
//           <int> = integer literal (eg 10)
//           <arg> = integer argument
//   -  calc complexity (recursively) before running a root query (if below option on) (eg <int>*<arg>*<arg>)
//   -  add complexity limit option
// caching of requests?
// dataloader
// finish introspection (__type.interfaces, directives)
