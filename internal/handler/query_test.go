package handler_test

import (
	"context"
	"encoding/json"
	"github.com/andrewwphillips/eggql/internal/handler"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strconv"
	"strings"
	"testing"
)

// Note that the schema strings (below) must closely match the structs (further below).  (In production, this is ensured
// by schema.Build which generates a schema based on a Go struct.)  For example, the stringSchema Query has a single
// String! field called "message" and the corresponding stringData struct has a single string field called
// "Message", where the field name must be capitalised (exported).  Similarly, the funcData struct, which is also
// used with the stringSchema, has a "Message" field, but in this case it is a func() returning string.

const (
	stringSchema           = "type Query { message: String! }"
	namedSchema            = "schema {query: QueryName} type QueryName { b: Int! }"
	intSchema              = "type Query { value: Int! }"
	boolSchema             = "type Query { b : Boolean! }"
	floatSchema            = "type Query { f: Float! }"
	listSchema             = "type Query { values: [Int!] }"
	nestedSchema           = "type Query { n: N! } type N { q: Boolean! p: Boolean! }"
	stringIntSchema        = "type Query { m: String! v: Int! }"
	paramSchema            = "type Query { dbl(v: Int!): Int! }"
	param2ArgSchema        = "type Query { f(i: Int!, s: String!): String! }"
	default1Schema         = "type Query { f(i: Int!, s: String! = \"xyz\"): String! }"
	default2Schema         = "type Query { f(i: Int! = 87, s: String! = \"ijk\"): String! }"
	inputParamSchema       = "type Query { inputQuery(param: inputType!): Int! } input inputType { field: String! }"
	inputParam2FieldSchema = "type Query { q(p: R!): String! } input R{s:String! f:Float!}"
	listParamSchema        = "type Query { listQuery(list: [Int!]!): Int! }"
	interfaceSchema        = "type Query { a: D! } interface X { x1: Int! } type D implements X { x1: Int! e: String! }"
	union1Schema           = "type Query { a: U! } type U1 { v: Int! } union U = U1"
	union2Schema           = "type Query { b: U! } type U1 { v: Int! } type U2 { v: Int! w: String!} union U = U1|U2"
	union3Schema           = "type Query { c: [U] } type U1 { v: Int! } type U2 { v: Int! w: String!} union U = U1|U2"
	subscriptSlice         = "schema {query: QuerySubscript} type QuerySubscript { slice(id: Int!): String! }"
	subscriptMap           = "schema {query: QuerySubscript} type QuerySubscript { map(number: String!): Float! }"
)

