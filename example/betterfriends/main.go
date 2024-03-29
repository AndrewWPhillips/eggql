package main

// This is the "Better Friends" example from the README.md file

import (
	"net/http"
	"time"

	"github.com/andrewwphillips/eggql"
)

type Friend struct {
	Dob   eggql.Date
	Email string
}

var friends = map[string]*Friend{
	"Alice": {Dob: Date(2006, 1, 2), Email: "alice@example.com"},
	"Bob":   {Dob: Date(1964, 2, 21), Email: "bob@example.com"},
	"Carol": {Dob: Date(1996, 4, 16)},
}

// Create a query for a list of friends
var q = struct {
	List   map[string]*Friend `egg:"friends,field_id=name"`
	Single map[string]*Friend `egg:"friend,subscript=name"`
}{
	List:   friends,
	Single: friends,
}

func main() {
	http.Handle("/graphql", eggql.MustRun(q))
	http.ListenAndServe(":8080", nil)
}

// Date creates an eggql.Date given (numeric) year, month and day
func Date(y, m, d int) eggql.Date {
	return eggql.Date(time.Date(y, time.Month(m), d, 0, 0, 0, 0, time.UTC))
}
