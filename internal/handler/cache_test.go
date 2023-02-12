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

			// Build the request body and the HTTP request that uses it
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

// TestCacheArgs checks that function arguments result in correct cache behaviour
// That is, assuming a "pure" function, calling it with the same args should give the same
// cached value, but different args should result in a different cached value.
func TestCacheArgs(t *testing.T) {
	var next int32
	// getNext increments next and returns the value multiplied by its parameter
	getNext := func(a int) int { return a * int(atomic.AddInt32(&next, 1)) }
	// getNext3 increments next and returns the value multiplied by all its parameters
	getNext3 := func(a, b, c int) int { return a * b * c * int(atomic.AddInt32(&next, 1)) }
	const schemaString = "type Query { y(a:Int!):Int! n(a:Int!):Int! yyy(a:Int!,b:Int!,c:Int!):Int! nnn(a:Int!,b:Int!,c:Int!):Int!}"
	queryData := struct {
		Y   func(int) int           `egg:"(a)"`
		N   func(int) int           `egg:"(a),no_cache"`
		Yyy func(int, int, int) int `egg:"(a,b,c)"`
		Nnn func(int, int, int) int `egg:"(a,b,c),no_cache"`
	}{
		Y:   getNext,
		N:   getNext,
		Yyy: getNext3,
		Nnn: getNext3,
	}

	data := map[string]struct {
		query    string      // GraphQL query to send to the handler (query syntax)
		expected interface{} // expected result after decoding the returned JSON
	}{
		"SameParam": {
			// same resolver called with same param should be cached
			query:    "{ y(a:3) y2:y(a:3) }",
			expected: JsonObject{"y": 3.0, "y2": 3.0},
		},
		"DiffParam": {
			// diff param - not cached
			query:    "{ y(a:2) y2:y(a:5) }",
			expected: JsonObject{"y": 2.0, "y2": 10.0},
		},
		"MixedParam": {
			// Mixture of same and different parameters
			// Note that 4th call to y (alias=y4) is only the 3rd call to getNext so should return 3
			query:    "{ y(a:10) y2:y(a:20) y3:y(a:10) y4:y(a:1) y5:y(a:20) }",
			expected: JsonObject{"y": 10.0, "y2": 40.0, "y3": 10.0, "y4": 3.0, "y5": 40.0},
		},
		"NotCachedSameParam": {
			query:    "{ n(a:3) n2:n(a:3) }",
			expected: JsonObject{"n": 3.0, "n2": 6.0},
		},
		"NotCachedDiffParam": {
			query:    "{ n(a:3) n2:n(a:3) }",
			expected: JsonObject{"n": 3.0, "n2": 6.0},
		},

		"AllParamsSame": {
			query:    "{ yyy(a:2, b:3, c:5) yyy2:yyy(a:2, b:3, c:5) }",
			expected: JsonObject{"yyy": 30.0, "yyy2": 30.0},
		},
		"TwoParamsSame": {
			query:    "{ yyy(a:2, b:3, c:5) yyy2:yyy(a:2, b:3, c:7) }",
			expected: JsonObject{"yyy": 30.0, "yyy2": 84.0},
		},
		"LastParamsSame": {
			query:    "{ yyy(a:1, b:3, c:5) yyy2:yyy(a:2, b:3, c:5) }",
			expected: JsonObject{"yyy": 15.0, "yyy2": 60.0},
		},
		"NoParamsSame": {
			query:    "{ yyy(a:-1, b:2, c:3) yyy2:yyy(a:1, b:4, c:7) }",
			expected: JsonObject{"yyy": -6.0, "yyy2": 56.0},
		},
		"SomeSameSomeNot": {
			query:    "{ yyy(a:2,b:3,c:5) y2:yyy(a:0,b:0,c:0) y3:yyy(a:1,b:2,c:3) y4:yyy(a:1,b:2,c:3) y5:yyy(a:2,b:3,c:5) }",
			expected: JsonObject{"yyy": 30.0, "y2": 0.0, "y3": 18.0, "y4": 18.0, "y5": 30.0},
		},
		"NoCacheParamsSame": {
			query:    "{ nnn(a:2, b:3, c:5) nnn2:nnn(a:2, b:3, c:5) }",
			expected: JsonObject{"nnn": 30.0, "nnn2": 60.0},
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
				handler.FuncCache(true),       // turn on function result cachi
				handler.NoConcurrency(true),   // simplifies tests (concurrent execution can affect order)
				handler.NoIntrospection(true), // aids debugging
			)

			// Build the request body and the HTTP request that uses it
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
