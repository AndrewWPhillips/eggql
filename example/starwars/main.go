package main

import (
	"fmt"
	"github.com/andrewwphillips/eggql"
	"net/http"
	"time"
)

const (
	FirstHumanID = 1000
	FirstDroidID = 2000
)

type (
	Query struct {
		Hero func(episode int) (interface{}, error) `graphql:"hero:Character,args(episode:Episode=JEDI)"`
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
		Height func(int) float64 `graphql:",args(unit:Unit=METER)"`
		height float64           // meters
	}
	Droid struct {
		Character
		PrimaryFunction string
	}
	EpisodeDetails struct {
		Name       string
		HeroId     int
		Stars      int
		Commentary string
	}

	Mutation struct {
		CreateReview func(int, ReviewInput) *EpisodeDetails `graphql:",args(episode:Episode,review)"`
	}
	ReviewInput struct {
		Stars      int
		Commentary string
	}
)

var (
	gqlEnums = map[string][]string{
		"Episode": {"NEWHOPE", "EMPIRE", "JEDI"},
		"Unit":    {"METER", "FOOT"}, // order of strings should match METER, etc consts below
	}
)

const (
	METER = iota
	FOOT
)

var (
	humans = []Human{
		{Character: Character{Name: "Luke Skywalker"}, height: 1.67},
		{Character: Character{Name: "Leia Organa"}, height: 1.65},
		{Character: Character{Name: "Han Solo"}, height: 1.85},
		{Character: Character{Name: "Chewbacca"}, height: 2.3},
	}
	droids = []Droid{
		{Character: Character{Name: "C-3PO"}, PrimaryFunction: "Protocol"},
		{Character: Character{Name: "R2-D2"}, PrimaryFunction: "Astromech"},
	}
	episodes = []EpisodeDetails{
		{Name: "A New Hope", HeroId: 1000},
		{Name: "The Empire Strikes Back", HeroId: 1000},
		{Name: "Return of the Jedi", HeroId: 2001},
	}
)

func init() {
	// Set up friendships
	luke := &humans[0].Character
	leia := &humans[1].Character
	solo := &humans[2].Character
	chew := &humans[3].Character
	c3po := &droids[0].Character
	r2d2 := &droids[1].Character

	humans[0].Friends = []*Character{leia, solo, chew, r2d2}
	humans[1].Friends = []*Character{luke, solo, r2d2, c3po}
	humans[2].Friends = []*Character{chew, leia, luke}
	humans[3].Friends = []*Character{solo, luke}

	droids[0].Friends = []*Character{r2d2, leia}
	droids[1].Friends = []*Character{c3po, luke, leia}

	// Set up human Height closure
	for i := range humans {
		humans[i].Height = (&humans[i]).getHeight
	}
	// Set up appearances
	humans[0].Appears = []int{0, 1, 2}
	humans[1].Appears = []int{0, 1, 2}
	humans[2].Appears = []int{0, 1, 2}
	humans[3].Appears = []int{0, 1, 2}
	droids[0].Appears = []int{0, 1, 2}
	droids[1].Appears = []int{0, 1, 2}
}

func main() {
	handler := eggql.MustRun(
		gqlEnums,
		Query{
			Hero: func(episode int) (interface{}, error) {
				if episode < 0 || episode >= len(episodes) {
					return nil, fmt.Errorf("episode %d not found", episode)
				}
				ID := episodes[episode].HeroId
				if ID >= FirstDroidID {
					// droids have IDs starting at FirstDroidID
					ID -= FirstDroidID
					if ID < len(droids) {
						return droids[ID], nil
					}
				}
				// humans have IDs starting at FirstHumanID
				ID -= FirstHumanID
				if ID > 0 && ID < len(humans) {
					return humans[ID], nil
				}
				return nil, fmt.Errorf("internal error: no character with ID %d in episode %d", ID, episode)
			},
		},
		Mutation{
			CreateReview: func(episode int, review ReviewInput) *EpisodeDetails {
				if episode < 0 || episode >= len(episodes) {
					return nil
				}
				episodes[episode].Stars = review.Stars
				episodes[episode].Commentary = review.Commentary
				return &episodes[episode]
			},
		},
	)
	handler = http.TimeoutHandler(handler, 5*time.Second, `{"errors":[{"message":"timeout"}]}`)
	http.Handle("/graphql", handler)
	http.ListenAndServe(":8080", nil)
}

// getHeight returns the height of a human
// Parameters
//  h (receiver) is a pointer to the Human
//  unit is the unit for the return value (FOOT or METER)
func (h *Human) getHeight(unit int) float64 {
	switch unit {
	case METER:
		// nothing here - height is already in meters
	case FOOT:
		return h.height * 3.28084
	default:
		panic("unknown unit value")
	}
	return h.height
}
