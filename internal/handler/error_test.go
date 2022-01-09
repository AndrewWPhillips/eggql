package handler_test

import (
	"encoding/json"
	"errors"
	"github.com/andrewwphillips/eggql/internal/handler"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const (
	errorMessage = "resolver func error"
)

var (
	errorFuncData = struct{ V func() (int, error) }{func() (int, error) { return 0, errors.New(errorMessage) }}
)

// errorData is for testing GraphQL error responses returned for a bad query or internal error/timeout
var errorData = map[string]struct {
	schema    string      // GraphQL schema
	data      interface{} // corresponding matching struct
	query     string      // GraphQL query to send to the handler (query syntax)
	variables string      // GraphQL variables to use with the query (JSON)
	expError  string      // expected error decoded from the returned JSON response
}{
	"FuncError":    {"type Query{v:Int!}", errorFuncData, `{v}`, "", errorMessage},
	"QueryError":   {"type Query{v:Int!}", errorFuncData, `x`, "", `Unexpected Name "x"`},
	"UnknownQuery": {"type Query{v:Int!}", errorFuncData, `{ unknown }`, "", `Cannot query field "unknown" on type "Query".`},
	// TODO test all error conditions
}

func TestErrors(t *testing.T) {
	for name, testData := range errorData {
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
		Assertf(t, result.Data == nil, "%12s: Expected no data and got %v", name, result.Data)
		Assertf(t, result.Errors[0].Message == testData.expError, "%12s: Expected error %q, got %v", name, testData.expError, result.Errors)
	}
}
