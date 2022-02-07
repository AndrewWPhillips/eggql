package main

import (
	"github.com/andrewwphillips/eggql"
	"net/http"
)

type (
	Query struct {
		Hero Character
	}
	Character struct {
		Name    string
		Friends []*Character
	}
)

func main() {
	http.Handle("/graphql", eggql.MustRun(
		Query{
			Hero: Character{
				Name: "R2-D2",
				Friends: []*Character{
					{Name: "Leia Organa"},
					{Name: "Luke Skywalker"},
					{Name: "Han Solo"},
					{Name: "C-3PO"},
				},
			},
		}))
	http.ListenAndServe(":8080", nil)
}