type (
	inputParam2FieldType struct {
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

	ParentRef struct {
		private int
		Value   func() int // closure (set to point to ParentRef.valueFunc method below)
	}

	QuerySubscript struct {
		Slice []string           `graphql:",subscript"`
		Map   map[string]float64 `graphql:",subscript=number"`
	}

	U  struct{} // U is embedded in other structs to implement a union
	U1 struct {
		U
		V int
	}
	U2 struct {
		U
		V int
		W string
	}
)

var (
	stringData    = struct{ Message string }{"hello"}
	namedData     = QueryName{B: 'A'}
	intData       = struct{ Value int }{42}
	boolData      = struct{ B bool }{true}
	floatData     = struct{ F float64 }{1.5}
	sliceData     = struct{ Values []int }{[]int{1, 8, 3}}
	arrayData     = struct{ Values [1]int }{[1]int{42}}
	mapData       = struct{ Values map[string]int }{map[string]int{"": 42}}
	real          = 1.73205 // must be var not const so we can take it's address
	ptrData       = struct{ F *float64 }{&real}
	funcData      = struct{ Message func() string }{func() string { return "hi" }}
	nestedData    = struct{ N struct{ P, Q bool } }{struct{ P, Q bool }{true, false}}
	stringIntData = struct {
		M string
		V int
	}{"mmm", 43}
	paramData = struct {
		Dbl func(int) int `graphql:",args(v)"`
	}{func(value int) int { return 2 * value }}
	param2ArgData = struct {
		F func(int, string) string `graphql:",args(i,s)"`
	}{func(i int, s string) string { return s + strconv.Itoa(i) }}
	default1Data = struct {
		F func(int, string) string `graphql:",args(i,s=xyz)"`
	}{func(i int, s string) string { return s + strconv.Itoa(i*2) }}
	default2Data = struct {
		F func(int, string) string `graphql:",args(i=87,s=ijk)"`
	}{func(i int, s string) string { return strconv.Itoa(i) + s }}
	inputParamData = struct {
		InputQuery func(struct{ Field string }) int `graphql:",args(param)"`
	}{func(p struct{ Field string }) int { r, _ := strconv.Atoi(p.Field); return r }}
	inputParam2FieldData = struct {
		Q func(inputParam2FieldType) string `graphql:",args(p)"`
	}{func(parm inputParam2FieldType) string { return parm.S + strconv.FormatFloat(parm.F, 'g', 10, 64) }}
	sliceParamData = struct {
		ListQuery func([]int) int `graphql:",args(list)"`
	}{func(list []int) int { return len(list) }}
	arrayParamData = struct {
		ListQuery func([3]int) int `graphql:",args(list)"`
	}{func(list [3]int) int { return len(list) }}

	interfaceData  = struct{ A D }{D{X{4}, "fff"}}
	interfaceFunc  = struct{ A func() D }{func() D { return D{X{5}, "ggg"} }}
	inlineFragFunc = struct{ A func() interface{} }{func() interface{} { return D{X{1}, "e in D"} }}

	contextFunc  = struct{ Value func(context.Context) int }{func(ctx context.Context) int { return 100 }}
	contextFunc1 = struct {
		Dbl func(context.Context, int) int `graphql:",args(v)"`
	}{func(ctx context.Context, i int) int { return 100 + 2*i }}
	contextFunc2 = struct {
		F func(context.Context, int, string) string `graphql:",args(i,s)"`
	}{func(ctx context.Context, i int, s string) string { return strconv.Itoa(i) + s }}

	parRef = ParentRef{private: 42}

	subscript = QuerySubscript{
		Slice: []string{"zero", "", "two"},
		Map:   map[string]float64{"pi": 3.14159265359, "root2": 1.41421356237},
	}
)

func (p *ParentRef) valueFunc() int {
	return p.private
}

// JsonObject is what json.Unmarshaler produces when it decodes a JSON object.  Not that we use a type alias here,
//   hence the equals sing (=) rather than a type definition otherwise reflect.DeepEqual does not work.
type JsonObject = map[string]interface{}

// HappyData test the "happy" paths (ie no errors)
var happyData = map[string]struct {
	schema    string      // GraphQL schema
	data      interface{} // corresponding matching struct
	query     string      // GraphQL query to send to the handler (query syntax)
	variables string      // GraphQL variables to use with the query (JSON)

	expected interface{} // expected result after decoding the returned JSON
}{
	"String": {stringSchema, stringData, `{ message }`, "",
		JsonObject{"message": "hello"}},
	"NamedQuery": {namedSchema, namedData, `{ b }`, "",
		JsonObject{"b": float64('A')}},
	"Integer": {intSchema, &intData, `{ value }`, "",
		JsonObject{"value": float64(42)}}, // all numbers decode to float64
	"Boolean": {boolSchema, boolData, `{ b }`, "",
		JsonObject{"b": true}},
	"Float": {floatSchema, floatData, `{ f }`, "",
		JsonObject{"f": 1.5}},
	"Slice": {listSchema, sliceData, `{ values }`, "",
		JsonObject{"values": []interface{}{1.0, 8.0, 3.0}}},
	"Array": {listSchema, arrayData, `{ values }`, "",
		JsonObject{"values": []interface{}{42.0}}},
	// TODO: write test for map with more than one element (order of returned map elements is indeterminate)
	"Map": {listSchema, mapData, `{ values }`, "",
		JsonObject{"values": []interface{}{42.0}}},
	"Pointer": {floatSchema, ptrData, `{ f }`, "",
		JsonObject{"f": real}},
	"Func": {stringSchema, funcData, `{ message }`, "",
		JsonObject{"message": "hi"}},
	"Nested": {nestedSchema, nestedData, `{ n { p q } }`, "",
		JsonObject{"n": JsonObject{"p": true, "q": false}}},
	"TwoQueries": {stringIntSchema, stringIntData, `{ m v }`, "",
		JsonObject{"m": "mmm", "v": 43.0}},

	// Resolvers with arguments (inline)
	"ParamInt": {paramSchema, paramData, `{ dbl(v: 21) }`, "",
		JsonObject{"dbl": 42.0}},
	"Param2": {param2ArgSchema, param2ArgData, `{ f(i:7, s:\"abc\") }`, "",
		JsonObject{"f": "abc7"}},
	"Default1": {default1Schema, default1Data, `{ f(i:7) }`, "",
		JsonObject{"f": "xyz14"}},
	"NoDefault1": {default1Schema, default1Data, `{ f(i:8, s:\"ABC\") }`, "",
		JsonObject{"f": "ABC16"}},
	"Default2": {default2Schema, default2Data, `{ f }`, "",
		JsonObject{"f": "87ijk"}},
	"NoDefault2": {default2Schema, default2Data, `{ f(i:99, s:\"IJK\") }`, "",
		JsonObject{"f": "99IJK"}},
	"FirstDefault2": {default2Schema, default2Data, `{ f(s:\"\") }`, "",
		JsonObject{"f": "87"}},
	"SecondDefault2": {default2Schema, default2Data, `{ f(i:0) }`, "",
		JsonObject{"f": "0ijk"}},
	"InputParam": {inputParamSchema, inputParamData, `{ inputQuery(param: {field: \"55\"}) }`, "",
		JsonObject{"inputQuery": 55.0}},
	"InputParam2": {inputParam2FieldSchema, inputParam2FieldData, `{ q(p: {s: \"a\", f: 1.25}) }`, "",
		JsonObject{"q": "a1.25"}},
	"SliceParam": {listParamSchema, sliceParamData, `{ listQuery(list: [1, 2, 3]) }`, "",
		JsonObject{"listQuery": 3.0}},
	"ArrayParam": {listParamSchema, arrayParamData, `{ listQuery(list: [1, 2, 3]) }`, "",
		JsonObject{"listQuery": 3.0}},

	// Resolvers with variable arguments
	"VarInt": {paramSchema, paramData, `query Test($value: Int!) {dbl(v: $value)}`, `{"value": -2}`,
		JsonObject{"dbl": float64(-4)}},
	"VarObject": {inputParamSchema, inputParamData, `query Test($t: inputType!) {inputQuery(param: $t)}`,
		`{ "t": {"field": "66"} }`,
		JsonObject{"inputQuery": float64(66)}},
	"VarObject2": {inputParam2FieldSchema, inputParam2FieldData, `query Test($t2: R!) {q(p: $t2)}`,
		`{ "t2": {"s": "bbb", "f": 2.5} }`,
		JsonObject{"q": "bbb2.5"}},

	"Alias": {paramSchema, paramData, `{ two: dbl(v: 1) six: dbl(v: 3) }`, "",
		JsonObject{"two": 2.0, "six": 6.0}},
	"Fragment2Uses": {nestedSchema, nestedData, `{n1: n {...f} n2: n {...f}} fragment f on N {p}`, "",
		JsonObject{"n1": JsonObject{"p": true}, "n2": JsonObject{"p": true}}},
	"Fragment2Fields": {nestedSchema, nestedData, `{n {...f}} fragment f on N {p q}`, "",
		JsonObject{"n": JsonObject{"p": true, "q": false}}},
	"Interface": {interfaceSchema, interfaceData, `{ a { x1 e } }`, "",
		JsonObject{"a": JsonObject{"x1": 4.0, "e": "fff"}}},
	"InterfaceFunc": {interfaceSchema, interfaceFunc, `{ a { x1 e } }`, "",
		JsonObject{"a": JsonObject{"x1": 5.0, "e": "ggg"}}},
	"InlineFrag": {interfaceSchema, inlineFragFunc, `{ a { ... on D { e } } }`, "",
		JsonObject{"a": JsonObject{"e": "e in D"}}},
	"InlineFrag2Fields": {interfaceSchema, inlineFragFunc, `{ a { ... on D { x1 e } } }`, "",
		JsonObject{"a": JsonObject{"x1": 1.0, "e": "e in D"}}},
	"Union1": {union1Schema, struct{ A interface{} }{U1{V: 87}}, `{ a { ... on U1 { v } } }`, "",
		JsonObject{"a": JsonObject{"v": 87.0}}},
	"Union2": {union2Schema, struct{ B interface{} }{U2{W: "U2 w"}}, `{b{... on U1{v} ... on U2{w}}}`, "",
		JsonObject{"b": JsonObject{"w": "U2 w"}}},
	"Union3": {union3Schema, struct{ C []interface{} }{C: []interface{}{U1{V: 6}, U2{V: 7}}},
		`{c{... on U1{v} ... on U2{v}}}`, "",
		JsonObject{"c": []interface{}{JsonObject{"v": 6.0}, JsonObject{"v": 7.0}}}},
	"Union4": {union3Schema, struct{ C []interface{} }{C: []interface{}{U1{V: 1}, U2{V: 2, W: "w"}, U1{V: 3}}},
		`{c{... on U1{v} ... on U2{v}}}`, "",
		JsonObject{"c": []interface{}{JsonObject{"v": 1.0}, JsonObject{"v": 2.0}, JsonObject{"v": 3.0}}}},

	"Context0": {intSchema, contextFunc, `{ value }`, "",
		JsonObject{"value": float64(100)}},
	"Context1": {paramSchema, contextFunc1, `{ dbl(v:1) }`, "",
		JsonObject{"dbl": float64(102)}},
	"Context2": {param2ArgSchema, contextFunc2, `{ f(i:3,s:\"abc\") }`, "",
		JsonObject{"f": "3abc"}},
	// Note that we can't pass parRef by value (must use pointer) since parRef.value has not been set yet
	"ParRef": {intSchema, &parRef, `{ value }`, "",
		JsonObject{"value": float64(42)}},
	"SubscriptSlice0": {subscriptSlice, subscript, `{ slice(id:0) }`, "",
		JsonObject{"slice": "zero"}},
	"SubscriptSlice1": {subscriptSlice, subscript, `{ slice(id:1) }`, "",
		JsonObject{"slice": ""}},
	"SubscriptSlice2": {subscriptSlice, subscript, `{ slice(id:2) }`, "",
		JsonObject{"slice": "two"}},
	"SubscriptMap": {subscriptMap, subscript, `{ map(number:\"pi\") }`, "",
		JsonObject{"map": 3.14159265359}},
}

func TestQuery(t *testing.T) {
	// Value stores a closure on the method valueFunc so that it can refer back to field "private" via the receiver
	parRef.Value = parRef.valueFunc

	for name, testData := range happyData {
		//log.Println(name) // we only need this if a test panics - to see which one it was
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

func Assertf(t *testing.T, succeeded bool, format string, args ...interface{}) {
	const (
		succeed = "\u2713" // tick
		failed  = "X"      //"\u2717" // cross
	)

	t.Helper()
	if !succeeded {
		t.Errorf("%s\t"+format, append([]interface{}{failed}, args...)...)
	} else {
		t.Logf("%s\t"+format, append([]interface{}{succeed}, args...)...)
	}
}
