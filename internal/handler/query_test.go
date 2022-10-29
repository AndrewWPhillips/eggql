package handler_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/andrewwphillips/eggql"
	"github.com/andrewwphillips/eggql/internal/handler"
)

// Note that the schema strings (below) must closely match the structs (further below).  (In production, this is ensured
// by schema.Build which generates a schema based on a Go struct.)  For example, the stringSchema Query has a single
// String! field called "message" and the corresponding stringData struct has a single string field called
// "Message", where the field name must be capitalised (exported).  Similarly, the funcData struct, which is also
// used with the stringSchema, has a "Message" field, but in this case it is a func() returning string.

const (
	stringSchema         = "type Query { message: String! }"
	listSchema           = "type Query { values: [Int!] }"
	nestedSchema         = "type Query { n: N! } type N { q: Boolean! p: Boolean! }"
	argsSchema           = "type Query { dbl(v: Int!): Int! }"
	args2Schema          = "type Query { f(i: Int!, s: String!): String! }"
	default1Schema       = "type Query { f(i: Int!, s: String! = \"xyz\"): String! }"
	default2Schema       = "type Query { f(i: Int! = 87, s: String! = \"ijk\"): String! }"
	inputArgSchema       = "type Query { inputQuery(param: inputType!): Int! } input inputType { field: String! }"
	inputArg2FieldSchema = "type Query { q(p: R!): String! } input R{s:String! f:Float!}"
	listArgSchema        = "type Query { listQuery(list: [Int!]!): Int! }"
	interfaceSchema      = "type Query { a: D! } interface X { x1: Int! } type D implements X { x1: Int! e: String! }"
	union3Schema         = "type Query { c: [U] } type U1 { v: Int! } type U2 { v: Int! w: String!} union U = U1|U2"
	subscriptSlice       = "schema {query: QuerySubscript} type QuerySubscript { slice(id: Int!): String! }"
	subscriptMap         = "schema {query: QuerySubscript} type QuerySubscript { map(number: String!): Float! }"
	sliceFieldSchema     = "schema {query:QuerySliceFieldID} type QuerySliceFieldID{ s:[Element]! } type Element{ id:String! b:Int!}"
	mapFieldSchema       = "schema {query:QueryMapFieldID} type QueryMapFieldID{ m:[Element]! } type Element{ id:String! b:Int!}"
)

type (
	inputArg2FieldType struct {
		S string
		F float64
	}
	QueryName struct{ B byte }

	// X and Y are embedded in other structs to implement GraphQL interfaces X and Y
	X struct {
		X1 int
	}
	Y struct {
		Y1 bool
	}
	D struct {
		X
		E string
	}

	Element           struct{ B byte }
	QuerySliceFieldID struct {
		S []Element `egg:",field_id"`
	}
	QueryMapFieldID struct {
		M map[string]Element `egg:",field_id"`
	}
	QueryOffsetID struct {
		S []Element `egg:",field_id,base=100"`
	}

	// U is embedded in other structs to implement a union
	U  struct{}
	U1 struct {
		U
		V int
	}
	U2 struct {
		U
		V int
		W string
	}

	ParentRef struct {
		private int
		Value   func() int // closure (set to point to ParentRef.valueFunc method below)
	}

	QuerySubscript struct {
		Slice []string           `egg:",subscript"`
		Map   map[string]float64 `egg:",subscript=number"`
	}
)

