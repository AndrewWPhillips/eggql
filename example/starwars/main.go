package main

import (
	"encoding/base64"
	"errors"
	"fmt"
	"github.com/andrewwphillips/eggql"
	"net/http"
	"time"
)

const (
	FirstHumanID    = 1000
	FirstDroidID    = 2000
	FirstStarShipID = 3000
)

type (
	Query struct {
		_        Character                              // needed so eggql knows about Character struct
		Hero     func(episode int) (interface{}, error) `graphql:"hero:Character,args(episode:Episode=JEDI)"`
		Human    func(int) (*Human, error)              `graphql:",args(id = 1000)"`
		Droid    func(int) (*Droid, error)              `graphql:",args(id)"` // id is required
		StarShip func(int) (*StarShip, error)           `graphql:",args(id = 3000)"`
		Reviews  func(episode int) ([]Review, error)    `graphql:",args(episode:Episode)"`
	}
	Character struct {
		Name              string
		Friends           []*Character
		FriendsConnection func(first int, after string) (FriendsConnection, error) `graphql:",args(first=-1, after=\"\")"`
		Appears           []int                                                    `graphql:"appearsIn:[Episode]"`
		SecretBackstory   func() (string, error)
	}
	Human struct {
		Character
		Height     func(int) (float64, error) `graphql:",args(unit:LengthUnit=METER)"`
		height     float64                    // meters
		HomePlanet string
		StarShips  []*StarShip
	}
	Droid struct {
		Character
		PrimaryFunction string
	}
	EpisodeDetails struct {
		Name       string
		HeroId     int
		Stars      []int
		Commentary []string
	}
	Review struct {
		Stars      int
		Commentary string
	}
	StarShip struct {
		Name   string
		Length func(int) (float64, error) `graphql:",args(unit:LengthUnit=METER)"`
		length float64                    // meters
	}

	// Movie reviews
	Mutation struct {
		CreateReview func(int, ReviewInput) (*EpisodeDetails, error) `graphql:",args(episode:Episode,review)"`
	}
	ReviewInput struct {
		Stars      int
		Commentary string
		//Time TODO
	}

	// The following are for pagination of a list of friends
	FriendsConnection struct {
		TotalCount int           // total number of friends
		Edges      []FriendsEdge // list of (subset of) friends
		Friends    []*Character
		PageInfo   PageInfo
	}
	FriendsEdge struct {
		Cursor string
		Node   *Character
	}
	PageInfo struct {
		StartCursor *string
		EndCursor   *string
		HasNextPage bool
	}
)

var (
	gqlEnums = map[string][]string{
		"Episode":    {"NEWHOPE", "EMPIRE", "JEDI"},
		"LengthUnit": {"METER", "FOOT"}, // order of strings in the slice should match METER, etc consts below
	}
)

const (
	METER = iota
	FOOT
)

var (
	humans = []Human{
		{Character: Character{Name: "Luke Skywalker"}, height: 1.67, HomePlanet: "Tatooine"},
		{Character: Character{Name: "Darth Vader"}, height: 2.0, HomePlanet: "Tatooine"},
		{Character: Character{Name: "Han Solo"}, height: 1.85, HomePlanet: "Corellia"},
		{Character: Character{Name: "Leia Organa"}, height: 1.65, HomePlanet: "Alderaa"},
		{Character: Character{Name: "Wilhuff Tarkin"}, height: 1.85, HomePlanet: "Eriadu"},
		{Character: Character{Name: "Chewbacca"}, height: 2.3, HomePlanet: "Kashyyyk"},
	}
	droids = []Droid{
		{Character: Character{Name: "C-3PO"}, PrimaryFunction: "Protocol"},
		{Character: Character{Name: "R2-D2"}, PrimaryFunction: "Astromech"},
	}
	episodes = []EpisodeDetails{
		{Name: "A New Hope", HeroId: FirstHumanID},
		{Name: "The Empire Strikes Back", HeroId: FirstHumanID},
		{Name: "Return of the Jedi", HeroId: FirstDroidID + 1},
	}
	starShips = []StarShip{
		{Name: "Millenium Falcon", length: 34.37},
		{Name: "X-Wing", length: 12.5},
		{Name: "Tie Advanced x1", length: 9.2},
		{Name: "Imperial Shuttle", length: 20},
	}
)

