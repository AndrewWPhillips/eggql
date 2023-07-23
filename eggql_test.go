package eggql_test

// End-to-end tests (also see low-level tests in the field, schema and handler packages)

import (
	"encoding/json"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/andrewwphillips/eggql"
)

type (
	// JsonObject is what json.Unmarshaler produces when it decodes a JSON object.  Note that we use a type alias here,
	// hence the equals sign (=), rather than a type definition - otherwise reflect.DeepEqual does not work.
	JsonObject = map[string]interface{}

	Person struct {
		Name string
		Age  int
	}
)

// TestQuery performs high-level (end to end) tests of GraphQL queries.  More thorough low-level tests are included
// in the internal packages (field, schema, and handler).
func TestQuery(t *testing.T) {
	tests := map[string]struct {
		q         interface{} // must be a struct with exported field(s)
		query     string      // main part of request body (GraphQl query format)
		variables string      // if not empty: added to request body (JSON key/value pairs)
		expected  interface{} // encoded JSON
	}{
		"hello": {
			q:        struct{ Message string }{"hello"},
			query:    "{ message }",
			expected: JsonObject{"message": "hello"},
		},
		"slice": {
			// get list of objects from slice
			q:     struct{ Friends []Person }{[]Person{{"Al", 21}, {"Bob", 22}}},
			query: "{ friends { name age } }",
			expected: JsonObject{
				"friends": []interface{}{
					JsonObject{"name": "Al", "age": 21.0},
					JsonObject{"name": "Bob", "age": 22.0},
				},
			},
		},
		"slice_field_id": {
			// get list of objects, incl. fake "id" field (field_id option)
			q: struct {
				Friends []Person `egg:",field_id,base=500"`
			}{[]Person{{"Al", 21}, {"Bob", 22}}},
			query: "{ friends { id name age } }",
			expected: JsonObject{
				"friends": []interface{}{
					JsonObject{"id": 500.0, "name": "Al", "age": 21.0},
					JsonObject{"id": 501.0, "name": "Bob", "age": 22.0},
				},
			},
		},
		"slice_element": {
			q: struct {
				Friend []Person `egg:",subscript,base=600"`
			}{[]Person{{"Al", 21}, {"Bob", 22}, {"Cam", 74}}},
			query:    `{ friend(id: 602) { id name age } }`,
			expected: JsonObject{"friend": JsonObject{"id": 602.0, "name": "Cam", "age": 74.0}},
		},
		"map": {
			// get list of objects using a map, incl. fake "id" field (field_id option)
			// Note: Originally this succeeded but occasionally failed due to random iteration of Friends map
			// So we run it with "-test.count=10" to be fairly sure we will see the error.
			q: struct {
				Friends map[string]Person `egg:",field_id"`
			}{
				Friends: map[string]Person{
					"a": {"Al", 21},
					"b": {"Bob", 22},
				},
			},
			query: "{ friends { id name age } }",
			expected: JsonObject{
				"friends": []interface{}{
					JsonObject{"id": "a", "name": "Al", "age": 21.0},
					JsonObject{"id": "b", "name": "Bob", "age": 22.0},
				},
			},
		},
		"map_element": {
			// get one element of a map (subscript option)
			q: struct {
				Friend map[int]Person `egg:",subscript"`
			}{
				Friend: map[int]Person{
					0: {"Al", 21},
					1: {"Bob", 22},
				},
			},
			query:    `{ friend(id: 1) { id name age } }`,
			expected: JsonObject{"friend": JsonObject{"id": 1.0, "name": "Bob", "age": 22.0}},
		},
		"map_string_key": {
			// get one element of a map with string key
			q: struct {
				Friend map[string]Person `egg:",subscript"`
			}{
				Friend: map[string]Person{
					"a": {"Al", 21},
					"b": {"Bob", 22},
				},
			},
			query:    `{ friend(id: \"b\") { id name age } }`,
			expected: JsonObject{"friend": JsonObject{"id": "b", "name": "Bob", "age": 22.0}},
		},
		"map_variable_arg": {
			q: struct {
				Friend map[string]Person `egg:",subscript"`
			}{
				Friend: map[string]Person{
					"a": {"Al", 21},
					"b": {"Bob", 22},
				},
			},
			query:     `query ($id: String!) { friend(id: $id) { id name age } }`,
			variables: `{ "id": "a" }`,
			expected:  JsonObject{"friend": JsonObject{"id": "a", "name": "Al", "age": 21.0}},
		},
		"map_of_pointers": {
			q: struct {
				Friend map[string]*Person `egg:",subscript"`
			}{
				Friend: map[string]*Person{
					"a": {"Al", 21},
					"b": {"Bob", 22},
				},
			},
			query:    `{ friend(id: \"b\") { id name } }`,
			expected: JsonObject{"friend": JsonObject{"id": "b", "name": "Bob"}},
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			name, test := name, test // capture loop variables
			//t.Parallel()

			// Create a test server
			server := httptest.NewTLSServer(eggql.MustRun(test.q))
			defer server.Close()

			// build and POST the query
			inBody := `{ "query": "` + test.query + `"`
			if test.variables != "" {
				inBody += `, "variables": ` + test.variables
			}
			inBody += ` }`
			resp, err := server.Client().Post(server.URL, "application/json", strings.NewReader(inBody))
			if err != nil {
				t.Logf("Error POSTing the query: %v", err)
				return
			}
			defer resp.Body.Close()

			// decode the response
			var result struct {
				Data interface {
				}
				Errors []struct {
					Message string
				}
			}
			//json.Unmarshal(writer.Body.Bytes(), &result)
			decoder := json.NewDecoder(resp.Body)
			if err := decoder.Decode(&result); err != nil {
				t.Logf("Error decoding JSON: %v", err)
				return
			}

			// Check that the resulting GraphQL result (error and data)
			Assertf(t, result.Errors == nil, "%-12s: expected no error and got %v", name, result.Errors)
			Assertf(t, reflect.DeepEqual(result.Data, test.expected), "%-12s: expected %v, got %v", name, test.expected, result.Data)
		})
	}
}

// Assertf displays a tick or cross depending on the success of the test (succeeded)
// It also displays a nicely formated message if the test failed, and also displays the message for successful tests if
// all results are displayed (-v testing option) OR any other test run at the same time fails
func Assertf(t *testing.T, succeeded bool, format string, args ...interface{}) {
	const (
		succeed = "\u2713" // tick
		failed  = "XXXXX"  //"\u2717" // cross
	)

	t.Helper()
	if !succeeded {
		t.Errorf("%-6s"+format, append([]interface{}{failed}, args...)...)
	} else {
		t.Logf("%-6s"+format, append([]interface{}{succeed}, args...)...)
	}
}
