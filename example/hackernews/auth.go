package main

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/andrewwphillips/eggql"
	"github.com/golang-jwt/jwt/v4"
)

const (
	APP_ISSUER = "github.com/andrewwphillips/eggql/example/hackernews"
	APP_SECRET = "GraphQL-is-awesome" // TODO get this from secret store

	userIDClaim = "jti"
	expiryClaim = "exp"
	issuerClaim = "iss"
)

type authHandler struct {
	inner http.Handler
}

// ServeHTTP gets the user ID from the JWT token in the HTTP Authorization Header
// and adds it to the request context so the handler can check that it's authorised.
func (h *authHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.inner.ServeHTTP(w, func(r *http.Request) *http.Request {
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" || !strings.HasPrefix(authHeader, "Bearer ") {
			return r // no auth hdr
		}
		token, err := jwt.Parse(authHeader[len("Bearer "):], func(token *jwt.Token) (interface{}, error) {
			if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, fmt.Errorf("unexpected signing method %v", token.Header["alg"])
			}
			return []byte(APP_SECRET), nil
		})
		if err != nil || !token.Valid {
			return r // token invalid
		}
		ID := token.Claims.(jwt.MapClaims)["jti"]
		if ID == nil {
			return r // no ID
		}
		return r.WithContext(context.WithValue(r.Context(), "user", ID))
	}(r))
}

// GetToken returns a JWT token for the given user ID.  This JWT indicates what user
// is logged in and can be used to authorise requests.
func GetToken(userID eggql.ID) (string, error) {
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		userIDClaim: string(userID),
		expiryClaim: time.Now().Add(time.Hour * 24).Unix(),
		issuerClaim: APP_ISSUER,
	})
	return token.SignedString([]byte(APP_SECRET))
}
