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
	setSchema = "schema {mutation: Mutation} type Mutation { set(p: Int!): Int! }"
)

var (
	toBeSet = 1
	setData = struct {
		Store func(int) int `graphql:",args(p)"`
	}{func(p int) int { toBeSet = p; return p }}
)

var mutationData = map[string]struct {
	schema    string      // GraphQL schema
	data      interface{} // corresponding matching struct
	query     string      // GraphQL query to send to the handler (query syntax)
	variables string      // GraphQL variables to use with the query (JSON)

	expected interface{} // expected result after decoding the returned JSON
	expSet   int
}{
	"Set": {setSchema, setData, `{ store(p: 3) }`, "",
		JsonObject{"store": 3.0}, 3},
}

func TestMutation(t *testing.T) {
	for name, testData := range mutationData {
		h := handler.New(testData.schema, testData.data)

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
			t.Logf("%12s: Error decoding JSON: %v", name, err)
			continue
		}

		// Check that the resulting GraphQL result (error and data)
		Assertf(t, result.Errors == nil, "%12s: Expected no error and got %v", name, result.Errors)
		Assertf(t, reflect.DeepEqual(result.Data, testData.expected), "%12s: Expected %v, got %v", name, testData.expected, result.Data)
		Assertf(t, toBeSet == testData.expSet, "%12s: Expected stored value %d, got %d", name, testData.expSet, toBeSet)
	}
}
