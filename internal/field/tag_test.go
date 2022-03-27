package field_test

import (
	"github.com/andrewwphillips/eggql/internal/field"
	"reflect"
	"testing"
)

var splitData = map[string]struct {
	in   string
	exp  []string
	desc string
}{
	"Empty":         {"", []string{""}, ""},
	"DoubleEmpty":   {",", []string{"", ""}, ""},
	"One":           {"a", []string{"a"}, ""},
	"OneSpace":      {" a ", []string{"a"}, ""},
	"OneLong":       {"abdecfeghijklmnopqrstuvwxyz", []string{"abdecfeghijklmnopqrstuvwxyz"}, ""},
	"OneQuotes":     {`"a" `, []string{`"a"`}, ""},
	"BracketP0":     {"a( )", []string{"a( )"}, ""},
	"BracketP1":     {" a(b)", []string{"a(b)"}, ""},
	"BracketP2":     {"a(b,c)", []string{"a(b,c)"}, ""},
	"BracketP3":     {"a(b,c,def)", []string{"a(b,c,def)"}, ""},
	"Params2":       {"a(b, c), d(e,f)", []string{"a(b, c)", "d(e,f)"}, ""},
	"Params3":       {"a(b),c,d(e, f)", []string{"a(b)", "c", "d(e, f)"}, ""},
	"Params4":       {"  a,  b,  c,  d(e, f) ", []string{"a", "b", "c", "d(e, f)"}, ""},
	"BracketNested": {"a(b(c), d), e(f)", []string{"a(b(c), d)", "e(f)"}, ""},
	"String":        {`a(b"(c), d), e("f)`, []string{`a(b"(c), d), e("f)`}, ""},
	"String2":       {`"[]]](c), (d), )e(",""`, []string{`"[]]](c), (d), )e("`, `""`}, ""},
	"String3":       {` a("{]}"), b[1,2,3] `, []string{`a("{]}")`, `b[1,2,3]`}, ""},
	"WithDefaults":  {`list=[1,3,6],obj={a:""}`, []string{`list=[1,3,6]`, `obj={a:""}`}, ""},
	"ParamsOption":  {`, args(list=[1,3,6],obj={a:""}) `, []string{``, `args(list=[1,3,6],obj={a:""})`}, ""},

	"Desc0":        {`# abc`, []string{""}, " abc"},
	"Desc1":        {`,# abc`, []string{"", ""}, " abc"},
	"Desc2":        {`,z# abc`, []string{"", "z"}, " abc"},
	"Desc3":        {`#"# abc`, []string{""}, `"# abc`},
	"DescString":   {`"#"# abc`, []string{`"#"`}, " abc"}, // # in string
	"DescBrackets": {`(#)# abc`, []string{`(#)`}, " abc"}, // # in brackets
	"NoDescString": {`"a#b"`, []string{`"a#b"`}, ""},      // # in string but none at end
	"DescWithArg":  {`, args(list=[1,3,6]#arg1)# abc`, []string{``, `args(list=[1,3,6]#arg1)`}, " abc"},
	"NoDescArgs":   {`, args(list=[1,3,6]#arg1,obj={a:""}#arg2) `, []string{``, `args(list=[1,3,6]#arg1,obj={a:""}#arg2)`}, ""},
}

func TestSplit(t *testing.T) {
	for name, data := range splitData {
		got, desc, err := field.SplitNested(data.in)
		Assertf(t, err == nil, "Error: %12s: expected no error got %v", name, err)
		Assertf(t, reflect.DeepEqual(got, data.exp), "Name : %12s: expected %q got %q", name, data.exp, got)
		if data.desc != "" && desc != "" {
			Assertf(t, desc == data.desc, "Desc : %12s: expected %q got %q", name, data.desc, desc)
		}
	}
}
