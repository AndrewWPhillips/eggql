package main

import (
	"github.com/andrewwphillips/eggql"
	"net/http"
)

type (
	Query struct {
		Hero func(episode int) interface{} `graphql:"hero:Character,args(episode:Episode=JEDI)"`
		_    Character
		_    Human
		_    Droid
	}
	Character struct {
		Name    string
		Friends []*Character
		Appears []int `graphql:"appearsIn:[Episode]"`
	}
	Human struct {
		Character
		Height float64 // meters
	}
	Droid struct {
		Character
		PrimaryFunction string
	}
	EpisodeDetails struct {
		Name   string
		HeroId int
	}
)

var (
	gqlEnums = map[string][]string{
		"Episode": {"NEWHOPE", "EMPIRE", "JEDI"},
		"Unit":    {"METER", "FOOT"},
	}
	humans = []Human{
		{Character{Name: "Luke Skywalker"}, 1.67},
		{Character{Name: "Leia Organa"}, 1.65},
		{Character{Name: "Han Solo"}, 1.85},
		{Character{Name: "Chewbacca"}, 2.3},
	}
	droids = []Droid{
		{Character{Name: "R2-D2"}, "Astromech"},
		{Character{Name: "C-3PO"}, "Protocol"},
	}
	episodes = []EpisodeDetails{
		{Name: "A New Hope", HeroId: 1000},
		{Name: "The Empire Strikes Back", HeroId: 1000},
		{Name: "Return of the Jedi", HeroId: 2000},
	}
)

func main() {
	// Set up friendships
	luke := &humans[0].Character
	leia := &humans[1].Character
	solo := &humans[2].Character
	chew := &humans[3].Character
	r2d2 := &droids[0].Character
	c3po := &droids[1].Character

	humans[0].Friends = []*Character{leia, solo, chew, r2d2}
	humans[1].Friends = []*Character{luke, solo, r2d2, c3po}
	humans[2].Friends = []*Character{chew, leia, luke}
	humans[3].Friends = []*Character{solo, luke}

	droids[0].Friends = []*Character{c3po, luke, leia}
	droids[1].Friends = []*Character{r2d2, leia}

	// Set up appearances
	humans[0].Appears = []int{0, 1, 2}
	humans[1].Appears = []int{0, 1, 2}
	humans[2].Appears = []int{0, 1, 2}
	humans[3].Appears = []int{0, 1, 2}
	droids[0].Appears = []int{0, 1, 2}
	droids[1].Appears = []int{0, 1, 2}

	http.Handle("/graphql", eggql.MustRun(gqlEnums, Query{Hero: func(episode int) interface{} {
		if episode < 0 || episode >= len(episodes) {
			return nil
		}
		ID := episodes[episode].HeroId
		if ID >= 2000 {
			// droids have IDs starting at 2000
			ID -= 2000
			if ID > len(droids) {
				return nil
			}
			return droids[ID]
		}
		// humans have IDs starting at 1000
		ID -= 1000
		if ID < 0 || ID > len(humans) {
			return nil
		}
		return humans[ID]
	}}))
	http.ListenAndServe(":8080", nil)
}
