package main

import (
	"net/http"

	"github.com/andrewwphillips/eggql"
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
	Albums map[string]Album // query list of all albums
	Album  map[string]Album `egg:",subscript"` // query one album using arg "id"
}

func main() {
	http.Handle("/graphql", eggql.MustRun(Query{Albums: albums, Album: albums}))
	http.ListenAndServe(":8080", nil)
}
