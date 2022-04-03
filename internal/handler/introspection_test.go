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
	Query  struct{ A Nested }
	Nested struct {
		V    int
		List []bool
	}
	Mutation struct {
		F func(int) int `graphql:":E,args(e:E)"`
	}
)

const (
	schema = `"Descr. Q" type Query { a:Nested! } ` +
		`"Description N" type Nested { v:Int! list:[Boolean!] } ` +
		`"Description M" type Mutation { f(e:E!): E! }` +
		`"Description E" enum E{E0 E1 E2}`
)

var introspectionData = map[string]struct {
	data  interface{} // corresponding struct
	query string      // GraphQL query to send to the handler (query syntax)

	expected interface{} // expected result after decoding the returned JSON
}{
	"Query TypeName": {
		query:    "{ __typename }",
		expected: JsonObject{"__typename": "Query"},
	},
	"Mutation TypeName": {
		query:    "mutation { __typename }",
		expected: JsonObject{"__typename": "Mutation"},
	},
	"QueryType": {
		query: "{ __schema { queryType { name kind description } } }",
		expected: JsonObject{"__schema": JsonObject{"queryType": JsonObject{
			"name": "Query", "kind": "OBJECT", "description": "Descr. Q",
		}}},
	},
	"MutationType": {
		query: "{ __schema { mutationType { name kind description } } }",
		expected: JsonObject{"__schema": JsonObject{"mutationType": JsonObject{
			"name": "Mutation", "kind": "OBJECT", "description": "Description M",
		}}},
	},
	"Type Query": {
		query:    `{ __type(name:\"Query\") { name } }`,
		expected: JsonObject{"__type": JsonObject{"name": "Query"}},
	},
	"Type Int": {
		query:    `{ __type(name:\"Int\") { name kind } }`,
		expected: JsonObject{"__type": JsonObject{"name": "Int", "kind": "SCALAR"}},
	},
	"Type Nested": {
		query:    `{ __type(name:\"Nested\") { name kind description } }`,
		expected: JsonObject{"__type": JsonObject{"name": "Nested", "kind": "OBJECT", "description": "Description N"}},
	},
	"Type Enum": {
		query:    `{ __type(name:\"E\") { name description } }`,
		expected: JsonObject{"__type": JsonObject{"name": "E", "description": "Description E"}},
	},
	"Args": {
		query:    `{ __type(name:\"Mutation\") { fields { name args { name }}} }`,
		expected: JsonObject{"__type": JsonObject{"fields": []interface{}{JsonObject{"name": "f", "args": []interface{}{JsonObject{"name": "e"}}}}}},
	},
	"Type List": {
		query: `{ __type(name:\"Nested\") { fields { name type { name kind ofType { name kind } } } } }`,
		expected: JsonObject{
			"__type": JsonObject{
				"fields": []interface{}{
					JsonObject{
						"name": "v",
						"type": JsonObject{
							"name":   "Int",
							"kind":   "SCALAR",
							"ofType": nil,
						},
					},
					JsonObject{
						"name": "list",
						"type": JsonObject{
							"name": "",
							"kind": "LIST",
							"ofType": JsonObject{
								"name": "Boolean",
								"kind": "SCALAR",
							},
						},
					},
				},
			},
		},
	},
	/*
		"AllTypes": {
			query:    "{ __schema { types { name } } }",
			expected: JsonObject{"__schema": JsonObject{"types": "TODO"}},
		},
	*/
}

func TestIntrospection(t *testing.T) {
	for name, testData := range introspectionData {
		//log.Println(name) // we only need this if a test panics - to see which one it was
		h := handler.New(schema, map[string][]string{"E": {"E0", "E1", "E2"}}, Query{A: Nested{V: 1}}, Mutation{})

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
			t.Fail()
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
			t.Fail()
			continue
		}

		// Check that the resulting GraphQL result (error and data)
		Assertf(t, result.Errors == nil, "%12s: Expected no error and got %v", name, result.Errors)
		Assertf(t, reflect.DeepEqual(result.Data, testData.expected), "%12s: Expected %v, got %v", name, testData.expected, result.Data)
	}
}
