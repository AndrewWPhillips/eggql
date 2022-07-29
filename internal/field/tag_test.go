package field_test

import (
	"reflect"
	"testing"

	"github.com/andrewwphillips/eggql/internal/field"
)

func TestSplitNested(t *testing.T) {
	splitNestedData := map[string]struct {
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
		"ArgsOption":    {`, args(list=[1,3,6],obj={a:""}) `, []string{``, `args(list=[1,3,6],obj={a:""})`}, ""},

		"Desc0":        {`# abc`, []string{""}, " abc"},
		"Desc1":        {`,# abc`, []string{"", ""}, " abc"},
		"Desc2":        {`,z# abc`, []string{"", "z"}, " abc"},
		"Desc3":        {`#"# abc`, []string{``}, `"# abc`},
		"DescString":   {`"#"# abc`, []string{`"#"`}, " abc"}, // first # is in quotes, so 2nd # starts desc.
		"DescBrackets": {`(#)# abc`, []string{`(#)`}, " abc"}, // # in brackets
		"NoDescString": {`"a#b"`, []string{`"a#b"`}, ""},      // # in string but none at end
		"DescWithArg":  {`, args(list=[1,3,6]#arg1)# abc`, []string{``, `args(list=[1,3,6]#arg1)`}, " abc"},
		"NoDescArgs": {
			`, args(list=[1,3,6]#arg1,obj={a:""}#arg2) `,
			[]string{``, `args(list=[1,3,6]#arg1,obj={a:""}#arg2)`}, "",
		},
	}
	for name, data := range splitNestedData {
		t.Run(name, func(t *testing.T) {
			got, desc, err := field.SplitWithDesc(data.in)
			Assertf(t, err == nil, "Error: expected no error got %v", err)
			Assertf(t, reflect.DeepEqual(got, data.exp), "Name : expected %q got %q", data.exp, got)
			if data.desc != "" && desc != "" {
				Assertf(t, desc == data.desc, "Desc : expected %q got %q", data.desc, desc)
			}
		})
	}
}

func TestSplitArgs(t *testing.T) {
	splitArgsData := map[string]struct {
		in  string
		exp []string
	}{
		"Empty":         {"", []string{""}},
		"DoubleEmpty":   {",", []string{"", ""}},
		"One":           {"a", []string{"a"}},
		"OneSpace":      {" a ", []string{"a"}},
		"OneLong":       {"abdecfeghijklmnopqrstuvwxyz", []string{"abdecfeghijklmnopqrstuvwxyz"}},
		"OneQuotes":     {`"a" `, []string{`"a"`}},
		"Brackets":      {"(a)", []string{"(a)"}},
		"BracketNested": {"a(b(c), d), e(f)", []string{"a(b(c), d)", "e(f)"}},
		"WithDefaults":  {`list=[1,3,6],obj={a:"("}`, []string{`list=[1,3,6]`, `obj={a:"("}`}},
		"WithDesc": {
			`list=[1,3,6]#arg1,obj={a:"][][["}#arg2`,
			[]string{`list=[1,3,6]#arg1`, `obj={a:"][][["}#arg2`},
		},

		"Desc0": {`# abc`, []string{"# abc"}},
		"Desc1": {`,# abc`, []string{"", "# abc"}},
		"Desc2": {`,z# abc`, []string{"", "z# abc"}},
		"Desc3": {`"#"# abc`, []string{`"#"# abc`}},
	}
	for name, data := range splitArgsData {
		t.Run(name, func(t *testing.T) {
			got, err := field.SplitArgs(data.in)
			Assertf(t, err == nil, "Error: expected no error got %v", err)
			Assertf(t, reflect.DeepEqual(got, data.exp), "Name : expected %q got %q", data.exp, got)
		})
	}
}
