package handler_test

// enum_test.go has tests of enum processing

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/andrewwphillips/eggql/internal/handler"
)

type (
	In struct {
		V int `egg:"v:E"`
	}
	In2 struct {
		V1 int `egg:":E"`
		V2 int `egg:":E"`
	}
)

// TestEnumQuery has test queries for checking enum fields, arguments, defaults, descriptions, etc
func TestEnumQuery(t *testing.T) {
	enumData := map[string]struct {
		schema string      // GraphQL schema
		data   interface{} // corresponding struct
		query  string      // GraphQL query to send to the handler (query syntax)
		enums  map[string][]string

		expected string // expected result (JSON)
	}{
		"Value": {
			schema: "type Query { v: Int! } enum E { E0 E1 E2 }",
			data: struct {
				V int `egg:"v:E"`
			}{2},
			query:    "{ v }",
			enums:    map[string][]string{"E": {"E0", "E1", "E2"}},
			expected: `{"v": "E2"}`,
		},
		"Value0": {
			schema: "type Query { v: Int! } enum E { E0 E1 E2 }",
			data: struct {
				V int `egg:"v:E"`
			}{V: 0},
			query:    "{ v }",
			enums:    map[string][]string{"E": {"E0", "E1", "E2"}},
			expected: `{"v": "E0"}`,
		},
		"Param": {
			schema: "type Query { f(p:E!): Int! } enum E { E0 E1 E2 }",
			data: struct {
				F func(int) int `egg:"(p:E)"`
			}{
				F: func(p int) int { return p },
			},
			query:    "{ f(p:E2) }",
			enums:    map[string][]string{"E#desc": {"E0", "E1", "E2"}},
			expected: `{"f": 2.0}`,
		},
		"DefaultParam": {
			schema: "type Query { f(p:E=E1): Int! } enum E { E0 E1 E2 }",
			data: struct {
				F func(int) int `egg:"(p:E=E0)"`
			}{
				F: func(p int) int { return p },
			},
			query:    "{ f }",
			enums:    map[string][]string{"E#desc": {"E0", "E1#desc", "E2"}},
			expected: `{"f": 1.0}`,
		},
		"InputParam": {
			// input type (arg to f) has an enum field (v)
			schema: "type Query { f(p:In!): Int! } input In { v: E! } enum E { E0 E1 E2 }",
			data: struct {
				F func(In) int `egg:"(p)"`
			}{
				F: func(in In) int { return in.V },
			},
			query:    `{ f(p: { v: E2 }) }`,
			enums:    map[string][]string{"E": {"E0", "E1", "E2"}},
			expected: `{"f": 2.0}`,
		},
		"InputParam2": {
			// input type (arg to f) has two enum fields (v1 and v2)
			schema: "type Query { f(p:In!): Int! } input In { v1: E! v2: E! } enum E { E0 E1 E2 }",
			data: struct {
				F func(In2) int `egg:"(p)"`
			}{
				F: func(in In2) int { return 10*in.V1 + in.V2 },
			},
			query:    `{ f(p: { v1: E2 v2: E1 }) }`,
			enums:    map[string][]string{"E": {"E0", "E1", "E2"}},
			expected: `{"f": 21.0}`,
		},
		"EnumDescription": {
			schema: "type Query { v: Int! } enum E { E0 E1 E2 }",
			data: struct {
				V int `egg:"v:E"`
			}{2},
			query:    "{ v }",
			enums:    map[string][]string{"E# desc": {"E0", "E1", "E2"}},
			expected: `{"v": "E2"}`,
		},
		"EnumValueDescription": {
			schema: "type Query { v: Int! } enum E { E0 E1 E2 }",
			data: struct {
				V int `egg:"v:E"`
			}{2},
			query:    "{ v }",
			enums:    map[string][]string{"E": {"E0", "E1", "E2#desc"}},
			expected: `{"v": "E2"}`,
		},
		"DescriptionBoth": {
			schema: "type Query { v: Int! } enum E { E0 E1 E2 }",
			data: struct {
				V int `egg:"v:E"`
			}{0},
			query:    "{ v }",
			enums:    map[string][]string{"E# enum description": {"E0#desc 0", "E1#desc 1", "E2#desc 2"}},
			expected: `{"v": "E0"}`,
		},
	}

	for name, testData := range enumData {
		t.Run(name, func(t *testing.T) {
			h := handler.New(testData.schema, testData.enums, testData.data)

			// Make the request body and the HTTP request that uses it
			body := strings.Builder{}
			body.WriteString(`{"query":"`)
			body.WriteString(testData.query)
			body.WriteString(`"}`)

			request := httptest.NewRequest("POST", "/", strings.NewReader(body.String()))
			request.Header.Add("Content-Type", "application/json")

			// Invoke the handler, recording the response
			writer := httptest.NewRecorder()
			h.ServeHTTP(writer, request) /*****/

			// All of these tests should give status OK
			if writer.Result().StatusCode != http.StatusOK {
				t.Logf("Unexpected response code %d", writer.Code)
				t.Fail()
				return
			}

			// Decode the JSON response
			var result struct {
				Data   interface{}
				Errors []struct{ Message string }
			}
			//json.Unmarshal(writer.Body.Bytes(), &result)
			decoder := json.NewDecoder(writer.Body)
			if err := decoder.Decode(&result); err != nil {
				t.Logf("Error decoding JSON: %v", err)
				t.Fail()
				return
			}
			var expected interface{}
			decoder = json.NewDecoder(strings.NewReader(testData.expected))
			if err := decoder.Decode(&expected); err != nil {
				t.Logf("Error decoding expected JSON: %v", err)
				t.Fail()
				return
			}

			// Check that the resulting GraphQL result (error and data)
			if result.Errors != nil {
				Assertf(t, result.Errors == nil, "Expected no error and got %v", result.Errors)
			}
			Assertf(t, reflect.DeepEqual(result.Data, expected), "Expected %v, got %v", expected, result.Data)
		})
	}
}
