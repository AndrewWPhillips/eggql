package main

import (
	"github.com/andrewwphillips/eggql"
	"net/http"
)

// Album stores info about a record album.
type Album struct {
	Title  string
	Artist string
	Price  int
}

var albums = map[string]Album{
	"1": {Title: "Blue Train", Artist: "John Coltrane", Price: 56_99},
	"2": {Title: "Jeru", Artist: "Gerry Mulligan", Price: 17_99},
	"3": {Title: "Sarah Vaughan and Clifford Brown", Artist: "Sarah Vaughan", Price: 39_99},
}

type Query struct {
	Album map[string]Album `graphql:",subscript"`
}

func main() {
	http.Handle("/graphql", eggql.MustRun(Query{Album: albums}))
	http.ListenAndServe(":8080", nil)
}
