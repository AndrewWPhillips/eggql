package main

import (
	"github.com/andrewwphillips/eggql"
	"net/http"
)

type Friend struct{ Name string }

// Create a query for a list of friends
var q = struct{ Friends []Friend }{
	Friends: []Friend{{"Alice"}, {"Bob"}, {"Carol"}},
}

func main() {
	http.Handle("/graphql", eggql.MustRun(q))
	http.ListenAndServe(":8080", nil)
}
