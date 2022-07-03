package main

import (
	"context"
	"log"
	"net/http"
	"time"

	"github.com/andrewwphillips/eggql"
)

const (
	address = "localhost:8080"
	path    = "/graphql"
)

type (
	Query struct {
		Feed map[eggql.ID]Link
	}

	Mutation struct {
		Post   func(context.Context, string, string) (Link, error) `egg:"post(url,description)"`
		Signup func(string, string, string) (AuthPayload, error)   `egg:"signup(email,password,name)"`
		Login  func(string, string) (AuthPayload, error)           `egg:"login(email,password)"`
	}
)

func main() {
	handler := eggql.MustRun(
		Query{
			Feed: links,
		},
		Mutation{
			Post:   Post,
			Signup: Signup,
			Login:  Login,
		},
	)

	handler = http.TimeoutHandler(handler, 15*time.Second, `{"errors":[{"message":"timeout"}]}`)
	handler = &authHandler{inner: handler}
	http.Handle(path, handler)

	log.Println("starting server on: http://", address+path)
	http.ListenAndServe(address, nil)
	log.Println("stopping server")
}

/*
// UniqueID returns a unique ID (with a fixed prefix) for the given map.
func UniqueID[T any](m map[eggql.ID]T, prefix string) eggql.ID {
	var ID eggql.ID

	for {
		ID = eggql.ID(prefix + strconv.Itoa(rand.Int()))
		if _, ok := m[ID]; !ok {
			return ID
		}
	}
}
*/
