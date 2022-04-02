package handler_test

import (
	"encoding/json"
	"github.com/andrewwphillips/eggql/internal/handler"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
)

type (
	In struct {
		V int `graphql:"v:E"`
	}
	In2 struct {
		V1 int `graphql:":E"`
		V2 int `graphql:":E"`
	}
)

var enumData = map[string]struct {
	schema string      // GraphQL schema
	data   interface{} // corresponding struct
	query  string      // GraphQL query to send to the handler (query syntax)
	enums  map[string][]string

	expected interface{} // expected result after decoding the returned JSON
}{
	"Value": {
		schema: "type Query { v: Int! } enum E { E0 E1 E2 }",
		data: struct {
			V int `graphql:"v:E"`
		}{2},
		query:    "{ v }",
		enums:    map[string][]string{"E": {"E0", "E1", "E2"}},
		expected: JsonObject{"v": "E2"},
	},
	"Value0": {
		schema: "type Query { v: Int! } enum E { E0 E1 E2 }",
		data: struct {
			V int `graphql:"v:E"`
		}{V: 0},
		query:    "{ v }",
		enums:    map[string][]string{"E": {"E0", "E1", "E2"}},
		expected: JsonObject{"v": "E0"},
	},
	"Param": {
		schema: "type Query { f(p:E!): Int! } enum E { E0 E1 E2 }",
		data: struct {
			F func(int) int `graphql:",args(p:E)"`
		}{
			F: func(p int) int { return p },
		},
		query:    "{ f(p:E2) }",
		enums:    map[string][]string{"E#desc": {"E0", "E1", "E2"}},
		expected: JsonObject{"f": 2.0},
	},
	"DefaultParam": {
		schema: "type Query { f(p:E=E1): Int! } enum E { E0 E1 E2 }",
		data: struct {
			F func(int) int `graphql:",args(p:E=E0)"`
		}{
			F: func(p int) int { return p },
		},
		query:    "{ f }",
		enums:    map[string][]string{"E#desc": {"E0", "E1#desc", "E2"}},
		expected: JsonObject{"f": 1.0},
	},
	"InputParam": { // input type (arg to f) has an enum field (v)
		schema: "type Query { f(p:In!): Int! } input In { v: E! } enum E { E0 E1 E2 }",
		data: struct {
			F func(In) int `graphql:",args(p)"`
		}{
			F: func(in In) int { return in.V },
		},
		query:    `{ f(p: { v: E2 }) }`,
		enums:    map[string][]string{"E": {"E0", "E1", "E2"}},
		expected: JsonObject{"f": 2.0},
	},
	"InputParam2": { // input type (arg to f) has two enum fields (v1 and v2)
		schema: "type Query { f(p:In!): Int! } input In { v1: E! v2: E! } enum E { E0 E1 E2 }",
		data: struct {
			F func(In2) int `graphql:",args(p)"`
		}{
			F: func(in In2) int { return 10*in.V1 + in.V2 },
		},
		query:    `{ f(p: { v1: E2 v2: E1 }) }`,
		enums:    map[string][]string{"E": {"E0", "E1", "E2"}},
		expected: JsonObject{"f": 21.0},
	},
	"EnumDescription": {
		schema: "type Query { v: Int! } enum E { E0 E1 E2 }",
		data: struct {
			V int `graphql:"v:E"`
		}{2},
		query:    "{ v }",
		enums:    map[string][]string{"E# desc": {"E0", "E1", "E2"}},
		expected: JsonObject{"v": "E2"},
	},
	"EnumValueDescription": {
		schema: "type Query { v: Int! } enum E { E0 E1 E2 }",
		data: struct {
			V int `graphql:"v:E"`
		}{2},
		query:    "{ v }",
		enums:    map[string][]string{"E": {"E0", "E1", "E2#desc"}},
		expected: JsonObject{"v": "E2"},
	},
	"DescriptionBoth": {
		schema: "type Query { v: Int! } enum E { E0 E1 E2 }",
		data: struct {
			V int `graphql:"v:E"`
		}{0},
		query:    "{ v }",
		enums:    map[string][]string{"E# enum description": {"E0#desc 0", "E1#desc 1", "E2#desc 2"}},
		expected: JsonObject{"v": "E0"},
	},
}

func TestEnumQuery(t *testing.T) {
	for name, testData := range enumData {
		//log.Println(name) // we only need this if a test panics - to see which one it was
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
			t.Logf("%12s: Unexpected response code %d", name, writer.Code)
			continue
		}

		// Decode the JSON response
		var result struct {
			Data   interface{}
			Errors []struct{ Message string }
		}
		//json.Unmarshal(writer.Body.Bytes(), &result)
		decoder := json.NewDecoder(writer.Body)
		if err := decoder.Decode(&result); err != nil {
			t.Logf("%12s: Error decoding JSON: %v", name, err)
			continue
		}

		// Check that the resulting GraphQL result (error and data)
		Assertf(t, result.Errors == nil, "%12s: Expected no error and got %v", name, result.Errors)
		Assertf(t, reflect.DeepEqual(result.Data, testData.expected), "%12s: Expected %v, got %v", name, testData.expected, result.Data)
	}
}