var (
	stringData    = struct{ Message string }{"hello"}
	real          = 1.73205 // must be var not const so we can take it's address
	funcData      = struct{ Message func() string }{func() string { return "hi" }}
	nestedData    = struct{ N struct{ P, Q bool } }{struct{ P, Q bool }{true, false}}
	stringIntData = struct {
		M string
		V int
	}{"mmm", 43}
	paramData = struct {
		Dbl func(int) int `egg:"(v)"`
	}{func(value int) int { return 2 * value }}
	param2ArgData = struct {
		F func(int, string) string `egg:"(i,s)"`
	}{func(i int, s string) string { return s + strconv.Itoa(i) }}
	default1Data = struct {
		F func(int, string) string `egg:"(i,s=xyz)"`
	}{func(i int, s string) string { return s + strconv.Itoa(i*2) }}
	default2Data = struct {
		F func(int, string) string `egg:"(i=87,s=ijk)"`
	}{func(i int, s string) string { return strconv.Itoa(i) + s }}
	inputArgData = struct {
		InputQuery func(struct{ Field string }) int `egg:"(param)"`
	}{func(p struct{ Field string }) int { r, _ := strconv.Atoi(p.Field); return r }}
	inputArg2FieldData = struct {
		Q func(inputArg2FieldType) string `egg:"(p)"`
	}{func(parm inputArg2FieldType) string { return parm.S + strconv.FormatFloat(parm.F, 'g', 10, 64) }}
	sliceArgData = struct {
		ListQuery func([]int) int `egg:"(list)"`
	}{func(list []int) int { return len(list) }}
	arrayArgData = struct {
		ListQuery func([3]int) int `egg:"(list)"`
	}{func(list [3]int) int { return len(list) }}

	interfaceData  = struct{ A D }{D{X{4}, "fff"}}
	interfaceFunc  = struct{ A func() D }{func() D { return D{X{5}, "ggg"} }}
	inlineFragFunc = struct {
		_ [0]D // we need this as A returns a struct D as an interface
		A func() interface{}
	}{A: func() interface{} { return D{X{1}, "e in D"} }}

	contextFunc  = struct{ Value func(context.Context) int }{func(ctx context.Context) int { return 100 }}
	contextFunc1 = struct {
		Dbl func(context.Context, int) int `egg:"(v)"`
	}{func(ctx context.Context, i int) int { return 100 + 2*i }}
	contextFunc2 = struct {
		F func(context.Context, int, string) string `egg:"(i,s)"`
	}{func(ctx context.Context, i int, s string) string { return strconv.Itoa(i) + s }}

	parRef = ParentRef{private: 42}

	subscript = QuerySubscript{
		Slice: []string{"zero", "", "two"},
		Map:   map[string]float64{"pi": 3.14159265359, "root2": 1.41421356237},
	}
	sliceFieldID  = QuerySliceFieldID{[]Element{{11}, {12}}}
	mapFieldID    = QueryMapFieldID{map[string]Element{"a": {1}}}
	sliceOffsetID = QueryOffsetID{[]Element{{21}, {22}}}
)

func (p *ParentRef) valueFunc() int {
	return p.private
}

// JsonObject is what json.Unmarshaler produces when it decodes a JSON object.  Not that we use a type alias here,
//   hence the equals sing (=) rather than a type definition otherwise reflect.DeepEqual does not work.
type JsonObject = map[string]interface{}

