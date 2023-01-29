package handler_test

import (
	"encoding/json"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/andrewwphillips/eggql/internal/handler"
)

// TestFuncCache tests simple resolver func caching
func TestFuncCache(t *testing.T) {
	// next is a value that is incremented between resolver calls so that a resolver returns different results
	var next int32

	// getNext increments and returns the value of the above next variable. We use atomic operations to avoid a
	// data race, since resolvers execute concurrently (though we use the handler.NoConcurrency() option below).
	getNext := func() int { return int(atomic.AddInt32(&next, 1)) }

	// Note that schemaString and queryData must be consistent (usually handled by schema package)
	const schemaString = "type Query { i: Int! j: Int! k: Int! }"
	queryData := struct {
		I func() int // cached (see handler.CacheOn() option below)
		J func() int `egg:",no_cache"`
		K func() int // cached
	}{
		I: getNext,
		J: getNext,
		K: getNext,
	}

	data := map[string]struct {
		query    string      // GraphQL query to send to the handler (query syntax)
		expected interface{} // expected result after decoding the returned JSON
	}{
		"RepeatCached": {
			query:    "{ i repeat:i }",
			expected: JsonObject{"i": 1.0, "repeat": 1.0},
		},
		"RepeatNotCached": {
			query:    "{ j repeat:j }",
			expected: JsonObject{"j": 1.0, "repeat": 2.0},
		},
		"DifferentField": {
			// ensure i and k are independent as they are different fields (but using same func)
			query:    "{ i k }",
			expected: JsonObject{"i": 1.0, "k": 2.0},
		},
		"AllField": {
			query:    "{ i j k }",
			expected: JsonObject{"i": 1.0, "j": 2.0, "k": 3.0},
		},
		"JThenRepeat": {
			query:    "{ j i repeat:i }",
			expected: JsonObject{"j": 1.0, "i": 2.0, "repeat": 2.0},
		},
		"RepeatThenJ": {
			query:    "{ i repeat:i j }",
			expected: JsonObject{"i": 1.0, "repeat": 1.0, "j": 2.0},
		},
		"RepeatNoAlias": {
			// Surprisingly querying the same field twice without an alias is OK, but just gives one result
			query:    "{ i i }",
			expected: JsonObject{"i": 1.0},
		},
	}

	for name, testData := range data {
		t.Run(name, func(t *testing.T) {
			next = 0 // reset counter
			h := handler.New([]string{schemaString},
				nil,
				[3][]interface{}{
					{queryData},
					nil,
					nil,
				},
				handler.FuncCache(true),     // turn on function result cachi
				handler.NoConcurrency(true), // simplifies tests (concurrent execution can affect order)
			)

			// Make the request body and the HTTP request that uses it
			body := strings.Builder{}
			body.WriteString(`{"query":"`)
			body.WriteString(testData.query)
			body.WriteString(`"`)
			body.WriteString(`}`)

			request := httptest.NewRequest("POST", "/", strings.NewReader(body.String()))
			request.Header.Add("Content-Type", "application/json")

			// Invoke the handler, recording the response
			writer := httptest.NewRecorder()
			h.ServeHTTP(writer, request) /*****/

			// All of these tests should give status OK
			if writer.Result().StatusCode != http.StatusOK {
				t.Logf("Unexpected response code %d", writer.Code)
				return
			}

			// Decode the JSON response
			var result struct {
				Data   interface{}
				Errors []struct{ Message string }
			}
			decoder := json.NewDecoder(writer.Body)
			if err := decoder.Decode(&result); err != nil {
				t.Logf("Error decoding JSON: %v", err)
				return
			}

			// Check that the resulting GraphQL result (error and data)
			Assertf(t, result.Errors == nil, "Expected no error and got %v", result.Errors)
			Assertf(t, reflect.DeepEqual(result.Data, testData.expected), "Expected %v, got %v", testData.expected, result.Data)
		})
	}
}

type Object struct {
	Name func() string
	name string
}

// getName returns a different value each time it's called so we can check if the value is cached
func (obj Object) getName() string {
	return obj.name + strconv.Itoa(int(rand.Uint64()))
}

// TestObjectCache tests that an object (struct) is cached properly
func TestObjectCache(t *testing.T) {
	a := Object{name: "A"}
	a.Name = a.getName
	b := Object{name: "B"}
	b.Name = b.getName

	const queryString = "type Query { a: Object! a2: Object! b: Object! } type Object { name: String! }"
	queryData := struct {
		A  func() *Object
		A2 func() *Object
		B  func() *Object
	}{
		A:  func() *Object { return &a },
		A2: func() *Object { return &a },
		B:  func() *Object { return &b },
	}

	// Each query string must have 2 queries (x and y) that return an Object (from which we get the name)
	data := map[string]struct {
		query   string // GraphQL query to send to the handler (query syntax)
		noCache bool
		same    bool
	}{
		"SameObjectSameFunc": {
			query: "{ x:a {name} y:a {name} }",
			same:  true,
		},
		"SameNoCache": {
			query:   "{ x:a {name} y:a {name} }",
			noCache: true,
			same:    false, // values should be different if not cached
		},
		"SameObjectDiffFunc": {
			query: "{ x:a {name} y:a2 {name} }",
			// TODO: decide if the same object returned through a different func be cached?
			same: false,
		},
		"DiffObject": {
			query: "{ x:a {name} y:b {name} }",
			same:  false,
		},
	}

	for name, testData := range data {
		t.Run(name, func(t *testing.T) {
			h := handler.New([]string{queryString},
				nil,
				[3][]interface{}{
					{queryData},
					nil,
					nil,
				},
				handler.FuncCache(!testData.noCache),
				handler.NoConcurrency(true), // simplifies tests (concurrent execution can affect order)
			)

			// Make the request body and the HTTP request that uses it
			body := strings.Builder{}
			body.WriteString(`{"query":"`)
			body.WriteString(testData.query)
			body.WriteString(`"`)
			body.WriteString(`}`)

			request := httptest.NewRequest("POST", "/", strings.NewReader(body.String()))
			request.Header.Add("Content-Type", "application/json")

			// Invoke the handler, recording the response
			writer := httptest.NewRecorder()
			h.ServeHTTP(writer, request) /*****/

			// All of these tests should give status OK
			if writer.Result().StatusCode != http.StatusOK {
				t.Logf("Unexpected response code %d", writer.Code)
				return
			}

			// Decode the JSON response
			var result struct {
				Data   interface{}
				Errors []struct{ Message string }
			}
			decoder := json.NewDecoder(writer.Body)
			if err := decoder.Decode(&result); err != nil {
				t.Logf("Error decoding JSON: %v", err)
				return
			}

			// Check that the resulting GraphQL result (error and data)
			Assertf(t, result.Errors == nil, "Expected no error and got %v", result.Errors)
			m, ok := result.Data.(JsonObject)
			if !ok {
				t.FailNow()
			}
			if testData.same {
				Assertf(t, reflect.DeepEqual(m["x"], m["y"]), "Expected x and y to be the same (cached) %v", result.Data)
			} else {
				Assertf(t, !reflect.DeepEqual(m["x"], m["y"]), "Expected x and y to be different %v", result.Data)
			}
		})
	}
}
