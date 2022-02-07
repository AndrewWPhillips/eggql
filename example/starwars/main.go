package main

import (
	"github.com/andrewwphillips/eggql"
	"net/http"
)

type (
	Query struct {
		Hero func(episode int) *Character `graphql:",args(episode=2)"`
	}
	Character struct {
		Name    string
		Friends []*Character
	}
	EpisodeDetails struct {
		Name   string
		HeroId int
	}
)

var (
	characters = []Character{
		{Name: "Luke Skywalker"},
		{Name: "Leia Organa"},
		{Name: "Han Solo"},
		{Name: "R2-D2"},
	}
	episodes = []EpisodeDetails{
		{Name: "A New Hope", HeroId: 0},
		{Name: "The Empire Strikes Back", HeroId: 0},
		{Name: "Return of the Jedi", HeroId: 3},
	}
)

func main() {
	// Set up friendships
	characters[0].Friends = []*Character{&characters[1], &characters[2], &characters[3]}
	characters[1].Friends = []*Character{&characters[0], &characters[2], &characters[3]}
	characters[2].Friends = []*Character{&characters[0], &characters[1]}
	characters[3].Friends = []*Character{&characters[0], &characters[1]}

	http.Handle("/graphql", eggql.MustRun(Query{Hero: func(episode int) *Character {
		if episode < 0 || episode >= len(episodes) {
			return nil
		}
		return &characters[episodes[episode].HeroId]
	}}))
	http.ListenAndServe(":8080", nil)
}
