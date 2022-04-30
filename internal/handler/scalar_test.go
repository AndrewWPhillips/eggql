package handler_test

// scalar_test.go has tests for checking the custom scalars are processed correctly

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/andrewwphillips/eggql/internal/handler"
)

// SimpleScalar - implements UnmarshalEGGQL (relies on %v printf format for marshaling)
type SimpleScalar int8

func (pi *SimpleScalar) UnmarshalEGGQL(s string) error {
	tmp, err := strconv.Atoi(s)
	if err != nil {
		return fmt.Errorf("UnmarshalEGGQL: error %w decoding Cust with Atoi", err)
	}
	*pi = SimpleScalar(tmp)
	return nil
}

// TimeScalar uses Go time type to implement a custom scalar - using time.Time.String() for marshaling
type TimeScalar struct {
	time.Time // by embedding we automatically get time.Time.String() method
}

func (pt *TimeScalar) UnmarshalEGGQL(in string) error {
	tmp, err := time.Parse("2006-01-02 15:04:05.999999999 -0700 MST", in)
	if err != nil {
		return fmt.Errorf("%w error in UnmarshalEGGQL for custom scalar Time", err)
	}
	pt.Time = tmp
	return nil
}

// BothScalar is a custom scalar with both MarshalEGGQL and UnmarshalEGGQL
type BothScalar int16

func (i BothScalar) MarshalEGGQL() (string, error) {
	return "test_" + strconv.Itoa(int(i)), nil
}

func (pi *BothScalar) UnmarshalEGGQL(in string) error {
	s := strings.TrimPrefix(in, "test_")
	if s == in {
		return errors.New(`UnmarshalEGGQL: can't decode BothScalar value - should begin with "test_"`)
	}
	if tmp, err := strconv.Atoi(s); err != nil {
		return fmt.Errorf("UnmarshalEGGQL: error %w decoding Cust with Atoi", err)
	} else {
		*pi = BothScalar(tmp)
	}
	return nil
}

// StringScalar implements MarshalEGGQL with pointer receiver
type StringScalar string

func (p *StringScalar) MarshalEGGQL() (string, error) {
	return "PU:" + string(*p), nil
}

func (p *StringScalar) UnmarshalEGGQL(in string) error {
	s := strings.TrimPrefix(in, "PU:")
	if s == in {
		return errors.New(`UnmarshalEGGQL: can't decode StringScalar value - should begin with "CUST:"`)
	}
	*p = StringScalar(s)
	return nil
}

// TestCustomScalar tests different uses of custom scalars
func TestCustomScalar(t *testing.T) {
	scalarData := map[string]struct {
		schema string      // GraphQL schema
		data   interface{} // corresponding struct
		query  string      // GraphQL query to send to the handler (query syntax)

		expected string // expected result (JSON)
	}{
		"Simple": {
			schema: "type Query { f(a:Simple!): Simple! } scalar Simple",
			data: struct {
				F func(scalar SimpleScalar) SimpleScalar `egg:",args(a)"`
			}{
				F: func(a SimpleScalar) SimpleScalar { return a * a },
			},
			query:    `{ f(a:\"7\") }`,
			expected: `{"f": "49"}`,
		},
		"Field Value": {
			schema:   "type Query { v: BothScalar! } scalar BothScalar",
			data:     struct{ V BothScalar }{2},
			query:    "{ v }",
			expected: `{"v": "test_2"}`,
		},
		"Field List": {
			schema:   "type Query { v: [BothScalar!] } scalar BothScalar",
			data:     struct{ V []BothScalar }{[]BothScalar{2, 3}},
			query:    "{ v }",
			expected: `{"v": ["test_2", "test_3"] }`,
		},
		"Input Field": {
			schema: "type Query { f(a:A):Int! } input A{ v:BothScalar! } scalar BothScalar",
			data: struct {
				F func(struct{ V BothScalar }) int `egg:",args(a)"`
			}{
				F: func(a struct{ V BothScalar }) int { return int(a.V) },
			},
			query:    "{ f(a:{ v:test_7}) }",
			expected: `{"f": 7 }}`,
		},
		"Arg Value": {
			schema: "type Query { f(v:BothScalar!): Int! } scalar BothScalar",
			data: struct {
				F func(BothScalar) int `egg:",args(v)"`
			}{
				F: func(v BothScalar) int { return int(v) },
			},
			query:    `{ f(v:test_3) }`,
			expected: `{"f": 3}`,
		},
		"Time": {
			schema: "type Query { f(t:TimeScalar!): TimeScalar! } scalar TimeScalar",
			data: struct {
				F func(scalar TimeScalar) TimeScalar `egg:",args(t)"`
			}{
				F: func(t TimeScalar) (r TimeScalar) { r.Time = t.Time.Add(time.Hour); return },
			},
			query:    `{ f(t:\"2006-01-02 15:04:05.99 -0700 MST\") }`,
			expected: `{"f": "2006-01-02 16:04:05.99 -0700 MST"}`,
		},
		"String": {
			schema: "type Query { f(a:StringScalar!): StringScalar! } scalar StringScalar",
			data: struct {
				F func(scalar StringScalar) StringScalar `egg:",args(a)"`
			}{
				F: func(a StringScalar) StringScalar { return StringScalar(strings.ToUpper(string(a))) },
			},
			query:    `{ f(a:\"PU:test\") }`,
			expected: `{"f": "PU:TEST"}`,
		},
	}

	for name, testData := range scalarData {
		t.Run(name, func(t *testing.T) {
			h := handler.New(testData.schema, testData.data)

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
				t.Logf("%12s: Error decoding JSON: %v", name, err)
				t.Fail()
				return
			}
			var expected interface{}
			decoder = json.NewDecoder(strings.NewReader(testData.expected))
			if err := decoder.Decode(&expected); err != nil {
				t.Logf("%12s: Error decoding expected JSON: %v", name, err)
				t.Fail()
				return
			}

			// Check the resulting GraphQL result (error and data)
			if result.Errors != nil {
				Assertf(t, result.Errors == nil, "Expected no error and got %v", result.Errors)
			}
			Assertf(t, reflect.DeepEqual(result.Data, expected), "Expected %v, got %v", expected, result.Data)
		})
	}
}
