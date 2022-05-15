package handler_test

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/andrewwphillips/eggql/internal/handler"
)

// BenchmarkQuery is used to benchmark GraphQL queries to see if code changes have improved performance
// TODO: see if perf. is improved by (1) avoid linear field search (2) result cache (3) ?
func BenchmarkQuery(b *testing.B) {
	const query = `{ "Query": "{ value }" }`

	// ~71 microsec, 154 allocs @ 2022/04/20
	//h := handler.New("type Query { value: String! }", struct{ Value string }{"hello"})

	// ~90microsec, 180 allocs @ 2022/04/20
	//h := handler.New("type Query { value(low:Int!=1 high:Int!=6): Int! }",
	//	struct {
	//		Value func(int, int) int `egg:"(low=1,high=6)"`
	//	}{
	//		Value: func(low, high int) int {
	//			return low + rand.Intn(high+1-low)
	//		},
	//	},
	//)

	// ~71 microsec 154 allocs @ 2022/04/20
	//h := handler.New("type Query { value: Int! }", struct{ Value int }{42})

	// ~111microsec, 284 allocs => cf above (1 field vs 27 fields) 40 microsec slower due to linear search @ 2022/04/20
	h := handler.New("type Query { value: Int! }",
		struct {
			A, B, C, D, E, F, G, H, I, J, K, L, M, N, O, P, Q, R, S, T, U, V, W, X, Y, Z string
			Value                                                                        int
		}{Value: 42})

	body := strings.NewReader(query)
	request := httptest.NewRequest("POST", "/", body)
	request.Header.Add("Content-Type", "application/json")
	writer := httptest.NewRecorder()

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		h.ServeHTTP(writer, request)

		// Note: I tried wrapping the code below in b.Stop/StartTime but it *slowed* the times!?!
		if !strings.Contains(writer.Body.String(), `"data":{"value":`) {
			b.Error("GraphQL query failed:\n", writer.Result().StatusCode, writer.Body.String())
		}
		body.Reset(query)
		writer.Body.Reset()
	}
}
