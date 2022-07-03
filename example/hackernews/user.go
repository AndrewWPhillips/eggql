package main

import (
	"errors"
	"math/rand"
	"strconv"

	"github.com/andrewwphillips/eggql"
	"golang.org/x/crypto/bcrypt"
)

type (
	User struct {
		ID       eggql.ID `egg:"id"`
		Name     string
		Email    string
		password string
	}

	AuthPayload struct {
		Token string
		User  User
	}
)

var users = map[eggql.ID]User{}

// Signup creates a new user.
func Signup(email, password, name string) (AuthPayload, error) {
	ID := UniqueUserID(users) // get a new ID for a new user
	tokenString, err := GetToken(ID)
	if err != nil {
		return AuthPayload{}, err
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return AuthPayload{}, err
	}

	users[ID] = User{
		ID:       ID,
		Name:     name,
		Email:    email,
		password: string(hash),
	}
	return AuthPayload{Token: tokenString, User: users[ID]}, nil
}

// Login authenticates a user.
func Login(email, password string) (AuthPayload, error) {
	for ID, user := range users {
		if user.Email == email {
			if err := bcrypt.CompareHashAndPassword([]byte(user.password), []byte(password)); err == nil {
				tokenString, err := GetToken(ID)
				if err != nil {
					return AuthPayload{}, err
				}
				return AuthPayload{Token: tokenString, User: user}, nil
			}
			// break - don't break in case of multiple logins with the same email addr.
		}
	}
	return AuthPayload{}, errors.New("invalid email or password")
}

// UniqueUserID returns a unique user ID (with a "U" prefix) for the given map.
func UniqueUserID(m map[eggql.ID]User) eggql.ID {
	var ID eggql.ID

	for {
		ID = eggql.ID("U" + strconv.Itoa(rand.Int()))
		if _, ok := m[ID]; !ok {
			return ID
		}
	}
}
