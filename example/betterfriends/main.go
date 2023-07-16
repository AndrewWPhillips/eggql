package main

import (
	"github.com/andrewwphillips/eggql"
	"net/http"
	"time"
)

type Friend struct {
	Dob   eggql.Time `egg:"dob"`
	Email string     `egg:"email"`
}

var friends = map[string]*Friend{
	"Alice": {Dob: Date(2006, 1, 2), Email: "alice@example.com"},
	"Bob":   {Dob: Date(1964, 2, 21)},
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
	//gql := eggql.New(q)
	//log.Println(gql.GetSchema())
}

func Date(y, m, d int) eggql.Time {
	return eggql.Time(time.Date(y, time.Month(m), d, 0, 0, 0, 0, time.UTC))
}
