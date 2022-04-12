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

// Note that the schema strings (below) must closely match the structs (further below).  (In production, this is ensured
// by schema.Build which generates a schema based on a Go struct.)  For example, the stringSchema Query has a single
// String! field called "message" and the corresponding stringData struct has a single string field called
// "Message", where the field name must be capitalised (exported).  Similarly, the funcData struct, which is also
// used with the stringSchema, has a "Message" field, but in this case it is a func() returning string.

const (
	storeSchema = "type Mutation { store(p: Int!): Int! }"
	threeSchema = "type Mutation { three(a:Int!, b:Int!, c:Int!): Int! }"
)

var (
	toBeSet int

	storeData = struct {
		Store func(int) int `graphql:",args(p)"`
	}{
		func(p int) int { toBeSet = p; return p },
	}

	threeData = struct {
		Three func(int, int, int) int `graphql:",args(a,b,c)"`
	}{
		func(a, b, c int) int {
			toBeSet = a + b + c
			return a*100 + b*10 + c
		},
	}
)

var mutationData = map[string]struct {
	schema    string      // GraphQL schema
	data      interface{} // corresponding matching mutation
	query     string      // GraphQL query to send to the handler (query syntax)
	variables string      // GraphQL variables to use with the query (JSON)

	expected string // expected returned JSON
	expSet   int    // expected value of global
}{
	"Store": {storeSchema, storeData, `mutation { store(p: 42) }`, "",
		`{"store": 42}`, 42},
	"Three": {threeSchema, threeData, `mutation { three(a:1 b:2 c:3) }`, "",
		`{"three": 123}`, 6},
	"Reverse": {threeSchema, threeData, `mutation { three(c:1 b:2 a:3) }`, "",
		`{"three": 321}`, 6},
	"Variables": {threeSchema, threeData, `mutation Vars($i: Int! $j: Int!) { three(b:5 a:$i, c:$j) }`,
		`{"i": 3, "j": 7 }`,
		`{"three": 357}`, 15},
}

func TestMutation(t *testing.T) {
	for name, testData := range mutationData {
		toBeSet = -1
		h := handler.New(testData.schema, nil, testData.data)

		// Make the request body and the HTTP request that uses it
		body := strings.Builder{}
		body.WriteString(`{"query":"`)
		body.WriteString(testData.query)
		body.WriteString(`"`)
		if testData.variables != "" {
			body.WriteString(`,"variables":`)
			body.WriteString(testData.variables)
		}
		body.WriteString(`}`)

		request := httptest.NewRequest("POST", "/", strings.NewReader(body.String()))
		request.Header.Add("Content-Type", "application/json")

		// Invoke the handler, recording the response
		writer := httptest.NewRecorder()
		h.ServeHTTP(writer, request)

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
			t.Errorf("%12s: Error decoding JSON: %v", name, err)
			continue
		}
		var expected interface{}
		decoder = json.NewDecoder(strings.NewReader(testData.expected))
		if err := decoder.Decode(&expected); err != nil {
			t.Errorf("%12s: Error decoding expected JSON: %v", name, err)
			continue
		}

		// Check that the resulting GraphQL result (error and data)
		if result.Errors != nil {
			Assertf(t, result.Errors == nil, "%12s: Expected no error and got %v", name, result.Errors)
		}
		Assertf(t, reflect.DeepEqual(result.Data, expected), "%12s: Expected %v, got %v", name, expected, result.Data)
		Assertf(t, toBeSet == testData.expSet, "%12s: Expected stored value %d, got %d", name, testData.expSet, toBeSet)
	}
}
