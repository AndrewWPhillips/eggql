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

// TestErrors checks error responses returned for a bad GraphQL query or internal error/timeout
func TestErrors(t *testing.T) {
	errorData := map[string]struct {
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
		"UnknownArg": {
			"type Query{f(i:Int!):Int!}", struct {
				F func(int) int `egg:",args(j)"`
			}{F: func(j int) int { return j }}, `{ f(i:1) }`, "",
			`unknown argument "i" in resolver "f"`,
		},
		// TODO test all error conditions
	}

	for name, testData := range errorData {
		t.Run(name, func(t *testing.T) {
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
				Assertf(t, false, "Expected Status OK (200) got %d", status)
				return
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
				t.Logf("Error decoding JSON: %v", err)
				return
			}

			// Check that the resulting GraphQL result (error and data)
			if len(result.Data) > 0 {
				Assertf(t, false, "Expected no data and got %v", result.Data)
			}
			Assertf(t, result.Errors != nil && result.Errors[0].Message == testData.expError, "Expected error %q, got %v", testData.expError, result.Errors)
		})
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
