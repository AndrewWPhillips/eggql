package main

import (
	"context"
	"errors"

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

	ID := UniqueID(links, "") // generate a new link ID
	links[ID] = Link{
		ID:          ID,
		Description: description,
		URL:         url,
		PostedBy:    user,
	}
	return links[ID], nil
}
