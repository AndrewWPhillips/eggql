package main

import (
	"context"
	"errors"
	"math/rand"
	"strconv"

	"github.com/andrewwphillips/eggql"
)

type (
	Link struct {
		ID          eggql.ID `egg:"id"`
		Description string
		URL         string `egg:"url"`
		PostedBy    User
	}
)

var links = map[eggql.ID]Link{}

// Post creates a new link for the currently logged-in user
func Post(ctx context.Context, url, description string) (Link, error) {
	userID, ok := ctx.Value("user").(string)
	if !ok || userID == "" {
		return Link{}, errors.New("you must be logged in to post")
	}
	user, ok := users[eggql.ID(userID)]
	if !ok {
		return Link{}, errors.New("unknown user:" + userID)
	}

	ID := UniqueLinkID(links, "") // generate a new link ID
	links[ID] = Link{
		ID:          ID,
		Description: description,
		URL:         url,
		PostedBy:    user,
	}
	return links[ID], nil
}

// UniqueID returns a unique ID (with a fixed prefix) for the given map.
func UniqueLinkID(m map[eggql.ID]Link, prefix string) eggql.ID {
	var ID eggql.ID

	for {
		ID = eggql.ID(prefix + strconv.Itoa(rand.Int()))
		if _, ok := m[ID]; !ok {
			return ID
		}
	}
}
