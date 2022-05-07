package main

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/andrewwphillips/eggql"
)

// ReviewTime implements a GraphQL custom scalar used to keep track of when a movie review was posted
type ReviewTime struct{ time.Time } // embed Time so we get String() method (for marshaling)

// UnmarshalEGGQL is called when eggql needs to decode a string to a Time
// The existence of this method signals that this type is a custom scalar/
func (rt *ReviewTime) UnmarshalEGGQL(in string) error {
	tmp, err := time.Parse(time.RFC3339, in)
	if err != nil {
		return fmt.Errorf("%w error in UnmarshalEGGQL for custom scalar Time", err)
	}
	rt.Time = tmp
	return nil
}

const (
	FirstHumanID    = 1000
	FirstDroidID    = 2000
	FirstStarshipID = 3000
)

type (
	Query struct {
		// Attaching an "egg:" key to the tag of a field named "_" here adds a description to the GraphQL "Query" type
		_ eggql.TagHolder `egg:"# The root query object stores all the queries that can be made"`

		// We need to reference the `Character` struct so that eggql can find it (using reflection) and can generate
		// the GraphQL Character "interface" for the schema.  This is necessary as Hero() has to return a Go interface
		// in order to return anything that implements the Character GraphQL interface (ie, Human or Droid objects).
		// NOTE: unnamed fields (ie, with name "_") cannot be used except with reflection and since this one is the
		// 1st field of the struct and is declared as a zero-length array it uses no memory.
		_ [0]Character

		// Hero is a function used to implement the GraphQL resolver: "hero(episode: Episode = JEDI): Character", where:
		//   hero = the resolver name taken from the 1st tag option (but could have been deduced from the field name "Hero")
		//   episode = the name of the resolver argument (can't be deduced from the func parameter name as Go reflection only includes types, not names, of parameters)
		//   Episode = type of the resolver argument - in GraphQL an enum (see gqlEnums below) but the Go func parameter must be an integer type (int, int8, etc)
		//   JEDI = default value of the argument - must be one of the strings in gqlEnums["Episode"] below
		//   Character = the resolver return type (taken from the 1st tag option after the colon)
		//    - this can't be deduced from the func return type which must be an interface{} when implementing a GraphQL interface
		Hero func(episode int) (interface{}, error) `egg:"hero:Character,args(episode:Episode=JEDI)"`

		// Human resolves one human given their id: "human(id: Int!): Human"
		Human []Human `egg:",subscript,base=1000"` // base = FirstHumanID

		// Humans resolves a list of all humans: "humans: [Human!]"
		Humans []Human `egg:",field_id,base=1000,nullable"` // base = FirstHumanID

		// Droid resolves a droid given their id: "droid(id: Int!): Droid"
		Droid []Droid `egg:",subscript,base=2000"` // base = FirstDroidID

		// Droids returns a list of all droids: "Droids: [Droid!]"
		Droids []Droid `egg:",field_id,base=2000,nullable"` // base = FirstDroidID

		// Starship resolves a starship given it's id: "starship(id: Int!): Starship"
		Starship []Starship `egg:",subscript,base=3000"`

		// Starships returns a list of all ships: "starships: [Starship!]"
		Starships []Starship `egg:",field_id,base=3000,nullable"` // base = FirstStarshipID

		// Reviews is a function used to implement the GraphQl resolver: "reviews(episode: Episode): [Review]"
		//  reviews = resolver name, deduced from the field name "Reviews"
		//  episode = argument name (from 1st value of "args" option before the colon)
		//  Episode = argument type (from 1st value of "args" option after the colon)
		//  [Review] = return type is a list of Review, deduced from the fact that the func returns a slice ([]Review)
		Reviews func(int) ([]Review, error) `egg:",args(episode:Episode)"`

		// Search implements the resolver: "search(text: String!): [SearchResult]"
		Search func(context.Context, string) ([]interface{}, error) `egg:":[SearchResult],args(text)"`
	}
	SearchResult struct { // SearchResult has no exported fields so represents a Union of all types in which it is embedded
		_ eggql.TagHolder `egg:"# Union that defines which object types are searchable"`
	}
	Character struct {
		_                 eggql.TagHolder `egg:"# Represents a character (human or droid) in the Star Wars trilogy"`
		Name              string          `egg:"# Name of the character"`
		Friends           []*Character
		FriendsConnection func(first int, after string) FriendsConnection `egg:",args(first=-1, after=\"\")"`
		Appears           []int                                           `egg:"appearsIn:[Episode]"`
		SecretBackstory   func() (string, error)
	}
	Human struct {
		_            eggql.TagHolder            `egg:"# An intelligent humanoid creature from Star Wars"`
		SearchResult                            // Human is part of the SearchResult union so can be returned from a search query
		Character                               // Human implements the Character interface
		Height       func(int) (float64, error) `egg:",args(unit:LengthUnit=METER)"`
		height       float64                    // meters
		HomePlanet   string
		Starships    []*Starship `egg:",nullable"`
	}
	Droid struct {
		_               eggql.TagHolder `egg:"# A mobile, semi-autonomous machine from Star Wars"`
		SearchResult                    // Droid is part of the SearchResult union so can be returned from a search query
		Character                       // Droid implements the Character interface
		PrimaryFunction string
	}
	EpisodeDetails struct {
		_      eggql.TagHolder `egg:"# Stores info and reviews of each of the movies"`
		Name   string
		HeroId int
		// The following are submitted reviews (with stars and time)
		Stars      []int
		Commentary []string
		Time       []ReviewTime
	}
	Review struct {
		_          eggql.TagHolder `egg:"# One person's rating and review for a movie"`
		Stars      int
		Commentary string
		Time       ReviewTime
	}
	Starship struct {
		_            eggql.TagHolder `egg:"# Machines for inter-planetary and inter-stellar travel"`
		SearchResult                 // Starship is part of the SearchResult union so can be returned from a search query
		Name         string
		Length       func(int) (float64, error) `egg:",args(unit:LengthUnit=METER)"`
		length       float64                    // meters
	}

	// Movie reviews
	Mutation struct {
		_            eggql.TagHolder                                 `egg:"# Represents all the updates that can be made to the data"`
		CreateReview func(int, ReviewInput) (*EpisodeDetails, error) `egg:",args(episode:Episode,review)"`
	}
	ReviewInput struct {
		_          eggql.TagHolder `egg:"# The input object sent when someone is creating a new review"`
		Stars      int
		Commentary string
		Time       *ReviewTime `egg:"# time the review was written - current time is used if NULL"`
	}

	// The following are for pagination of a list of friends
	FriendsConnection struct {
		_          eggql.TagHolder `egg:"# A connection object for a character's friends"`
		TotalCount int             `egg:"# The total number of friends"`
		Edges      []FriendsEdge   `egg:"# Edges for each of the character's friends"`
		Friends    []*Character    `egg:"# A list of the friends, as a convenience when edges are not needed"`
		PageInfo   PageInfo        `egg:"# Information for paginating this connection"`
	}
	FriendsEdge struct {
		_      eggql.TagHolder `egg:"# An edge object for a character's friends"`
		Cursor string
		Node   *Character
	}
	PageInfo struct {
		_           eggql.TagHolder `egg:"# Information for paginating this connection"`
		StartCursor *string
		EndCursor   *string
		HasNextPage bool
	}
)

