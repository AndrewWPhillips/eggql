package handler_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/andrewwphillips/eggql/internal/handler"
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
	"FuncError":  {"type Query{v:Int!}", errorFuncData, `{v}`, "", errorMessage},
	"QueryError": {"type Query{v:Int!}", errorFuncData, `x`, "", `Unexpected Name "x"`},
	"UnknownQuery": {
		"type Query{v:Int!}", errorFuncData, `{ unknown }`, "",
		`Cannot query field "unknown" on type "Query".`,
	},
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
		h.ServeHTTP(writer, request) /****/

		// All of these tests should give status OK
		if status := writer.Result().StatusCode; status != http.StatusOK {
			Assertf(t, false, "%12s: Expected Status OK (200) got %d", name, status)
			continue
		}

		// Decode the JSON response
		var result struct {
			Data map[string]interface{} `json:"data,omitempty"`
			//Data   interface{}                `json:",omitempty"`
			Errors []struct{ Message string } `json:",omitempty"`
		}
		//json.Unmarshal(writer.Body.Bytes(), &result)
		decoder := json.NewDecoder(writer.Body)
		if err := decoder.Decode(&result); err != nil {
			t.Logf("%12s: Error decoding JSON: %v", name, err)
			continue
		}

		// Check that the resulting GraphQL result (error and data)
		if len(result.Data) > 0 {
			Assertf(t, false, "%12s: Expected no data and got %v", name, result.Data)
		}
		Assertf(t, result.Errors != nil && result.Errors[0].Message == testData.expError, "%12s: Expected error %q, got %v", name, testData.expError, result.Errors)
	}
}

func TestQueryTimeout(t *testing.T) {
	h := handler.New("type Query{v:Int!}", struct{ V func() int }{func() int { time.Sleep(5 * time.Second); return 0 }})

	request := httptest.NewRequest("POST", "/", strings.NewReader(`{"query":"{v}"}`))
	request.Header.Add("Content-Type", "application/json")

	ctx, cancel := context.WithCancel(context.Background())
	request = request.WithContext(ctx)

	writer := httptest.NewRecorder()
	wg := sync.WaitGroup{}
	wg.Add(1)
	go func() {
		h.ServeHTTP(writer, request) /****/
		wg.Done()
	}()

	// Cancel the context and wait for the request to finish
	cancel()
	wg.Wait()
	if !strings.Contains(writer.Body.String(), `"context canceled"`) {
		t.Fatalf("Expected returned JSON to contain an error about canceled context but got %q", writer.Body.String())
	}
}

func TestMutationTimeout(t *testing.T) {
	h := handler.New("type Mutation{m:Int!}", nil, struct {
		M func(context.Context) (int, error)
	}{
		func(ctx context.Context) (int, error) {
			for i := 0; i < 100; i++ {
				if ctx.Err() != nil {
					return -1, ctx.Err()
				}
				time.Sleep(100 * time.Millisecond)
			}
			return 0, nil
		},
	})

	request := httptest.NewRequest("POST", "/", strings.NewReader(`{"query":"mutation{m}"}`))
	request.Header.Add("Content-Type", "application/json")

	ctx, cancel := context.WithCancel(context.Background())
	request = request.WithContext(ctx)

	writer := httptest.NewRecorder()
	wg := sync.WaitGroup{}
	wg.Add(1)
	go func() {
		h.ServeHTTP(writer, request) /****/
		wg.Done()
	}()

	// Cancel the context and wait for the request to finish
	cancel()
	wg.Wait()
	if !strings.Contains(writer.Body.String(), `"context canceled"`) {
		t.Fatalf("Expected returned JSON to contain an error about canceled context but got %q", writer.Body.String())
	}
}
