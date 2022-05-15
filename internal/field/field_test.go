package field_test

import (
	"reflect"
	"testing"

	"github.com/andrewwphillips/eggql/internal/field"
)

// TestTagInfo checks parsing of graphql options tags (metadata)
func TestGetTagInfo(t *testing.T) {
	testData := map[string]struct {
		in  string
		exp field.Info // Expected results
	}{
		"Empty":    {``, field.Info{}},
		"Empty2":   {`,`, field.Info{}},
		"Empty3":   {`,,`, field.Info{}},
		"Nullable": {`,nullable`, field.Info{Nullable: true}},
		"All": {
			`a(b:d=f,c:e=g)`, field.Info{
				Name: "a", Args: []string{"b", "c"}, ArgTypes: []string{"d", "e"}, ArgDefaults: []string{"f", "g"},
				ArgDescriptions: []string{"", ""},
			},
		},
		"NameOnly":  {`X`, field.Info{Name: "X"}},
		"NameOnly2": {`joe,`, field.Info{Name: "joe"}},
		//"NameNull":  {`joe,nullable`, field.Info{Name: "joe", Nullable: true}},
		"Params0": {
			`()`,
			field.Info{Args: []string{}, ArgTypes: []string{}, ArgDefaults: []string{}, ArgDescriptions: []string{}},
		},
		"Params1": {
			`(a)`, field.Info{
				Args: []string{"a"}, ArgTypes: []string{""}, ArgDefaults: []string{""}, ArgDescriptions: []string{""},
			},
		},
		"Params2": {
			`(abc,d)`, field.Info{
				Args: []string{"abc", "d"}, ArgTypes: []string{"", ""}, ArgDefaults: []string{"", ""},
				ArgDescriptions: []string{"", ""},
			},
		},
		"Params3": {
			`(abc,"d e",f)`, field.Info{
				Args: []string{"abc", `"d e"`, "f"}, ArgTypes: []string{"", "", ""}, ArgDefaults: []string{"", "", ""},
				ArgDescriptions: []string{"", "", ""},
			},
		},
		"ParamsSpaced": {
			`( a , bcd , efg )`, field.Info{
				Args: []string{"a", "bcd", "efg"}, ArgTypes: []string{"", "", ""}, ArgDefaults: []string{"", "", ""},
				ArgDescriptions: []string{"", "", ""},
			},
		},
		"Defaults1": {
			`(one=1,2)`, field.Info{
				Args: []string{"one", "2"}, ArgTypes: []string{"", ""}, ArgDefaults: []string{"1", ""},
				ArgDescriptions: []string{"", ""},
			},
		},
		"Defaults2": {
			`(one=1,two="number two")`, field.Info{
				Args: []string{"one", "two"}, ArgTypes: []string{"", ""}, ArgDefaults: []string{"1", `"number two"`},
				ArgDescriptions: []string{"", ""},
			},
		},
		"Defaults3": {
			`(list=[1,2,4],obj={a:1, b:"two"})`, field.Info{
				Args: []string{"list", "obj"}, ArgTypes: []string{"", ""},
				ArgDefaults:     []string{"[1,2,4]", `{a:1, b:"two"}`},
				ArgDescriptions: []string{"", ""},
			},
		},
		"Enum": {`unit:Unit`, field.Info{Name: "unit", GQLTypeName: "Unit"}},
		//"EnumNull":        {`unit:Unit,nullable`, field.Info{Name: "unit", GQLTypeName: "Unit", Nullable: true}},
		"EnumDefaultName": {`:A`, field.Info{GQLTypeName: "A"}},
		"EnumParams": {
			`(height, unit:Unit)`, field.Info{
				Args: []string{"height", "unit"}, ArgTypes: []string{"", "Unit"}, ArgDefaults: []string{"", ""},
				ArgDescriptions: []string{"", ""},
			},
		},
		"EnumParams2": {
			`(h, w, unit:Unit = FOOT)`, field.Info{
				Args: []string{"h", "w", "unit"}, ArgTypes: []string{"", "", "Unit"},
				ArgDefaults:     []string{"", "", "FOOT"},
				ArgDescriptions: []string{"", "", ""},
			},
		},
		"Subscript": {`,subscript`, field.Info{Subscript: "id"}},
		//"SubscriptEmpty": {`,subscript=`, field.Info{Subscript: "id"}},  // now an error since it could catch a typo
		"SubscriptNamed": {`,subscript=idx`, field.Info{Subscript: "idx"}},
		"FieldDesc":      {`# abc`, field.Info{Description: " abc"}},
		"ArgDesc": {
			`(a#desc)`, field.Info{
				Args: []string{"a"}, ArgTypes: []string{""}, ArgDefaults: []string{""},
				ArgDescriptions: []string{"desc"},
			},
		},

		"AllOptions": {
			`a(c:d=e#f,g=h#i i i i):b,,,subscript=h#d #d`, // Note that this is invalid at a higher level as you can't use both "args" and "subscript" options together
			field.Info{
				Name: "a", GQLTypeName: "b", Args: []string{"c", "g"}, ArgTypes: []string{"d", ""},
				ArgDefaults:     []string{"e", "h"},
				ArgDescriptions: []string{"f", "i i i i"}, Subscript: "h", Description: "d #d",
			},
		},
	}

	for name, data := range testData {
		t.Run(name, func(t *testing.T) {
			got, err := field.GetInfoFromTag(data.in)
			if err != nil {
				Assertf(t, err == nil, "Error    : expected no error got %v", err)
				return
			}
			Assertf(t, got.Name == data.exp.Name, "Name     : expected %q got %q", data.exp.Name, got.Name)
			if got.GQLTypeName != "" || data.exp.GQLTypeName != "" {
				Assertf(t, got.GQLTypeName == data.exp.GQLTypeName, "TypeName : expected %q got %q", data.exp.GQLTypeName, got.GQLTypeName)
			}
			if got.Args != nil || data.exp.Args != nil {
				Assertf(t, reflect.DeepEqual(got.Args, data.exp.Args), "Args   : expected %q got %q", data.exp.Args, got.Args)
			}
			if got.ArgTypes != nil || data.exp.ArgTypes != nil {
				Assertf(t, reflect.DeepEqual(got.ArgTypes, data.exp.ArgTypes), "Enums    : expected %q got %q", data.exp.ArgTypes, got.ArgTypes)
			}
			if got.ArgDefaults != nil || data.exp.ArgDefaults != nil {
				Assertf(t, reflect.DeepEqual(got.ArgDefaults, data.exp.ArgDefaults), "Defaults : expected %q got %q", data.exp.ArgDefaults, got.ArgDefaults)
			}
			if got.ArgDescriptions != nil || data.exp.ArgDescriptions != nil {
				Assertf(t, reflect.DeepEqual(got.ArgDescriptions, data.exp.ArgDescriptions), "Arg Desc : expected %q got %q", data.exp.ArgDescriptions, got.ArgDescriptions)
			}

			Assertf(t, got.Nullable == data.exp.Nullable, "Nullable : expected %v got %v", data.exp.Nullable, got.Nullable)
			if got.Subscript != "" || data.exp.Subscript != "" {
				Assertf(t, got.Subscript == data.exp.Subscript, "Subscript: expected %q got %q", data.exp.Subscript, got.Subscript)
			}
			if got.Description != "" || data.exp.Description != "" {
				Assertf(t, got.Description == data.exp.Description, "Descript: expected %q got %q", data.exp.Description, got.Description)
			}
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
