package field_test

import (
	"reflect"
	"testing"

	"github.com/andrewwphillips/eggql/internal/field"
)

var testData = map[string]struct {
	in  string
	exp field.Info // Expected results
}{
	"Empty":  {``, field.Info{}},
	"Empty2": {`,`, field.Info{}},
	"Empty3": {`,,`, field.Info{}},
	//"Nullable": {`,nullable`, field.Info{Nullable: true}},
	"All": {
		`a, args(b:d=f,c:e=g)`, field.Info{
			Name: "a", Args: []string{"b", "c"}, Enums: []string{"d", "e"}, Defaults: []string{"f", "g"},
			DescArgs: []string{"", ""},
		},
	},
	"NameOnly":  {`X`, field.Info{Name: "X"}},
	"NameOnly2": {`joe,`, field.Info{Name: "joe"}},
	//"NameNull":  {`joe,nullable`, field.Info{Name: "joe", Nullable: true}},
	"Params0": {
		`,args()`,
		field.Info{Args: []string{}, Enums: []string{}, Defaults: []string{}, DescArgs: []string{}},
	},
	"Params1": {
		`,args(a)`, field.Info{
			Args: []string{"a"}, Enums: []string{""}, Defaults: []string{""}, DescArgs: []string{""},
		},
	},
	"Params2": {
		`,args(abc,d)`, field.Info{
			Args: []string{"abc", "d"}, Enums: []string{"", ""}, Defaults: []string{"", ""}, DescArgs: []string{"", ""},
		},
	},
	"Params3": {
		`,args(abc,"d e",f)`, field.Info{
			Args: []string{"abc", `"d e"`, "f"}, Enums: []string{"", "", ""}, Defaults: []string{"", "", ""},
			DescArgs: []string{"", "", ""},
		},
	},
	"ParamsSpaced": {
		`,args( a , bcd , efg )`, field.Info{
			Args: []string{"a", "bcd", "efg"}, Enums: []string{"", "", ""}, Defaults: []string{"", "", ""},
			DescArgs: []string{"", "", ""},
		},
	},
	"Defaults1": {
		`,args(one=1,2)`, field.Info{
			Args: []string{"one", "2"}, Enums: []string{"", ""}, Defaults: []string{"1", ""},
			DescArgs: []string{"", ""},
		},
	},
	"Defaults2": {
		`,args(one=1,two="number two")`, field.Info{
			Args: []string{"one", "two"}, Enums: []string{"", ""}, Defaults: []string{"1", `"number two"`},
			DescArgs: []string{"", ""},
		},
	},
	"Defaults3": {
		`,args(list=[1,2,4],obj={a:1, b:"two"})`, field.Info{
			Args: []string{"list", "obj"}, Enums: []string{"", ""}, Defaults: []string{"[1,2,4]", `{a:1, b:"two"}`},
			DescArgs: []string{"", ""},
		},
	},
	"Enum": {`unit:Unit`, field.Info{Name: "unit", GQLTypeName: "Unit"}},
	//"EnumNull":        {`unit:Unit,nullable`, field.Info{Name: "unit", GQLTypeName: "Unit", Nullable: true}},
	"EnumDefaultName": {`:A`, field.Info{GQLTypeName: "A"}},
	"EnumParams": {
		`,args(height, unit:Unit)`, field.Info{
			Args: []string{"height", "unit"}, Enums: []string{"", "Unit"}, Defaults: []string{"", ""},
			DescArgs: []string{"", ""},
		},
	},
	"EnumParams2": {
		`,args(h, w, unit:Unit = FOOT)`, field.Info{
			Args: []string{"h", "w", "unit"}, Enums: []string{"", "", "Unit"}, Defaults: []string{"", "", "FOOT"},
			DescArgs: []string{"", "", ""},
		},
	},
	"Subscript":      {`,subscript`, field.Info{Subscript: "id"}},
	"SubscriptEmpty": {`,subscript=`, field.Info{Subscript: "id"}},
	"SubscriptNamed": {`,subscript=idx`, field.Info{Subscript: "idx"}},
	"FieldDesc":      {`# abc`, field.Info{Description: " abc"}},
	"ArgDesc": {
		`,args(a#desc)`, field.Info{
			Args: []string{"a"}, Enums: []string{""}, Defaults: []string{""}, DescArgs: []string{"desc"},
		},
	},

	"AllOptions": {
		`a:b,,args(c:d=e#f,g=h#i i i i),subscript=h#d #d`, // Note that this is invalid at a higher level as you can't use both "args" and "subscript" options together
		field.Info{
			Name: "a", GQLTypeName: "b", Args: []string{"c", "g"}, Enums: []string{"d", ""},
			Defaults: []string{"e", "h"},
			DescArgs: []string{"f", "i i i i"}, Subscript: "h", Description: "d #d",
		},
	},
}

// TestTagInfo checks parsing of graphql options tags (metadata)
func TestGetTagInfo(t *testing.T) {
	for name, data := range testData {
		got, err := field.GetTagInfo(data.in)
		if err != nil {
			Assertf(t, err == nil, "Error    : %12s: expected no error got %v", name, err)
			continue
		}
		Assertf(t, got.Name == data.exp.Name, "Name     : %12s: expected %q got %q", name, data.exp.Name, got.Name)
		if got.GQLTypeName != "" || data.exp.GQLTypeName != "" {
			Assertf(t, got.GQLTypeName == data.exp.GQLTypeName, "TypeName : %12s: expected %q got %q", name, data.exp.GQLTypeName, got.GQLTypeName)
		}
		if got.Args != nil || data.exp.Args != nil {
			Assertf(t, reflect.DeepEqual(got.Args, data.exp.Args), "Args   : %12s: expected %q got %q", name, data.exp.Args, got.Args)
		}
		if got.Enums != nil || data.exp.Enums != nil {
			Assertf(t, reflect.DeepEqual(got.Enums, data.exp.Enums), "Enums    : %12s: expected %q got %q", name, data.exp.Enums, got.Enums)
		}
		if got.Defaults != nil || data.exp.Defaults != nil {
			Assertf(t, reflect.DeepEqual(got.Defaults, data.exp.Defaults), "Defaults : %12s: expected %q got %q", name, data.exp.Defaults, got.Defaults)
		}
		if got.DescArgs != nil || data.exp.DescArgs != nil {
			Assertf(t, reflect.DeepEqual(got.DescArgs, data.exp.DescArgs), "Arg Desc : %12s: expected %q got %q", name, data.exp.DescArgs, got.DescArgs)
		}

		//Assertf(t, got.Nullable == data.exp.Nullable, "Nullable : %12s: expected %v got %v", name, data.exp.Nullable, got.Nullable)
		if got.Subscript != "" || data.exp.Subscript != "" {
			Assertf(t, got.Subscript == data.exp.Subscript, "Subscript: %12s: expected %q got %q", name, data.exp.Subscript, got.Subscript)
		}
		if got.Description != "" || data.exp.Description != "" {
			Assertf(t, got.Description == data.exp.Description, "Descript: %12s: expected %q got %q", name, data.exp.Description, got.Description)
		}
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