// TestQuery runs test for "normal" GrqphQL queries (ie no errors, no special types, etc)
func TestQuery(t *testing.T) {
	var happyData = map[string]struct {
		schema    string      // GraphQL schema
		data      interface{} // corresponding matching struct
		query     string      // GraphQL query to send to the handler (query syntax)
		variables string      // GraphQL variables to use with the query (JSON)

		expected interface{} // expected result after decoding the returned JSON
	}{
		"String": {
			stringSchema, stringData, `{ message }`, "",
			JsonObject{"message": "hello"},
		},
		"NamedQuery": {
			"schema {query:QueryName} type QueryName{b:Int!}", QueryName{B: 'A'}, `{b}`, "",
			JsonObject{"b": float64('A')},
		},
		"Integer": {
			"type Query { value: Int! }", &struct{ Value int }{42}, `{ value }`, "",
			JsonObject{"value": float64(42)},
		}, // all numbers decode to float64
		"Boolean": {
			"type Query { b : Boolean! }", struct{ B bool }{true}, `{ b }`, "",
			JsonObject{"b": true},
		},
		"Float": {
			"type Query { f: Float! }", struct{ F float64 }{1.5}, `{ f }`, "",
			JsonObject{"f": 1.5},
		},
		"IDstring": {
			"type Query{id:ID!}", struct {
				Id string `egg:":ID"` // specify it has GraphQL ID type
			}{"12-34"}, `{ id }`, "",
			JsonObject{"id": "12-34"},
		},
		"IDint": {
			"type Query{id:ID!}", struct {
				Id int `egg:":ID"` // specify it has GraphQL ID type
			}{12}, `{ id }`, "",
			JsonObject{"id": 12.0},
		},
		"IDtype": {
			"type Query{id:ID!}", struct{ Id eggql.ID }{"0xFFFF"}, `{ id }`, "",
			JsonObject{"id": "0xFFFF"},
		},
		"Slice": {
			listSchema, struct{ Values []int }{[]int{1, 8, 3}}, `{ values }`, "",
			JsonObject{"values": []interface{}{1.0, 8.0, 3.0}},
		},
		"Array": {
			listSchema, struct{ Values [1]int }{[1]int{42}}, `{ values }`, "",
			JsonObject{"values": []interface{}{42.0}},
		},
		// TODO: write test for map with more than one element (order of returned map elements is indeterminate)
		"Map": {
			listSchema, struct{ Values map[string]int }{map[string]int{"": 42}}, `{values}`, "",
			JsonObject{"values": []interface{}{42.0}},
		},
		"Pointer": {
			"type Query { f: Float! }", struct{ F *float64 }{&real}, `{ f }`, "",
			JsonObject{"f": real},
		},
		"Func": {
			stringSchema, funcData, `{ message }`, "",
			JsonObject{"message": "hi"},
		},
		"Nested": {
			nestedSchema, nestedData, `{ n { p q } }`, "",
			JsonObject{"n": JsonObject{"p": true, "q": false}},
		},
		"TwoQueries": {
			"type Query { m: String! v: Int! }", stringIntData, `{ m v }`, "",
			JsonObject{"m": "mmm", "v": 43.0},
		},

		// Resolvers with arguments (inline)
		"ArgInt": {
			argsSchema, paramData, `{ dbl(v: 21) }`, "",
			JsonObject{"dbl": 42.0},
		},
		"ArgID": {
			"type Query { f(id: ID!): String! }",
			struct {
				F func(eggql.ID) string `egg:"(id)"`
			}{func(id eggql.ID) string { return string(id) }}, `{ f(id: 123) }`, "",
			JsonObject{"f": "123"},
		},
		"Arg2": {
			args2Schema, param2ArgData, `{ f(i:7, s:\"abc\") }`, "",
			JsonObject{"f": "abc7"},
		},
		"IDargInt": {
			"type Query{f(id:ID!):Boolean!}", struct {
				F func(int) bool `egg:"(id:ID)"`
			}{F: func(i int) bool { return i == 42 }},
			`{ f(id:42) }`, "", JsonObject{"f": true},
		},
		"IDargString": {
			"type Query{f(id:ID!):Boolean!}", struct {
				F func(string) bool `egg:"(id:ID)"`
			}{F: func(s string) bool { return s == "i42" }},
			`{ f(id:\"i42\") }`, "", JsonObject{"f": true},
		},
		"Default1": {
			default1Schema, default1Data, `{ f(i:7) }`, "",
			JsonObject{"f": "xyz14"},
		},
		"NoDefault1": {
			default1Schema, default1Data, `{ f(i:8, s:\"ABC\") }`, "",
			JsonObject{"f": "ABC16"},
		},
		"Default2": {
			default2Schema, default2Data, `{ f }`, "",
			JsonObject{"f": "87ijk"},
		},
		"NoDefault2": {
			default2Schema, default2Data, `{ f(i:99, s:\"IJK\") }`, "",
			JsonObject{"f": "99IJK"},
		},
		"FirstDefault2": {
			default2Schema, default2Data, `{ f(s:\"\") }`, "",
			JsonObject{"f": "87"},
		},
		"SecondDefault2": {
			default2Schema, default2Data, `{ f(i:0) }`, "",
			JsonObject{"f": "0ijk"},
		},
		"InputArg": {
			inputArgSchema, inputArgData, `{ inputQuery(param: {field: \"55\"}) }`, "",
			JsonObject{"inputQuery": 55.0},
		},
		"InputArg2": {
			inputArg2FieldSchema, inputArg2FieldData, `{ q(p: {s: \"a\", f: 1.25}) }`, "",
			JsonObject{"q": "a1.25"},
		},
		"SliceArg": {
			listArgSchema, sliceArgData, `{ listQuery(list: [1, 2, 3]) }`, "",
			JsonObject{"listQuery": 3.0},
		},
		"ArrayArg": {
			listArgSchema, arrayArgData, `{ listQuery(list: [1, 2, 3]) }`, "",
			JsonObject{"listQuery": 3.0},
		},

		// Resolvers with variable arguments
		"VarInt": {
			argsSchema, paramData, `query Test($value: Int!) {dbl(v: $value)}`, `{"value": -2}`,
			JsonObject{"dbl": float64(-4)},
		},
		"VarObject": {
			inputArgSchema, inputArgData, `query Test($t: inputType!) {inputQuery(param: $t)}`,
			`{ "t": {"field": "66"} }`,
			JsonObject{"inputQuery": float64(66)},
		},
		"VarObject2": {
			inputArg2FieldSchema, inputArg2FieldData, `query Test($t2: R!) {q(p: $t2)}`,
			`{ "t2": {"s": "bbb", "f": 2.5} }`,
			JsonObject{"q": "bbb2.5"},
		},

		"Alias": {
			argsSchema, paramData, `{ two: dbl(v: 1) six: dbl(v: 3) }`, "",
			JsonObject{"two": 2.0, "six": 6.0},
		},
		"Fragment2Uses": {
			nestedSchema, nestedData, `{n1: n {...f} n2: n {...f}} fragment f on N {p}`, "",
			JsonObject{"n1": JsonObject{"p": true}, "n2": JsonObject{"p": true}},
		},
		"Fragment2Fields": {
			nestedSchema, nestedData, `{n {...f}} fragment f on N {p q}`, "",
			JsonObject{"n": JsonObject{"p": true, "q": false}},
		},
		"Interface": {
			interfaceSchema, interfaceData, `{ a { x1 e } }`, "",
			JsonObject{"a": JsonObject{"x1": 4.0, "e": "fff"}},
		},
		"InterfaceFunc": {
			interfaceSchema, interfaceFunc, `{ a { x1 e } }`, "",
			JsonObject{"a": JsonObject{"x1": 5.0, "e": "ggg"}},
		},
		"InlineFrag": {
			interfaceSchema, inlineFragFunc, `{ a { ... on D { e } } }`, "",
			JsonObject{"a": JsonObject{"e": "e in D"}},
		},
		"InlineFrag2Fields": {
			interfaceSchema, inlineFragFunc, `{ a { ... on D { x1 e } } }`, "",
			JsonObject{"a": JsonObject{"x1": 1.0, "e": "e in D"}},
		},
		"Union1": {
			"type Query { a: U! } type U1 { v: Int! } union U = U1",
			struct {
				_ [0]U1 // we need to declare this so the handler knows U1 type (as A returns it as an interface{})
				A interface{}
			}{A: U1{V: 87}}, `{ a { ... on U1 { v } } }`, "",
			JsonObject{"a": JsonObject{"v": 87.0}},
		},
		"Union2": {
			"type Query { b: U! } type U1 { v: Int! } type U2 { v: Int! w: String!} union U = U1|U2",
			struct {
				_ [0]U2
				B interface{}
			}{B: U2{W: "U2 w"}}, `{b{... on U1{v} ... on U2{w}}}`, "",
			JsonObject{"b": JsonObject{"w": "U2 w"}},
		},
		"Union3": {
			union3Schema, struct {
				_ [0]U1
				_ [0]U2
				C []interface{}
			}{C: []interface{}{U1{V: 6}, U2{V: 7}}},
			`{c{... on U1{v} ... on U2{v}}}`, "",
			JsonObject{"c": []interface{}{JsonObject{"v": 6.0}, JsonObject{"v": 7.0}}},
		},
		"Union4": {
			union3Schema, struct {
				_ [0]U1
				_ [0]U2
				C []interface{}
			}{C: []interface{}{U1{V: 1}, U2{V: 2, W: "w"}, U1{V: 3}}},
			`{c{... on U1{v} ... on U2{v}}}`, "",
			JsonObject{"c": []interface{}{JsonObject{"v": 1.0}, JsonObject{"v": 2.0}, JsonObject{"v": 3.0}}},
		},

		"Context0": {
			"type Query { value: Int! }", contextFunc, `{ value }`, "",
			JsonObject{"value": float64(100)},
		},
		"Context1": {
			argsSchema, contextFunc1, `{ dbl(v:1) }`, "",
			JsonObject{"dbl": float64(102)},
		},
		"Context2": {
			args2Schema, contextFunc2, `{ f(i:3,s:\"abc\") }`, "",
			JsonObject{"f": "3abc"},
		},
		// Note that we can't pass parRef by value (must use pointer) since parRef.value has not been set yet
		"ParRef": {
			"type Query { value: Int! }", &parRef, `{ value }`, "",
			JsonObject{"value": float64(42)},
		},
		"SubscriptSlice0": {
			subscriptSlice, subscript, `{ slice(id:0) }`, "",
			JsonObject{"slice": "zero"},
		},
		"SubscriptSlice1": {
			subscriptSlice, subscript, `{ slice(id:1) }`, "",
			JsonObject{"slice": ""},
		},
		"SubscriptSlice2": {
			subscriptSlice, subscript, `{ slice(id:2) }`, "",
			JsonObject{"slice": "two"},
		},
		"SubscriptMap": {
			subscriptMap, subscript, `{ map(number:\"pi\") }`, "",
			JsonObject{"map": 3.14159265359},
		},
		"SliceFieldID": {
			sliceFieldSchema, sliceFieldID, `{ s { id b } }`, "",
			JsonObject{"s": []interface{}{JsonObject{"id": 0.0, "b": 11.0}, JsonObject{"id": 1.0, "b": 12.0}}},
		},
		"MapFieldID": {
			mapFieldSchema, mapFieldID, `{ m { id } }`, "",
			JsonObject{"m": []interface{}{JsonObject{"id": "a"}}},
		},
		"MapFieldID1": {
			mapFieldSchema, mapFieldID, `{ m { b } }`, "",
			JsonObject{"m": []interface{}{JsonObject{"b": 1.0}}},
		},
		"MapFieldID2": {
			mapFieldSchema, mapFieldID, `{ m { b id } }`, "",
			JsonObject{"m": []interface{}{JsonObject{"b": 1.0, "id": "a"}}},
		},
		"SliceOffsetID": {
			sliceFieldSchema, sliceOffsetID, `{ s { id b } }`, "",
			JsonObject{"s": []interface{}{JsonObject{"id": 100.0, "b": 21.0}, JsonObject{"id": 101.0, "b": 22.0}}},
		},
	}

	// Value stores a closure on the method valueFunc so that it can refer back to field "private" via the receiver
	parRef.Value = parRef.valueFunc

	for name, testData := range happyData {
		t.Run(name, func(t *testing.T) {
			h := handler.New([]string{testData.schema},
				nil,
				[3][]interface{}{
					{testData.data},
					nil,
					nil,
				},
			)

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
			h.ServeHTTP(writer, request) /*****/

			// All of these tests should give status OK
			if writer.Result().StatusCode != http.StatusOK {
				t.Logf("Unexpected response code %d", writer.Code)
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
				return
			}

			// Check that the resulting GraphQL result (error and data)
			Assertf(t, result.Errors == nil, "Expected no error and got %v", result.Errors)
			Assertf(t, reflect.DeepEqual(result.Data, testData.expected), "Expected %v, got %v", testData.expected, result.Data)
		})
	}
}

func Assertf(t *testing.T, succeeded bool, format string, args ...interface{}) {
	const (
		succeed = "\u2713" // tick
		failed  = "XXXXX"  //"\u2717" // cross
	)

	t.Helper()
	if !succeeded {
		t.Errorf("%-6s"+format, append([]interface{}{failed}, args...)...)
	} else {
		t.Logf("%-6s"+format, append([]interface{}{succeed}, args...)...)
	}
}
