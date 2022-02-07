package field_test

import (
	"github.com/andrewwphillips/eggql/internal/field"
	"reflect"
	"testing"
)

var testData = map[string]struct {
	in string
	// Expected results
	name     string
	params   []string
	enums    []string
	defaults []string
	enum     string
	nullable bool
}{
	"Empty":           {``, "", nil, nil, nil, "", false},
	"Empty2":          {`,`, "", nil, nil, nil, "", false},
	"Empty3":          {`,,`, "", nil, nil, nil, "", false},
	"Nullable":        {`,nullable`, "", nil, nil, nil, "", true},
	"All":             {`a, args(b:d=f,c:e=g)`, "a", []string{"b", "c"}, []string{"d", "e"}, []string{"f", "g"}, "", false},
	"NameOnly":        {`X`, "X", nil, nil, nil, "", false},
	"NameOnly2":       {`joe,`, "joe", nil, nil, nil, "", false},
	"NameNull":        {`joe,nullable`, "joe", nil, nil, nil, "", true},
	"Params0":         {`,args()`, "", []string{}, []string{}, []string{}, "", false},
	"Params1":         {`,args(a)`, "", []string{"a"}, []string{""}, []string{""}, "", false},
	"Params2":         {`,args(abc,d)`, "", []string{"abc", "d"}, []string{"", ""}, []string{"", ""}, "", false},
	"Params3":         {`,args(abc,"d e",f)`, "", []string{"abc", `"d e"`, "f"}, []string{"", "", ""}, []string{"", "", ""}, "", false},
	"ParamsSpaced":    {`,args( a , bcd , efg )`, "", []string{"a", "bcd", "efg"}, []string{"", "", ""}, []string{"", "", ""}, "", false},
	"Defaults1":       {`,args(one=1,2)`, "", []string{"one", "2"}, []string{"", ""}, []string{"1", ""}, "", false},
	"Defaults2":       {`,args(one=1,two="number two")`, "", []string{"one", "two"}, []string{"", ""}, []string{"1", `"number two"`}, "", false},
	"Defaults3":       {`,args(list=[1,2,4],obj={a:1, b:"two"})`, "", []string{"list", "obj"}, []string{"", ""}, []string{"[1,2,4]", `{a:1, b:"two"}`}, "", false},
	"Enum":            {`unit:Unit`, "unit", nil, nil, nil, "Unit", false},
	"EnumNull":        {`unit:Unit,nullable`, "unit", nil, nil, nil, "Unit", true},
	"EnumDefaultName": {`:A`, "", nil, nil, nil, "A", false},
	"EnumParams":      {`,args(height, unit:Unit)`, "", []string{"height", "unit"}, []string{"", "Unit"}, []string{"", ""}, "", false},
	"EnumParams2":     {`,args(h, w, unit:Unit = FOOT)`, "", []string{"h", "w", "unit"}, []string{"", "", "Unit"}, []string{"", "", "FOOT"}, "", false},
}

func TestGetTagInfo(t *testing.T) {
	for name, data := range testData {
		got, err := field.GetTagInfo(data.in)
		Assertf(t, err == nil, "Error   : %12s: expected no error got %v", name, err)
		Assertf(t, got.Name == data.name, "Name    : %12s: expected %q got %q", name, data.name, got.Name)
		Assertf(t, reflect.DeepEqual(got.Params, data.params), "Params  : %12s: expected %q got %q", name, data.params, got.Params)
		Assertf(t, reflect.DeepEqual(got.Enums, data.enums), "Enums   : %12s: expected %q got %q", name, data.enums, got.Enums)
		Assertf(t, reflect.DeepEqual(got.Defaults, data.defaults), "Defaults: %12s: expected %q got %q", name, data.defaults, got.Defaults)
		Assertf(t, got.Nullable == data.nullable, "Nullable: %12s: expected %v got %v", name, data.nullable, got.Nullable)
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
