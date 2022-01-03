package handler_test

import (
	"encoding/json"
	"github.com/andrewwphillips/eggql/internal/handler"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSimple(t *testing.T) {
	// Create handler that has a single GraphQL query called "hello" which returns a string (world)
	h := handler.New(
		"schema {query: Query}"+
			"type Query { hello: String! }",
		struct{ Hello string }{"world"})

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
