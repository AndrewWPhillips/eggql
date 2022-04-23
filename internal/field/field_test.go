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
			Name: "a", Args: []string{"b", "c"}, ArgTypes: []string{"d", "e"}, ArgDefaults: []string{"f", "g"},
			ArgDescriptions: []string{"", ""},
		},
	},
	"NameOnly":  {`X`, field.Info{Name: "X"}},
	"NameOnly2": {`joe,`, field.Info{Name: "joe"}},
	//"NameNull":  {`joe,nullable`, field.Info{Name: "joe", Nullable: true}},
	"Params0": {
		`,args()`,
		field.Info{Args: []string{}, ArgTypes: []string{}, ArgDefaults: []string{}, ArgDescriptions: []string{}},
	},
	"Params1": {
		`,args(a)`, field.Info{
			Args: []string{"a"}, ArgTypes: []string{""}, ArgDefaults: []string{""}, ArgDescriptions: []string{""},
		},
	},
	"Params2": {
		`,args(abc,d)`, field.Info{
			Args: []string{"abc", "d"}, ArgTypes: []string{"", ""}, ArgDefaults: []string{"", ""},
			ArgDescriptions: []string{"", ""},
		},
	},
	"Params3": {
		`,args(abc,"d e",f)`, field.Info{
			Args: []string{"abc", `"d e"`, "f"}, ArgTypes: []string{"", "", ""}, ArgDefaults: []string{"", "", ""},
			ArgDescriptions: []string{"", "", ""},
		},
	},
	"ParamsSpaced": {
		`,args( a , bcd , efg )`, field.Info{
			Args: []string{"a", "bcd", "efg"}, ArgTypes: []string{"", "", ""}, ArgDefaults: []string{"", "", ""},
			ArgDescriptions: []string{"", "", ""},
		},
	},
	"Defaults1": {
		`,args(one=1,2)`, field.Info{
			Args: []string{"one", "2"}, ArgTypes: []string{"", ""}, ArgDefaults: []string{"1", ""},
			ArgDescriptions: []string{"", ""},
		},
	},
	"Defaults2": {
		`,args(one=1,two="number two")`, field.Info{
			Args: []string{"one", "two"}, ArgTypes: []string{"", ""}, ArgDefaults: []string{"1", `"number two"`},
			ArgDescriptions: []string{"", ""},
		},
	},
	"Defaults3": {
		`,args(list=[1,2,4],obj={a:1, b:"two"})`, field.Info{
			Args: []string{"list", "obj"}, ArgTypes: []string{"", ""},
			ArgDefaults:     []string{"[1,2,4]", `{a:1, b:"two"}`},
			ArgDescriptions: []string{"", ""},
		},
	},
	"Enum": {`unit:Unit`, field.Info{Name: "unit", GQLTypeName: "Unit"}},
	//"EnumNull":        {`unit:Unit,nullable`, field.Info{Name: "unit", GQLTypeName: "Unit", Nullable: true}},
	"EnumDefaultName": {`:A`, field.Info{GQLTypeName: "A"}},
	"EnumParams": {
		`,args(height, unit:Unit)`, field.Info{
			Args: []string{"height", "unit"}, ArgTypes: []string{"", "Unit"}, ArgDefaults: []string{"", ""},
			ArgDescriptions: []string{"", ""},
		},
	},
	"EnumParams2": {
		`,args(h, w, unit:Unit = FOOT)`, field.Info{
			Args: []string{"h", "w", "unit"}, ArgTypes: []string{"", "", "Unit"}, ArgDefaults: []string{"", "", "FOOT"},
			ArgDescriptions: []string{"", "", ""},
		},
	},
	"Subscript":      {`,subscript`, field.Info{Subscript: "id"}},
	"SubscriptEmpty": {`,subscript=`, field.Info{Subscript: "id"}},
	"SubscriptNamed": {`,subscript=idx`, field.Info{Subscript: "idx"}},
	"FieldDesc":      {`# abc`, field.Info{Description: " abc"}},
	"ArgDesc": {
		`,args(a#desc)`, field.Info{
			Args: []string{"a"}, ArgTypes: []string{""}, ArgDefaults: []string{""}, ArgDescriptions: []string{"desc"},
		},
	},

	"AllOptions": {
		`a:b,,args(c:d=e#f,g=h#i i i i),subscript=h#d #d`, // Note that this is invalid at a higher level as you can't use both "args" and "subscript" options together
		field.Info{
			Name: "a", GQLTypeName: "b", Args: []string{"c", "g"}, ArgTypes: []string{"d", ""},
			ArgDefaults:     []string{"e", "h"},
			ArgDescriptions: []string{"f", "i i i i"}, Subscript: "h", Description: "d #d",
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
		if got.ArgTypes != nil || data.exp.ArgTypes != nil {
			Assertf(t, reflect.DeepEqual(got.ArgTypes, data.exp.ArgTypes), "Enums    : %12s: expected %q got %q", name, data.exp.ArgTypes, got.ArgTypes)
		}
		if got.ArgDefaults != nil || data.exp.ArgDefaults != nil {
			Assertf(t, reflect.DeepEqual(got.ArgDefaults, data.exp.ArgDefaults), "Defaults : %12s: expected %q got %q", name, data.exp.ArgDefaults, got.ArgDefaults)
		}
		if got.ArgDescriptions != nil || data.exp.ArgDescriptions != nil {
			Assertf(t, reflect.DeepEqual(got.ArgDescriptions, data.exp.ArgDescriptions), "Arg Desc : %12s: expected %q got %q", name, data.exp.ArgDescriptions, got.ArgDescriptions)
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
