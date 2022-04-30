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

var q = struct {
	// This works with latest "id_field" code but only if the Album (struct) is seen in the field with "field_id" specified.
	// If the order of fields below is reversed then the "Album" type is added to the schema without any "id" field.
	// But there is a bigger problem with "field_id": if we use the same struct as element of different maps/slices
	//  - we can have different names for the fabricated "id" field but can only have one name in the schema
	//  - the "id" field might need different types - eg int for a slice, string (etc) for map key

	Albums map[string]Album `egg:",field_id"`
	Album  map[string]Album `egg:",subscript"`
}{
	Album:  albums,
	Albums: albums,
}

func main() {
	http.Handle("/graphql", eggql.MustRun(q))
	http.ListenAndServe(":8080", nil)
}
