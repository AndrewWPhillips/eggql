package handler_test

// introspection_test.go tests that introspection queries produce the correct result

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
	Query struct {
		A Nested
		_ ObjectList
	}
	Nested struct {
		V    int
		List []bool
	}

	Simple     struct{ I int }
	ObjectList struct {
		List []Simple
	}

	Mutation struct {
		F func(int) int `egg:":E,args(e:E)"`
	}
)

const (
	schema = `"Descr. Q" type Query { a:Nested! } ` +
		`"Description N" type Nested { v:Int! list:[Boolean!] } ` +
		`"Description M" type Mutation { f(e:E!): E! }` +
		`"Description S" type Simple { i: Int! }` +
		`"Description L" type ObjectList { list: [Simple!] }` +
		`"Description E" enum E{E0 E1 E2}`
)

var introspectionData = map[string]struct {
	data  interface{} // corresponding struct
	query string      // GraphQL query to send to the handler (query syntax)

	//expected interface{} // expected result after decoding the returned JSON
	expected string // query result (JSON)
}{
	"Query TypeName": {
		query:    "{ __typename }",
		expected: `{"__typename": "Query"}`,
	},
	"Mutation TypeName": {
		query:    "mutation { __typename }",
		expected: `{"__typename": "Mutation"}`,
	},
	"QueryType": {
		query:    "{ __schema { queryType { name kind description } } }",
		expected: `{"__schema":{"queryType":{"name":"Query", "kind":"OBJECT", "description":"Descr. Q"} } }`,
	},
	"MutationType": {
		query:    "{ __schema { mutationType { name kind description } } }",
		expected: `{"__schema":{"mutationType":{"name":"Mutation", "kind":"OBJECT", "description":"Description M"} } }`,
	},
	"Type Query": {
		query:    `{ __type(name:\"Query\") { name } }`,
		expected: `{"__type": {"name": "Query"}}`,
	},
	"Type Int": {
		query:    `{ __type(name:\"Int\") { name kind } }`,
		expected: `{"__type": {"name": "Int", "kind": "SCALAR"}}`,
	},
	"Type Nested": {
		query:    `{ __type(name:\"Nested\") { name kind description } }`,
		expected: `{"__type": {"name": "Nested", "kind": "OBJECT", "description": "Description N"}}`,
	},
	"Type Enum": {
		query:    `{ __type(name:\"E\") { name description } }`,
		expected: `{"__type": {"name": "E", "description": "Description E"}}`,
	},
	"Args": {
		query:    `{ __type(name:\"Mutation\") { fields { name args { name }}} }`,
		expected: `{"__type": {"fields": [{"name": "f", "args": [{"name": "e"}]}]}}`,
	},
	"Enum": {
		query:    `{ __type(name:\"E\") { description enumValues { name }} }`,
		expected: `{"__type": {"description":"Description E", "enumValues": [{"name":"E0"}, {"name":"E1"}, {"name":"E2"}]}}`,
	},
	"Type List": {
		query: `{ __type(name:\"Nested\") { fields { name type { name kind ofType { name kind ofType { name kind }} } } } }`,
		expected: `{"__type": { "fields": [` +
			`  {"name": "v",   "type": {"name":"", "kind": "NON_NULL", "ofType": {"name":"Int", "kind": "SCALAR", "ofType": null}}}, ` +
			`  {"name": "list", "type": {"name":"", "kind": "LIST", "ofType": {"name":"", "kind": "NON_NULL", "ofType": {"name":"Boolean", "kind": "SCALAR"}}}}` +
			`]}}`,
	},
	"Type ObjLst": {
		query: `{ __type(name:\"ObjectList\") { fields { name type { name kind ofType { name kind ofType { name kind }} } } } }`,
		expected: `{"__type": {"fields":[` +
			`  {"name":"list", "type": {"name":"", "kind":"LIST", "ofType": {"name":"", "kind":"NON_NULL", "ofType": {"name": "Simple", "kind": "OBJECT"}}}}` +
			`]}}`,
	},
}

func TestIntrospection(t *testing.T) {
	for name, testData := range introspectionData {
		//log.Println(name) // we only need this if a test panics - to see which one it was
		h := handler.New(schema, map[string][]string{"E#Description E": {"E0", "E1", "E2"}}, Query{A: Nested{V: 1}}, Mutation{})

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
		got := writer.Body.String()

		// Decode the response and expected response so we can compare results w/o regard to whitespace etc
		var result struct {
			Data   interface{}
			Errors []struct{ Message string }
		}
		decoder := json.NewDecoder(writer.Body)
		if err := decoder.Decode(&result); err != nil {
			t.Logf("%12s: Error decoding JSON response: %v", name, err)
			t.Fail()
			continue
		}
		var expected interface{}
		decoder = json.NewDecoder(strings.NewReader(testData.expected))
		if err := decoder.Decode(&expected); err != nil {
			t.Logf("%12s: Error decoding expected JSON: %v", name, err)
			t.Fail()
			continue
		}

		// Check that the resulting GraphQL result (error and data)
		Assertf(t, result.Errors == nil, "%12s: Expected no error and got %v", name, result.Errors)
		Assertf(t, reflect.DeepEqual(result.Data, expected), "%12s: Expected %s, got %s", name, testData.expected, got)
	}
}
