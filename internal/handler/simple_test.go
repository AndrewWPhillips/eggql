package handler_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/andrewwphillips/eggql/internal/handler"
)

func TestSimple(t *testing.T) {
	// Create handler that has a single GraphQL query called "hello" which returns a string (world)
	h := handler.New(
		[]string{"type Query { hello: String! }"},
		nil,
		[3][]interface{}{
			{struct{ Hello string }{"world"}},
			nil,
			nil,
		},
	)

	// Create a HTTP request that invokes the GraphQL "hello" query
	request := httptest.NewRequest("POST", "/",
		strings.NewReader(`{ "Query": "{ hello }" }`))
	request.Header.Add("Content-Type", "application/json")

	// Invoke the handler, recording the response
	writer := httptest.NewRecorder()
	h.ServeHTTP(writer, request)

	// Check the results
	if writer.Result().StatusCode != http.StatusOK {
		t.Fatalf("Unexpected response code %d", writer.Code)
	}
	var rv struct {
		Data *struct {
			Hello string
		}
		Errors []struct {
			Message    string
			Path       []interface{}
			Locations  []struct{ Line, Column int }
			Extensions map[string]interface{}
			Rule       string
		}
	}
	json.Unmarshal(writer.Body.Bytes(), &rv)
	if rv.Errors != nil {
		t.Fatalf("Got unexpected error(s) - first Error: %q", rv.Errors[0].Message)
	}
	if rv.Data == nil {
		t.Fatalf("No data returned from the query")
	}
	if rv.Data.Hello != "world" {
		t.Fatalf("Got unexpected result %q", rv.Data.Hello)
	}
}

// TestMultSchema tests using different schemas together
func TestMultSchema(t *testing.T) {
	type Greeting struct {
		Hello string
	}
	// Create handler that has a single GraphQL query called "hello" which returns a string (world)
	h := handler.New(
		[]string{
			"extend schema { query: Query } extend type Query { hi: String! }",
			"extend schema { query: Query } extend type Query { greeting: Greeting! } type Greeting { hello: String! }",
		},
		nil,
		[3][]interface{}{
			{
				struct{ Hi string }{"there"},
				struct{ Greeting Greeting }{Greeting{"world"}},
			},
			nil,
			nil,
		},
	)

	// Create HTTP request that invokes the GraphQL query
	request := httptest.NewRequest("POST", "/",
		strings.NewReader(`{ "Query": "{ hi greeting { hello } }" }`))
	request.Header.Add("Content-Type", "application/json")

	// Invoke the handler, recording the response
	writer := httptest.NewRecorder()
	h.ServeHTTP(writer, request)

	// Check the results
	if writer.Result().StatusCode != http.StatusOK {
		t.Fatalf("Unexpected response code %d", writer.Code)
	}
	var rv struct {
		Data *struct {
			Hi       string
			Greeting struct {
				Hello string
			}
		}
		Errors []struct {
			Message    string
			Path       []interface{}
			Locations  []struct{ Line, Column int }
			Extensions map[string]interface{}
			Rule       string
		}
	}
	json.Unmarshal(writer.Body.Bytes(), &rv)
	if rv.Errors != nil {
		t.Fatalf("Got unexpected error(s) - first Error: %q", rv.Errors[0].Message)
	}
	if rv.Data == nil {
		t.Fatalf("No data returned from the query")
	}
	if rv.Data.Hi != "there" {
		t.Fatalf("Got unexpected result %q", rv.Data.Hi)
	}
	if rv.Data.Greeting.Hello != "world" {
		t.Fatalf("Got unexpected result %q", rv.Data.Greeting.Hello)
	}
}
