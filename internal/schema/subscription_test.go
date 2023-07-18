package schema_test

import (
	"strconv"
	"testing"

	"github.com/andrewwphillips/eggql/internal/schema"
)

func TestBuildSubscription(t *testing.T) {
	testData := map[string]struct {
		subscription interface{}
		expected     string
	}{
		"empty": {struct{}{}, `type Subscription{ }`},
		"int":   {struct{ I <-chan int }{}, `type Subscription{ i: Int! }`},
		"two":   {struct{ I, J <-chan int }{}, `type Subscription{ i: Int! j:Int! }`},
		"twodiff": {
			struct {
				I <-chan int
				S <-chan string
			}{}, `type Subscription{ i: Int! s:String! }`,
		},
		"list": {struct{ I <-chan []int }{}, `type Subscription{ i: [Int!]! }`},

		"func":     {struct{ F func() <-chan int }{}, `type Subscription{ f: Int! }`},
		"funcList": {struct{ F func() <-chan []int }{}, `type Subscription{ f: [Int!]! }`},
	}

	for name, data := range testData {
		data := data
		t.Run(name, func(t *testing.T) {
			exp := RemoveWhiteSpace(t, data.expected)
			got := RemoveWhiteSpace(t, schema.MustBuild(nil, nil, data.subscription))
			same, where := got == exp, ""
			if !same {
				// Failing case - find the offset of the first different byte to help debug where the problem is
				for i := range got {
					if i >= len(exp) || got[i] != exp[i] {
						where = "\nwhere first difference is at character " + strconv.Itoa(i) + " of " + strconv.Itoa(len(exp))
						break
					}
				}
			}

			Assertf(t, same, "TestBuildSubscription: %12s: make schema expected %q got %q%s", name, exp, got, where)
		})
	}
}