var (
	gqlEnums = map[string][]string{
		"Episode# Movies of the Star Wars trilogy": {
			// order should match the episodes slice below
			"NEWHOPE# A New Hope (1977)",
			"EMPIRE# The Empire Strikes Back (1980)",
			"JEDI# Return of the Jedi (1983)",
		},
		"LengthUnit# Units for spatial measurements": {
			// order of strings in the slice should match METER, etc consts below
			"METER# Standard metric spatial unit",
			"FOOT# Imperial spatial unit used mainly in the US",
		},
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
	starships = []Starship{
		{Name: "Millenium Falcon", length: 34.37},
		{Name: "X-Wing", length: 12.5},
		{Name: "Tie Advanced x1", length: 9.2},
		{Name: "Imperial Shuttle", length: 20},
	}
)

func init() {
	// Set up friendships
	luke := &humans[0].Character
	bad1 := &humans[1].Character
	solo := &humans[2].Character
	leia := &humans[3].Character
	bad2 := &humans[4].Character
	chew := &humans[5].Character
	c3po := &droids[0].Character
	artu := &droids[1].Character

	humans[0].Friends = []*Character{leia, solo, chew, c3po, artu}
	humans[1].Friends = []*Character{bad1}
	humans[2].Friends = []*Character{chew, leia, luke}
	humans[3].Friends = []*Character{luke, solo, artu, c3po}
	humans[4].Friends = []*Character{bad2}
	humans[5].Friends = []*Character{solo, luke}
	droids[0].Friends = []*Character{artu, luke, leia, chew}
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
	millenium := &starships[0]
	xwing := &starships[1]
	tie := &starships[2]
	shuttle := &starships[3]

	humans[0].Starships = []*Starship{xwing, shuttle}
	humans[1].Starships = []*Starship{tie}
	humans[2].Starships = []*Starship{millenium, shuttle}
	humans[5].Starships = []*Starship{millenium}

	// Set up star ship Length closures
	for i := range starships {
		starships[i].Length = (&starships[i]).getLength
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

			Human:     humans, // get one (subscript)
			Humans:    humans, // get list
			Droid:     droids,
			Droids:    droids,
			Starship:  starships,
			Starships: starships,

			Reviews: func(episode int) ([]Review, error) {
				if episode < 0 || episode >= len(episodes) {
					return nil, fmt.Errorf("episode %d not found", episode)
				}
				var r []Review
				for i := range episodes[episode].Stars {
					r = append(r, Review{
						Stars:      episodes[episode].Stars[i],
						Commentary: episodes[episode].Commentary[i],
						Time:       episodes[episode].Time[i],
					})
				}
				return r, nil
			},
			Search: func(ctx context.Context, text string) (r []interface{}, err error) {
				toFind := strings.ToLower(text)
				for _, h := range humans {
					if e := ctx.Err(); e != nil {
						return nil, e
					}
					if strings.Contains(strings.ToLower(h.Name), toFind) {
						r = append(r, h)
					}
				}
				for _, d := range droids {
					if e := ctx.Err(); e != nil {
						return nil, e
					}
					if strings.Contains(strings.ToLower(d.Name), toFind) {
						r = append(r, d)
					}
				}
				for _, ss := range starships {
					if e := ctx.Err(); e != nil {
						return nil, e
					}
					if strings.Contains(strings.ToLower(ss.Name), toFind) {
						r = append(r, ss)
					}
				}
				return
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
				if review.Time == nil {
					episodes[episode].Time = append(episodes[episode].Time, ReviewTime{time.Now()})
					//episodes[episode].Time = append(episodes[episode].Time, ReviewTime(time.Now()))
				} else {
					episodes[episode].Time = append(episodes[episode].Time, *review.Time)
				}
				return &episodes[episode], nil
			},
		},
	)
	handler = http.TimeoutHandler(handler, 15*time.Hour, `{"errors":[{"message":"timeout"}]}`)
	http.Handle("/graphql", handler)

	log.Println("starting server")
	http.ListenAndServe(":8080", nil)
	log.Println("stopping server")
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

// getLength returns the length of a Starship
// Parameters
//  ss (receiver) is a pointer to the Starship
//  unit is the unit for the return value (FOOT or METER)
func (ss *Starship) getLength(unit int) (float64, error) {
	switch unit {
	case METER:
		return ss.length, nil
	case FOOT:
		return ss.length * feetPerMeter, nil
	default:
		return 0, fmt.Errorf("Starship.length: unknown LengthUnit value: %d", unit)
	}
}

// getFriendsConnection allows access to friends with recommended pagination model (see https://graphql.org/learn/pagination/)
// Note that to be compatible with the official Star Wars demo it does not return an error (eg if 'after' is not a valid
// "cursor") but returns empty edges and friends lists and null startCursor/endCursor.
// Parameters
//  c (receiver) is the character for which friends are wanted
//  first = max friends to return, -1 (default) means get all (ie from after 'after' till end of list)
//  after is the "cursor" indicating the 1st friend required
//  TODO: check defaults for 'first' and 'after' in Star Wars JS demo
func (c *Character) getFriendsConnection(first int, after string) FriendsConnection {
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
			if end > beg {
				s2 := base64.StdEncoding.EncodeToString([]byte(c.Friends[end-1].Name))
				r.PageInfo.EndCursor = &s2
			}
		}
		r.PageInfo.HasNextPage = end < len(c.Friends)
	}

	return r
}