func init() {
	// Set up friendships
	luke := &humans[0].Character
	solo := &humans[2].Character
	leia := &humans[3].Character
	chew := &humans[5].Character
	c3po := &droids[0].Character
	r2d2 := &droids[1].Character

	humans[0].Friends = []*Character{leia, solo, chew, c3po, r2d2}
	humans[2].Friends = []*Character{chew, leia, luke}
	humans[3].Friends = []*Character{luke, solo, r2d2, c3po}
	humans[5].Friends = []*Character{solo, luke}

	droids[0].Friends = []*Character{r2d2, luke, leia, chew}
	droids[1].Friends = []*Character{c3po, luke, leia}

	// Set up human closures
	for i := range humans {
		humans[i].SecretBackstory = getSecretBackstory // assign function to the closure
		humans[i].Height = (&humans[i]).getHeight      // assign method to allow access to height field
		humans[i].FriendsConnection = (&humans[i]).Character.getFriendsConnection
	}
	// Set up droid closures
	for i := range droids {
		droids[i].SecretBackstory = getSecretBackstory
		droids[i].FriendsConnection = (&droids[i]).Character.getFriendsConnection
	}

	// Set up appearances
	humans[0].Appears = []int{0, 1, 2}
	humans[1].Appears = []int{0, 1, 2}
	humans[2].Appears = []int{0, 1, 2}
	humans[3].Appears = []int{0, 1, 2}
	humans[4].Appears = []int{0}
	humans[5].Appears = []int{0, 1, 2}
	droids[0].Appears = []int{0, 1, 2}
	droids[1].Appears = []int{0, 1, 2}

	// Set up star ship piloting
	millenium := &starShips[0]
	xwing := &starShips[1]
	tie := &starShips[2]
	shuttle := &starShips[3]

	humans[0].StarShips = []*StarShip{xwing, shuttle}
	humans[1].StarShips = []*StarShip{tie}
	humans[2].StarShips = []*StarShip{millenium, shuttle}
	humans[5].StarShips = []*StarShip{millenium}

	// Set up star ship Length closures
	for i := range starShips {
		starShips[i].Length = (&starShips[i]).getLength
	}
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
				if ID >= 0 && ID < len(humans) {
					return humans[ID], nil
				}
				return nil, fmt.Errorf("internal error: no character with ID %d in episode %d", ID, episode)
			},
			Human: func(ID int) (*Human, error) {
				ID -= FirstHumanID
				if ID < 0 || ID >= len(humans) {
					return nil, fmt.Errorf("Human %d not found", FirstHumanID+ID)
				}
				return &humans[ID], nil
			},
			Droid: func(ID int) (*Droid, error) {
				ID -= FirstDroidID
				if ID < 0 || ID >= len(droids) {
					return nil, fmt.Errorf("Droid %d not found", ID)
				}
				return &droids[ID], nil
			},
			StarShip: func(ID int) (*StarShip, error) {
				ID -= FirstStarShipID
				if ID < 0 || ID >= len(starShips) {
					return nil, fmt.Errorf("Star ship %d not found", ID)
				}
				return &starShips[ID], nil
			},
			Reviews: func(episode int) ([]Review, error) {
				if episode < 0 || episode >= len(episodes) {
					return nil, fmt.Errorf("episode %d not found", episode)
				}
				var r []Review
				for i := range episodes[episode].Stars {
					r = append(r, Review{Stars: episodes[episode].Stars[i], Commentary: episodes[episode].Commentary[i]})
				}
				return r, nil
			},
		},
		Mutation{
			CreateReview: func(episode int, review ReviewInput) (*EpisodeDetails, error) {
				if episode < 0 || episode >= len(episodes) {
					return nil, fmt.Errorf("episode %d not found", episode)
				}
				if review.Stars < 0 || review.Stars > 5 {
					return nil, fmt.Errorf("review stars %d out of range", review.Stars)
				}
				episodes[episode].Stars = append(episodes[episode].Stars, review.Stars)
				episodes[episode].Commentary = append(episodes[episode].Commentary, review.Commentary)
				return &episodes[episode], nil
			},
		},
	)
	handler = http.TimeoutHandler(handler, 15*time.Second, `{"errors":[{"message":"timeout"}]}`)
	http.Handle("/graphql", handler)
	http.ListenAndServe(":8080", nil)
}

func getSecretBackstory() (string, error) { return "", errors.New("secretBackstory is secret.") }

const feetPerMeter = 3.28084

// getHeight returns the height of a human
// Parameters
//  h (receiver) is a pointer to the Human
//  unit is the unit for the return value (FOOT or METER)
func (h *Human) getHeight(unit int) (float64, error) {
	switch unit {
	case METER:
		return h.height, nil
	case FOOT:
		return h.height * feetPerMeter, nil
	default:
		return 0, fmt.Errorf("Human.height: unknown LengthUnit value: %d", unit)
	}
}

// getLength returns the length of a StarShip
// Parameters
//  ss (receiver) is a pointer to the StarShip
//  unit is the unit for the return value (FOOT or METER)
func (ss *StarShip) getLength(unit int) (float64, error) {
	switch unit {
	case METER:
		return ss.length, nil
	case FOOT:
		return ss.length * feetPerMeter, nil
	default:
		return 0, fmt.Errorf("StarShip.length: unknown LengthUnit value: %d", unit)
	}
}

func (c *Character) getFriendsConnection(first int, after string) (FriendsConnection, error) {
	r := FriendsConnection{
		TotalCount: len(c.Friends),
		Edges:      make([]FriendsEdge, 0),
		Friends:    make([]*Character, 0),
	}

	beg := -1

	// Find start index based on 'after' parameter
	if after == "" {
		beg = 0 // if 'after' not given start from the beginning
	} else {
		for i, friend := range c.Friends {
			if base64.StdEncoding.EncodeToString([]byte(friend.Name)) == after {
				beg = i + 1 // start after it
				break
			}
		}
	}

	// If 'after' was valid then get the 'first' friends after
	if beg > -1 {
		// Find (one past) last index based on 'first' parameter
		end := len(c.Friends)
		if first > -1 {
			end = beg + first
			if end > len(c.Friends) {
				end = len(c.Friends)
			}
		}

		// Get the friends in the range
		for i := beg; i < end; i++ {
			r.Edges = append(r.Edges, FriendsEdge{
				Cursor: base64.StdEncoding.EncodeToString([]byte(c.Friends[i].Name)),
				Node:   c.Friends[i],
			})
			r.Friends = append(r.Friends, c.Friends[i])
		}

		// Update paging info
		if beg < len(c.Friends) {
			s1 := base64.StdEncoding.EncodeToString([]byte(c.Friends[beg].Name))
			r.PageInfo.StartCursor = &s1
			if end >= beg {
				s2 := base64.StdEncoding.EncodeToString([]byte(c.Friends[end-1].Name))
				r.PageInfo.EndCursor = &s2
			}
		}
		r.PageInfo.HasNextPage = end < len(c.Friends)
	}

	return r, nil
}
