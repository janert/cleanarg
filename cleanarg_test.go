package cleanarg

import (
	"testing"

	"reflect"
	"slices"
	"strings"
	"time"
)

func Test_unwrap(t *testing.T) {
	i := 1
	s := struct{}{}

	tests := []struct {
		data      any
		wantError bool
		text      string
	}{
		{i, true, "int"},
		{&i, true, "*int"},
		{s, true, "struct"},
		{&s, false, "*struct"},
	}

	for _, test := range tests {
		_, err := unwrap(test.data)
		if (err != nil) != test.wantError {
			t.Error(test.text)
		}
	}
}

func Test_makeFieldInfoErr(t *testing.T) {

	// Struct with only disallowed types
	s := struct {
		a int64
		b float32
		c struct{}
		d *struct{}
		e *int
	}{}

	v, _ := unwrap(&s)

	for i := 0; i < v.NumField(); i++ {
		f := v.Type().Field(i)

		_, err := makeFieldInfo(f)
		if err == nil {
			t.Errorf("Field %s: expected error", f.Name)
		}
	}
}

func Test_makeFieldInfoTypes(t *testing.T) {

	// Short-hand notation
	f := func(i any) reflect.Type { return reflect.TypeOf(i) }

	tests := []struct {
		data  any
		text  string
		typ   reflect.Type
		isSlc bool
	}{
		{struct{}{}, "empty", f(true), false},
		{struct{ a string }{}, "string", f(""), false},
		{struct{ a []string }{}, "[]string", f(""), true},
		{struct{ i int }{}, "int", f(1), false},
		{struct{ i []int }{}, "[]int", f(1), true},
		{struct{ b bool }{}, "bool", f(true), false},
		{struct{ b []bool }{}, "[]bool", f(true), true},
	}

	for _, test := range tests {
		v := reflect.ValueOf(test.data)

		for i := 0; i < v.NumField(); i++ {
			f := v.Type().Field(i)

			info, err := makeFieldInfo(f)
			if err != nil {
				t.Errorf("%s: Unexpected error %v", test.text, err)
			}

			if test.typ != info.baseType {
				t.Errorf("%s: Type: got=%v want=%v",
					test.text, test.typ, info.baseType)
			}
			if test.isSlc != info.isSlice {
				t.Errorf("%s: Slice: got=%v want=%v",
					test.text, test.isSlc, info.isSlice)
			}
		}
	}
}

func Test_makeFieldInfoTags(t *testing.T) {
	tests := []struct {
		data            any
		help, def, frmt string
	}{
		{struct{}{},
			"", "", ""},
		{struct {
			s int `arg-help:"desc"`
		}{},
			"desc", "", ""},
		{struct {
			s int `arg-default:"desc"`
		}{},
			"", "desc", ""},
		{struct {
			s int `arg-format:"desc""`
		}{},
			"", "", "desc"},
		{struct {
			s int `arg-xxx:"desc"`
		}{},
			"", "", ""},
		{struct {
			s int `arg-help:"desc" arg-format:"fmt"`
		}{},
			"desc", "", "fmt"},
		{struct {
			s int `arg-help:"desc" arg-xxx:"fmt"`
		}{},
			"desc", "", ""},
		{struct {
			s int `arg-help:"desc" arg-flag:"-f"`
		}{},
			"desc", "", ""},
		{struct {
			s int `arg-help:desc`
		}{},
			"", "", ""},
	}

	for _, test := range tests {
		v := reflect.ValueOf(test.data)

		for i := 0; i < v.NumField(); i++ {
			f := v.Type().Field(i)

			info, err := makeFieldInfo(f)
			if err != nil {
				t.Errorf("Unexpected error %v for %v", err, test.data)
			}

			if info.help != test.help || info.defaultval != test.def ||
				info.format != test.frmt {
				t.Errorf("Incorrect tag value for %v", test.data)
			}
		}
	}
}

func Test_extractFlagsSortedOk(t *testing.T) {
	tests := []struct {
		data string
		want []string
	}{
		{"", []string{}},
		{"-b", []string{"-b"}},
		{"+b", []string{"+b"}},
		{"-1", []string{"-1"}},
		{"+1", []string{"+1"}},
		{"--bbb", []string{"--bbb"}},
		{"--111", []string{"--111"}},
		{"--a-b", []string{"--a-b"}},
		{"-b --bbb", []string{"-b", "--bbb"}},
		{"--bbb -b", []string{"-b", "--bbb"}},
		{"-a -b", []string{"-a", "-b"}},
		{"-b -a", []string{"-a", "-b"}},
		{"-a +a", []string{"+a", "-a"}},
	}

	for _, test := range tests {
		got, err := extractFlagsSorted(test.data)

		if err != nil {
			t.Errorf("%s returns error", test.data)
			continue
		}

		if len(got) != len(test.want) {
			t.Errorf("%s: len(got) = %d not equal len(want)=%d",
				test.data, len(got), len(test.want))
			continue
		}

		for i, g := range got {
			if g != test.want[i] {
				t.Errorf("%s: got = %s not equal want = %s",
					test.data, got, test.want)
			}
		}
	}
}

func Test_extractFlagsSortedErr(t *testing.T) {
	tests := []string{"--b", "-bbb", "+bbb", "%", "-$", "--a_b",
		"---", "---a", "---aa"}

	for _, test := range tests {
		_, err := extractFlagsSorted(test)
		if err == nil {
			t.Errorf("%s did not produce error", test)
		}
	}
}

func Test_analyzeStructErr(t *testing.T) {

	tests := []struct {
		data any
		text string
	}{
		{struct{ p *int }{}, "Disallowed member type"},
		{struct{ a struct{} }{}, "Disallowed member type"},
		{struct{ a, b []int }{}, "Two slices"},
		{struct {
			a int `arg-flag:"-f --ff"`
			b int
			c []int
			d int
			e []int
		}{},
			"Flags and two slices"},
	}

	for _, test := range tests {
		v := reflect.ValueOf(test.data)
		_, _, err := analyzeStruct(v)

		if err == nil {
			t.Errorf("%s: Expected error", test.text)
		}
	}
}

func Test_analyzeStructOk(t *testing.T) {

	tests := []struct {
		data      any
		opts, pos int
	}{
		{struct{}{}, 0, 0},
		{struct {
			a int `arg-flag:"-f"`
		}{}, 1, 0},
		{struct {
			a int `arg-flag:"-f --ff"`
		}{}, 2, 0},
		{struct {
			a int `arg-flag:"-f"`
			b int `arg-flag:"-g"`
		}{}, 2, 0},
		{struct {
			a int `arg-flag:"-f --ff"`
			b int `arg-flag:"-g"`
		}{}, 3, 0},
		{struct {
			a int
		}{}, 0, 1},
		{struct {
			a, b int
		}{}, 0, 2},

		// Flags and positionals combined
		{struct {
			a int `arg-flag:"-f"`
			b int
		}{}, 1, 1},
		{struct {
			a int `arg-flag:"-f --ff"`
			b int
		}{}, 2, 1},
		{struct {
			a int `arg-flag:"-f --ff"`
			b int `arg-flag:"-g"`
			c int
		}{}, 3, 1},
		{struct {
			a    int `arg-flag:"-f --ff"`
			b, c int
		}{}, 2, 2},
		{struct {
			a int
			b int `arg-flag:"-f --ff"`
			c int
		}{}, 2, 2},

		// Slices
		{struct {
			a int `arg-flag:"-f --ff"`
			b []int
		}{}, 2, 1},
		{struct {
			a int
			b int `arg-flag:"-f --ff"`
			c []int
		}{}, 2, 2},
		{struct {
			a int `arg-flag:"-f --ff"`
			b int
			c []int
		}{}, 2, 2},
		{struct {
			a int `arg-flag:"-f --ff"`
			b []int
			c int
		}{}, 2, 2},
		{struct {
			a int `arg-flag:"-f --ff"`
			b []int
			c int
		}{}, 2, 2},

		// Ignored fields
		{struct {
			a int `arg-ignore:""`
			b int `arg-flag:"-f --ff"`
			c int
		}{}, 2, 1},
		{struct {
			a int   `arg-ignore:""`
			b int   `arg-flag:"-f --ff"`
			c []int `arg-ignore:""`
			d []int
		}{}, 2, 1},
		{struct {
			a int `arg-ignore:""`
			b int `arg-flag:"-f --ff" arg-ignore:""` // ignore prevails!
			c int
		}{}, 0, 1},
	}

	for j, test := range tests {
		v := reflect.ValueOf(test.data)
		opts, pos, err := analyzeStruct(v)

		if err != nil {
			t.Errorf("%d: Unexpected error: %v", j, err)
		}

		if len(opts) != test.opts {
			t.Errorf("%d: Options: want=%d got=%d", j, test.opts, len(opts))
		}
		if len(pos) != test.pos {
			t.Errorf("%d: Positionals: want=%d got=%d", j, test.pos, len(pos))
		}
	}
}

func Test_populateFromSlice(t *testing.T) {
	// Handle as test for the FromSlice() wrapper functions!
}

func Test_populateDefaultsOk(t *testing.T) {
	s := struct {
		I1 int `arg-flag:"-a" arg-default:"7"`
		I2 int `arg-flag:"-b"`
		I3 int `arg-ignore:""`
		I4 int
		I5 []int `arg-flag:"-c" arg-default:"3"`

		F1 float64 `arg-flag:"-d" arg-default:"9.0"`
		F2 float64 `arg-flag:"-e"`
		F3 float64 `arg-ignore:""`
		F4 float64
		F5 []float64 `arg-flag:"-f" arg-default:"4.0"`

		S1 string `arg-flag:"-g" arg-default:"abc"`
		S2 string `arg-flag:"-h"`
		S3 string `arg-ignore:""`
		S4 string
		S5 []string `arg-flag:"-i" arg-default:"uvw"`

		T1 time.Time `arg-flag:"-j" arg-default:"2025-01-01 11:11:11"`
		T2 time.Time `arg-flag:"-k" arg-default:"2025-01-01" arg-format:"2006-01-02"`
		T3 time.Time `arg-flag:"-l"`
		T4 time.Time `arg-ignore:""`
		T5 time.Time
		T6 []time.Time `arg-flag:"-m" arg-default:"2025-01-01 11:11:11"`
	}{}

	v, _ := unwrap(&s)
	options, _, _ := analyzeStruct(v)

	err := populateDefaults(options, v)

	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}

	if s.I1 != 7 || s.F1 != 9.0 || s.S1 != "abc" {
		t.Errorf("Missing default: %v", s)
	}
	if s.I2 != 0 || s.I3 != 0 || s.I4 != 0 {
		t.Errorf("Missing int zero: %v", s)
	}
	if s.F2 != 0. || s.F3 != 0. || s.F4 != 0. {
		t.Errorf("Missing float zero: %v", s)
	}
	if s.S2 != "" || s.S3 != "" || s.S4 != "" {
		t.Errorf("Missing string zero: %v", s)
	}
	if s.I5 != nil || s.F5 != nil || s.S5 != nil || s.T6 != nil {
		t.Errorf("Missing slice nil: %v", s)
	}

	if s.T1 != time.Date(2025, 1, 1, 11, 11, 11, 0, time.UTC) {
		t.Errorf("Missing default: got=%v", s.T1)
	}
	if s.T2 != time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC) {
		t.Errorf("Missing default: got=%v", s.T2)
	}
	zeroTime := time.Time{}
	if s.T3 != zeroTime || s.T4 != zeroTime || s.T5 != zeroTime {
		t.Errorf("Missing time zero: %v", s)
	}
}

func Test_populateDefaultsErr(t *testing.T) {
	s1 := struct {
		E1 int `arg-flag:"-c" arg-default:"x"`
	}{}

	v1, _ := unwrap(&s1)
	options, _, _ := analyzeStruct(v1)
	if err := populateDefaults(options, v1); err == nil {
		t.Errorf("Expected error int: %v", v1)
	}

	s2 := struct {
		E2 float64 `arg-flag:"-z" arg-default:"x"`
	}{}

	v2, _ := unwrap(&s2)
	options, _, _ = analyzeStruct(v2)
	if err := populateDefaults(options, v2); err == nil {
		t.Errorf("Expected error float: %v", v2)
	}

	s3 := struct {
		E3 time.Time `arg-flag:"-t" arg-default:"2006-01-02"`
	}{}

	v3, _ := unwrap(&s3)
	options, _, _ = analyzeStruct(v3)
	if err := populateDefaults(options, v3); err == nil {
		t.Errorf("Expected error float: %v", v3)
	}
}

func Test_processTokensUnfused(t *testing.T) {

	s := struct {
		A int `arg-flag:"-a"`
		B int `arg-flag:"-b" arg-default:"7"`
		C int
		P bool `arg-flag:"-p"`
		Q bool
	}{}

	v, _ := unwrap(&s)
	options, _, _ := analyzeStruct(v)

	tests := []struct {
		toks []string
		want []string
		rest []string
		err  bool
	}{
		{[]string{}, []string{}, []string{}, false},
		{[]string{"-a", "1"}, []string{"1"}, []string{}, false},
		{[]string{"-a"}, []string{}, []string{}, true},
		{[]string{"-b", "9"}, []string{"9"}, []string{}, false},
		{[]string{"-a1"}, []string{"1"}, []string{}, false},
		{[]string{"-b9"}, []string{"9"}, []string{}, false},
		{[]string{"-x", "1"}, []string{}, []string{"-x", "1"}, false},
		{[]string{"-x1"}, []string{}, []string{"-x1"}, false},
		{[]string{"-x", "-a", "1"}, []string{"1"}, []string{"-x"}, false},
		{[]string{"-a", "1", "-x"}, []string{"1"}, []string{"-x"}, false},
		{[]string{"--", "-a", "1"}, []string{}, []string{"-a", "1"}, false},
		{[]string{"-p"}, []string{""}, []string{}, false},
		{[]string{"-p1"}, []string{"1"}, []string{}, false},
		{[]string{"-p", "1"}, []string{""}, []string{"1"}, false},
	}

	for _, test := range tests {
		flags, pos, err := processTokens(options, test.toks, false)

		if (err != nil) != test.err {
			t.Errorf("%v: Unexpected error: %v", test.toks, err)
		}

		if err != nil {
			continue
		}

		if len(flags) != len(test.want) {
			t.Errorf("%v: Unexpected flags", test.toks)
		}
		for i, f := range flags {
			if f.value != test.want[i] {
				t.Errorf("%v: got=%s want=%s", test.toks, f.value, test.want[i])
			}
		}

		if len(pos) != len(test.rest) {
			t.Errorf("%v: Unexpected flags", test.toks)
		}
		for i, p := range pos {
			if p != test.rest[i] {
				t.Errorf("%v: got=%s want=%s", test.toks, p, test.rest[i])
			}
		}
	}
}

func Test_processTokensFused(t *testing.T) {

	s := struct {
		A int `arg-flag:"-a"`
		B int `arg-flag:"-b" arg-default:"7"`
		C int
		P bool `arg-flag:"-p"`
		Q bool
	}{}

	v, _ := unwrap(&s)
	options, _, _ := analyzeStruct(v)

	tests := []struct {
		toks []string
		want []string
		rest []string
		err  bool
	}{
		{[]string{}, []string{}, []string{}, false},
		{[]string{"-a", "1"}, []string{""}, []string{"1"}, false},
		{[]string{"-a"}, []string{""}, []string{}, false},
		{[]string{"-b", "9"}, []string{"7"}, []string{"9"}, false},
		{[]string{"-a1"}, []string{"1"}, []string{}, false},
		{[]string{"-b9"}, []string{"9"}, []string{}, false},
		{[]string{"-b"}, []string{"7"}, []string{}, false},
		{[]string{"-x", "1"}, []string{}, []string{"-x", "1"}, false},
		{[]string{"-x1"}, []string{}, []string{"-x1"}, false},
		{[]string{"-x", "-a", "1"}, []string{""}, []string{"-x", "1"}, false},
		{[]string{"-a", "1", "-x"}, []string{""}, []string{"1", "-x"}, false},
		{[]string{"--", "-a", "1"}, []string{}, []string{"-a", "1"}, false},
		{[]string{"-p"}, []string{""}, []string{}, false},
		{[]string{"-p1"}, []string{"1"}, []string{}, false},
		{[]string{"-p", "1"}, []string{""}, []string{"1"}, false},
	}

	for _, test := range tests {
		flags, pos, err := processTokens(options, test.toks, true)

		if (err != nil) != test.err {
			t.Errorf("%v: Unexpected error: %v", test.toks, err)
		}

		if err != nil {
			continue
		}

		if len(flags) != len(test.want) {
			t.Errorf("%v: Unexpected flags", test.toks)
		}
		for i, f := range flags {
			if f.value != test.want[i] {
				t.Errorf("%v: got=%s want=%s", test.toks, f.value, test.want[i])
			}
		}

		if len(pos) != len(test.rest) {
			t.Errorf("%v: Unexpected flags", test.toks)
		}
		for i, p := range pos {
			if p != test.rest[i] {
				t.Errorf("%v: got=%s want=%s", test.toks, p, test.rest[i])
			}
		}
	}
}

func Test_lookupFlag(t *testing.T) {
	opts := map[string]fieldInfo{
		"-a":   fieldInfo{},
		"+a":   fieldInfo{},
		"--aa": fieldInfo{},
	}

	tests := []struct {
		input, flag, value string
		found              bool
	}{
		{"", "", "", false},
		{"-", "", "", false},
		{"--", "", "", false},
		{"-a", "-a", "", true},
		{"-a3", "-a", "3", true},
		{"-aaa", "-a", "aa", true},
		{"-b", "", "", false},
		{"-b3", "", "", false},
		{"+a", "+a", "", true},
		{"+a3", "+a", "3", true},
		{"+a+3", "+a", "+3", true},
		{"+a-3", "+a", "-3", true},
		{"+b", "", "", false},
		{"+b3", "", "", false},
		{"--aa", "--aa", "", true},
		{"--aa=3", "--aa", "3", true},
		{"--aa=+3", "--aa", "+3", true},
		{"--aa=-3", "--aa", "-3", true},
		{"--aa=aa", "--aa", "aa", true},
		{"--bb", "", "", false},
		{"--bb=3", "", "", false},
		{"-a 3", "", "", false},
		{"--aa 3", "", "", false},
	}

	for _, test := range tests {
		got, ok := lookupFlag(test.input, opts)

		if ok != test.found {
			t.Errorf("%s: Found: got=%v want=%v", test.input, ok, test.found)
			continue
		}

		if got.flag != test.flag {
			t.Errorf("%s: Flag: got=%s want=%s",
				test.input, got.flag, test.flag)
		}

		if got.value != test.value {
			t.Errorf("%s: Value: got=%s want=%s",
				test.input, got.value, test.value)
		}
	}
}

func Test_populateField(t *testing.T) {
	// Remember that only public (ie: capitalized) fields can be set!
	s := struct {
		I int
		S []int
	}{}

	v, _ := unwrap(&s)

	tests := []struct {
		fieldName string
		value     string
		err       bool
	}{
		{"I", "2", false},
		{"S", "4", false},
		{"S", "5", false},
		{"I", "a", true}, // cannot convert
		{"X", "0", true}, // non-existing field
	}

	for _, test := range tests {
		field, ok := v.Type().FieldByName(test.fieldName)
		if ok != true {
			// Skip non-existing fields
			continue
		}
		info, _ := makeFieldInfo(field)
		info.value = test.value

		err := populateField(info, v)
		if (err != nil) != test.err {
			t.Errorf("%s: Unexpected error=%v wantError=%v",
				field.Name, err, test.err)
		}
	}

	if s.I != 2 {
		t.Errorf("Bad scalar assignment: got=%v", s.I)
	}
	if len(s.S) != 2 || s.S[0] != 4 || s.S[1] != 5 {
		t.Errorf("Bad slice assignment: got=%v", s.S)
	}
}

func Test_convertToType(tt *testing.T) {

	// Shorthands:
	t := func(a any) reflect.Type { return reflect.TypeOf(a) }
	v := func(a any) reflect.Value { return reflect.ValueOf(a) }
	v2 := func(a any, _ error) reflect.Value { return reflect.ValueOf(a) }

	tests := []struct {
		baseType       reflect.Type
		val, def, frmt string
		isErr          bool
		want           reflect.Value
	}{
		{t(""), "", "", "", false, v("")},
		{t(""), " ", "", "", false, v(" ")},
		{t(""), "X", "", "", false, v("X")},
		{t(""), "", "Y", "", false, v("Y")},
		{t(""), "a", "b", "", false, v("a")},
		{t(""), "a", "b", "c", false, v("a")},

		{t(int(0)), "", "", "", true, v(0)},
		{t(int(0)), "0", "", "", false, v(0)},
		{t(int(0)), "0", "1", "", false, v(0)},
		{t(int(0)), "1", "-1", "", false, v(1)},
		{t(int(0)), "0a", "", "", true, v(0)},
		{t(int(0)), "0 a", "", "", true, v(0)},
		{t(int(0)), "", "1", "", false, v(1)},
		{t(int(0)), "a", "1", "", true, v(1)},
		{t(int(0)), "", "a", "", true, v(1)},
		{t(int(0)), "1.0", "", "", true, v(1)},
		{t(int(0)), "-1", "", "", false, v(-1)},
		{t(int(0)), "- 1", "", "", true, v(1)},

		{t(float64(0.)), "", "", "", true, v(0.)},
		{t(float64(0.)), "0", "", "", false, v(0.)},
		{t(float64(0.)), "", "0", "", false, v(0.)},
		{t(float64(0.)), "2e0", "0", "", false, v(2.)},
		{t(float64(0.)), "2e", "0", "", true, v(2.)},
		{t(float64(0.)), "2e1", "0", "", false, v(20.)},
		{t(float64(0.)), "2e-1", "0", "", false, v(.2)},
		{t(float64(0.)), "", "-1e2", "", false, v(-100.)},

		{t(time.Now()), "", "", "", true, v(time.Now())},
		{t(time.Now()), "2004-12-01 23:45:00", "", "", false,
			v(time.Date(2004, 12, 1, 23, 45, 0, 0, time.UTC))},
		{t(time.Now()), "2034-12-01 11:45:00", "", "", false,
			v(time.Date(2034, 12, 1, 11, 45, 0, 0, time.UTC))},
		{t(time.Now()), "2034-02-30 11:45:00", "", "", true, // 30Feb!
			v(time.Date(2034, 12, 1, 11, 45, 0, 0, time.UTC))},
		{t(time.Now()), "2025-12-01", "", "2006-01-02", false,
			v(time.Date(2025, 12, 1, 0, 0, 0, 0, time.UTC))},
		{t(time.Now()), "24/12/2025", "", "02/01/2006", false,
			v(time.Date(2025, 12, 24, 0, 0, 0, 0, time.UTC))},
		{t(time.Now()), "30/02/2025", "", "02/01/2006", true, // 30Feb!
			v(time.Date(2025, 12, 24, 0, 0, 0, 0, time.UTC))},
		{t(time.Now()), "23-33-31", "", "15-04-05", false,
			v(time.Date(0, 1, 1, 23, 33, 31, 0, time.UTC))},
		{t(time.Now()), "35-33-31", "", "15-04-05", true, // bad time
			v(time.Date(0, 1, 1, 35, 33, 31, 0, time.UTC))},
		{t(time.Now()), "", "2004-12-01 23:45:00", "", false,
			v(time.Date(2004, 12, 1, 23, 45, 0, 0, time.UTC))},
		{t(time.Now()), "", "2004-12-01", "2006-01-02", false,
			v(time.Date(2004, 12, 1, 0, 0, 0, 0, time.UTC))},

		{t(time.Since(time.Now())), "", "", "", true,
			v2(time.ParseDuration(""))},
		{t(time.Since(time.Now())), "300ms", "", "", false,
			v2(time.ParseDuration("300ms"))},
		{t(time.Since(time.Now())), "", "300ms", "", false,
			v2(time.ParseDuration("300ms"))},
	}

	for _, test := range tests {
		info := fieldInfo{baseType: test.baseType, value: test.val,
			defaultval: test.def, format: test.frmt}

		vv, err := convertToType(info)

		if (err != nil) != test.isErr {
			tt.Errorf("%s: Error: got=%v want=%v", test.val, err, test.isErr)
		}

		if err == nil && !test.want.Equal(vv) {
			tt.Errorf("%s: Value: got=%v want=%v", test.val, vv, test.want)
		}
	}

}

func Test_populateOptionsOk(t *testing.T) {
	// Compare: Test_populateField()

	// Remember that only public (ie: capitalized) fields can be set!
	s := struct {
		I int
		//		P *int
		S []int
	}{}

	v, _ := unwrap(&s)

	values := []struct {
		fieldName string
		value     string
		err       bool
	}{
		{"I", "2", false},
		//		{"P", "3", false},
		{"S", "4", false},
		{"S", "5", false},
	}

	options := []fieldInfo{}

	for _, val := range values {
		field, _ := v.Type().FieldByName(val.fieldName)
		info, _ := makeFieldInfo(field)
		info.value = val.value

		options = append(options, info)
	}

	err := populateOptions(options, v)
	if err != nil {
		t.Errorf("Unexpeced error: %v", err)
	}

	if s.I != 2 {
		t.Errorf("Bad scalar assignment: got=%v", s.I)
	}
	// if *s.P != 3 {
	// 	t.Errorf("Bad pointer assignment: got=%v", *s.P)
	// }
	if len(s.S) != 2 || s.S[0] != 4 || s.S[1] != 5 {
		t.Errorf("Bad slice assignment: got=%v", s.S)
	}
}

func Test_populateOptionsErr(t *testing.T) {
	s := struct {
		I int
	}{}

	v, _ := unwrap(&s)

	values := []struct {
		fieldName string
		value     string
		err       bool
	}{
		{"I", "a", false}, // non-convertible value
		{"X", "3", false}, // non-existing field
	}

	options := []fieldInfo{}

	for _, val := range values {
		field, ok := v.Type().FieldByName(val.fieldName)
		if ok != true {
			// Skip non-existing fields
			continue
		}

		info, _ := makeFieldInfo(field)
		info.value = val.value

		options = append(options, info)
	}

	err := populateOptions(options, v)
	if err == nil {
		t.Errorf("Expected error - bad value for field")
	}

	// Does not really test for non-existing field...
}

func Test_populatePositionals(t *testing.T) {

	type s struct {
		A, B, C int
		S, X    []int
		U, V, W int
	}

	tests := []struct {
		fields      []string
		tokens      []string
		wantScalars []int
		wantSlice   []int
		wantErr     bool
	}{
		// No slice
		{[]string{}, []string{}, []int{0, 0, 0, 0, 0, 0}, []int{}, false},
		{[]string{}, []string{"1"}, []int{0, 0, 0, 0, 0, 0}, []int{}, true},
		{[]string{"A"}, []string{}, []int{0, 0, 0, 0, 0, 0}, []int{}, true},
		{[]string{"A"}, []string{"3"}, []int{3, 0, 0, 0, 0, 0}, []int{}, false},
		{[]string{"V"}, []string{"3"}, []int{0, 0, 0, 0, 3, 0}, []int{}, false},
		{[]string{"A", "C"}, []string{"3", "4"},
			[]int{3, 0, 4, 0, 0, 0}, []int{}, false},
		{[]string{"A", "C", "U"}, []string{"3", "4", "5"},
			[]int{3, 0, 4, 5, 0, 0}, []int{}, false},
		{[]string{"A", "U", "B"}, []string{"3", "4", "5"},
			[]int{3, 5, 0, 4, 0, 0}, []int{}, false},

		// Two slices
		{[]string{"S", "X"}, []string{},
			[]int{0, 0, 0, 0, 0, 0}, []int{}, true},

		// One slice
		{[]string{"S"}, []string{},
			[]int{0, 0, 0, 0, 0, 0}, []int{}, false},
		{[]string{"S"}, []string{"1", "2", "3"},
			[]int{0, 0, 0, 0, 0, 0}, []int{1, 2, 3}, false},
		{[]string{"B", "S"}, []string{"-1", "1", "2", "3"},
			[]int{0, -1, 0, 0, 0, 0}, []int{1, 2, 3}, false},
		{[]string{"S", "W"}, []string{"1", "2", "3", "7"},
			[]int{0, 0, 0, 0, 0, 7}, []int{1, 2, 3}, false},
		{[]string{"A", "S", "W"}, []string{"-1", "1", "2", "3", "7"},
			[]int{-1, 0, 0, 0, 0, 7}, []int{1, 2, 3}, false},
		{[]string{"A", "B", "C", "S"}, []string{"-1", "1", "3", "8", "9"},
			[]int{-1, 1, 3, 0, 0, 0}, []int{8, 9}, false},
		{[]string{"S", "U", "V", "W"}, []string{"-1", "1", "3", "8", "9"},
			[]int{0, 0, 0, 3, 8, 9}, []int{-1, 1}, false},
		{[]string{"A", "B", "S", "V"}, []string{"-1", "1", "3", "4", "9"},
			[]int{-1, 1, 0, 0, 9, 0}, []int{3, 4}, false},
		{[]string{"B", "S", "U", "W"}, []string{"-1", "1", "2", "8", "9"},
			[]int{0, -1, 0, 8, 0, 9}, []int{1, 2}, false},
		{[]string{"A", "B", "S", "V"}, []string{"-1", "1", "9"},
			[]int{-1, 1, 0, 0, 9, 0}, []int{}, false},
		{[]string{"B", "S", "U", "W"}, []string{"-1", "8", "9"},
			[]int{0, -1, 0, 8, 0, 9}, []int{}, false},
	}

	for _, test := range tests {
		x := s{}
		v, _ := unwrap(&x)

		positionals := []fieldInfo{}
		for _, f := range test.fields {
			field, _ := v.Type().FieldByName(f)

			info, _ := makeFieldInfo(field)
			positionals = append(positionals, info)
		}

		err := populatePositionals(positionals, test.tokens, v)

		if (err != nil) != test.wantErr {
			t.Errorf("%v: Unexpected error=%v wantErr=%v",
				test.fields, err, test.wantErr)
		}

		// check wantScalars is well-formed:
		if len(test.wantScalars) != 6 {
			t.Errorf("Malformed test case: wantScalars=%v", test.wantScalars)
		}

		if !(x.A == test.wantScalars[0] && x.B == test.wantScalars[1] &&
			x.C == test.wantScalars[2] && x.U == test.wantScalars[3] &&
			x.V == test.wantScalars[4] && x.W == test.wantScalars[5]) {
			t.Errorf("%v: Wrong values: got=%v want=%v",
				test.fields, x, test.wantScalars)
		}

	}

}

func Test_formatHelp(t *testing.T) {
	s := struct {
		A int `arg-help:"text without term"`
		B int `arg-help:"text *with* term"`
		C int `arg-help:""`
		D int
	}{}

	v, _ := unwrap(&s)

	tests := []struct {
		fieldName          string
		text1, text2, term string // useName: false, true
	}{
		{"A", "text without term", "text without term", "int"},
		{"B", "text with term", "text with term", "with"},
		{"C", "", "C", "int"},
		{"D", "", "D", "int"},
	}

	for _, test := range tests {
		field, _ := v.Type().FieldByName(test.fieldName)
		info, _ := makeFieldInfo(field)

		got1, got2 := formatHelp(info, false)
		if got1 != test.text1 || got2 != test.term {
			t.Errorf("%s useName=false: got1=%s want1=%s got2=%s want2=%s",
				info.Name, got1, test.text1, got2, test.term)
		}

		got1, got2 = formatHelp(info, true)
		if got1 != test.text2 || got2 != test.term {
			t.Errorf("%s useName=true: got1=%s want1=%s got2=%s want2=%s",
				info.Name, got1, test.text2, got2, test.term)
		}
	}
}

type simpleArgs struct {
	Flag    bool      `arg-flag:"-b" arg-help:"This is a flag"`
	Counter int       `arg-flag:"+c" arg-help:"This is the *counter* here"`
	Name    string    `arg-flag:"-s --name" arg-default:"unknown"`
	Time    time.Time `arg-flag:"--time"`
	Ignored string    `arg-ignore:""`
	NoGood  uint      `arg-ignore:""`
	Number  int
	Source  string `arg-help:"The *source* of it all"`
	Rest    []string
}

func equal_simple(want, got *simpleArgs) bool {

	// Skips ignored fields!

	if want.Flag != got.Flag ||
		want.Counter != got.Counter ||
		want.Name != got.Name ||
		want.Time != got.Time ||
		want.Number != got.Number ||
		want.Source != got.Source {
		return false
	}

	if want.Rest == nil && got.Rest != nil || len(want.Rest) != len(got.Rest) {
		return false
	}
	for i, w := range want.Rest {
		if w != got.Rest[i] {
			return false
		}
	}

	return true
}

func Test_FromSliceSimple(t *testing.T) {

	tests := []struct {
		slice   []string
		want    simpleArgs
		wantErr bool
	}{
		// Positionals and options
		{
			[]string{"1", "a"},
			simpleArgs{false, 0, "unknown", time.Time{}, "", 0, 1, "a", nil},
			false,
		},
		{
			[]string{"-b", "1", "a"},
			simpleArgs{true, 0, "unknown", time.Time{}, "", 0, 1, "a", nil},
			false,
		},
		{
			[]string{"+c7", "1", "a"},
			simpleArgs{false, 7, "unknown", time.Time{}, "", 0, 1, "a", nil},
			false,
		},
		{
			[]string{"+c", "9", "1", "a"},
			simpleArgs{false, 9, "unknown", time.Time{}, "", 0, 1, "a", nil},
			false,
		},

		// Out of order
		{
			[]string{"+c", "9", "-b", "1", "a"},
			simpleArgs{true, 9, "unknown", time.Time{}, "", 0, 1, "a", nil},
			false,
		},
		{
			[]string{"-s", "9", "-b", "1", "a"},
			simpleArgs{true, 0, "9", time.Time{}, "", 0, 1, "a", nil},
			false,
		},
		{
			[]string{"-s", "9", "5", "x", "-b"},
			simpleArgs{true, 0, "9", time.Time{}, "", 0, 5, "x", nil},
			false,
		},
		{
			[]string{"3", "-s", "9", "c", "-b"},
			simpleArgs{true, 0, "9", time.Time{}, "", 0, 3, "c", nil},
			false,
		},
		{
			[]string{"-s", "9", "5", "-b", "x"},
			simpleArgs{true, 0, "9", time.Time{}, "", 0, 5, "x", nil},
			false,
		},
		{
			[]string{"3", "-s", "9", "-b", "c"},
			simpleArgs{true, 0, "9", time.Time{}, "", 0, 3, "c", nil},
			false,
		},

		// String flag
		{
			[]string{"-s", "thing", "1", "a"},
			simpleArgs{false, 0, "thing", time.Time{}, "", 0, 1, "a", nil},
			false,
		},
		{
			[]string{"-sthing", "1", "a"},
			simpleArgs{false, 0, "thing", time.Time{}, "", 0, 1, "a", nil},
			false,
		},
		{
			[]string{"--name", "thing", "1", "a"},
			simpleArgs{false, 0, "thing", time.Time{}, "", 0, 1, "a", nil},
			false,
		},
		{
			[]string{"--name=thing", "1", "a"},
			simpleArgs{false, 0, "thing", time.Time{}, "", 0, 1, "a", nil},
			false,
		},

		// Numeric argument
		{
			[]string{"-b", "+c", "1", "a"}, // "1" is eaten by +c!
			simpleArgs{true, 0, "unknown", time.Time{}, "", 0, 1, "a", nil},
			true,
		},
		{
			[]string{"+c", "-7", "1", "a"},
			simpleArgs{false, -7, "unknown", time.Time{}, "", 0, 1, "a", nil},
			false,
		},
		{
			[]string{"+c-7", "1", "a"},
			simpleArgs{false, -7, "unknown", time.Time{}, "", 0, 1, "a", nil},
			false,
		},

		// Repeated
		{
			[]string{"+c", "7", "1", "a", "+c", "9"},
			simpleArgs{false, 9, "unknown", time.Time{}, "", 0, 1, "a", nil},
			false,
		},
		{
			[]string{"+c9", "1", "a", "+c", "7"},
			simpleArgs{false, 7, "unknown", time.Time{}, "", 0, 1, "a", nil},
			false,
		},

		// Time
		{
			[]string{"--time", "", "1", "a"}, // bad time
			simpleArgs{false, 0, "unknown", time.Time{}, "", 0, 1, "a", nil},
			true,
		},
		{
			[]string{"--time", "2025-02-30 23:45:00", "1", "a"}, // bad time
			simpleArgs{false, 0, "unknown", time.Time{}, "", 0, 1, "a", nil},
			true,
		},
		{
			[]string{"--time", "2025-02-15 11:33:00", "1", "a"},
			simpleArgs{false, 0, "unknown",
				time.Date(2025, 02, 15, 11, 33, 0, 0, time.UTC),
				"", 0, 1, "a", nil},
			false,
		},

		// Token "-" is not special
		{
			[]string{"-s", "-", "1", "a"},
			simpleArgs{false, 0, "-", time.Time{}, "", 0, 1, "a", nil},
			false,
		},
		{
			[]string{"-s", "-b", "1", "a"},
			simpleArgs{false, 0, "-b", time.Time{}, "", 0, 1, "a", nil},
			false,
		},
		{
			[]string{"-s-", "1", "a"},
			simpleArgs{false, 0, "-", time.Time{}, "", 0, 1, "a", nil},
			false,
		},
		{
			[]string{"--name", "-", "1", "a"},
			simpleArgs{false, 0, "-", time.Time{}, "", 0, 1, "a", nil},
			false,
		},
		{
			[]string{"--name=-", "1", "a"},
			simpleArgs{false, 0, "-", time.Time{}, "", 0, 1, "a", nil},
			false,
		},

		// Positionals
		{
			[]string{"-s", "-", "1", "a", "x"},
			simpleArgs{false, 0, "-", time.Time{}, "", 0, 1, "a",
				[]string{"x"}},
			false,
		},
		{
			[]string{"-s", "-", "1", "a", "x", "y", "z"},
			simpleArgs{false, 0, "-", time.Time{}, "", 0, 1, "a",
				[]string{"x", "y", "z"}},
			false,
		},
		{
			[]string{"1", "a", "x", "y", "z", "-s", "-"},
			simpleArgs{false, 0, "-", time.Time{}, "", 0, 1, "a",
				[]string{"x", "y", "z"}},
			false,
		},
		{
			[]string{"1", "a", "x", "y", "z"},
			simpleArgs{false, 0, "unknown", time.Time{}, "", 0, 1, "a",
				[]string{"x", "y", "z"}},
			false,
		},
		{
			[]string{"1", "a", "x", "y", "z", "-Y", "-s", "-", "-X"},
			simpleArgs{false, 0, "-", time.Time{}, "", 0, 1, "a",
				[]string{"x", "y", "z", "-Y", "-X"}},
			false,
		},

		// Token "--" is special
		{
			[]string{"--", "1", "a"},
			simpleArgs{false, 0, "unknown", time.Time{}, "", 0, 1, "a",
				[]string{}},
			false,
		},
		{
			[]string{"--", "1", "a", "x"},
			simpleArgs{false, 0, "unknown", time.Time{}, "", 0, 1, "a",
				[]string{"x"}},
			false,
		},
		{
			[]string{"--", "1", "a", "x", "-b"},
			simpleArgs{false, 0, "unknown", time.Time{}, "", 0, 1, "a",
				[]string{"x", "-b"}},
			false,
		},
		{
			[]string{"-s", "foo", "--", "1", "a", "x", "-b", "-sboo"},
			simpleArgs{false, 0, "foo", time.Time{}, "", 0, 1, "a",
				[]string{"x", "-b", "-sboo"}},
			false,
		},
	}

	for _, test := range tests {
		s := simpleArgs{}

		err := FromSlice(test.slice, &s)
		if (err != nil) != test.wantErr {
			t.Errorf("%s: Unexpected error: %v", test.slice, err)
		}

		if err == nil && !equal_simple(&test.want, &s) {
			t.Errorf("%s: Not equal\nwant=%v\ngot=%v",
				test.slice, test.want, s)
		}
	}
}

type sliceArgs struct {
	Flags  []bool   `arg-flag:"-b"`
	Names  []string `arg-flag:"-s"`
	Source string
	Rest   []string
}

func equal_slices(want, got *sliceArgs) bool {

	if want.Flags == nil && got.Flags != nil {
		return false
	}
	if !slices.Equal(want.Flags, got.Flags) {
		return false
	}

	if want.Names == nil && got.Names != nil {
		return false
	}
	if !slices.Equal(want.Names, got.Names) {
		return false
	}

	if want.Source != got.Source {
		return false
	}

	if want.Rest == nil && got.Rest != nil {
		return false
	}
	if !slices.Equal(want.Rest, got.Rest) {
		return false
	}

	return true
}

func Test_FromSliceSlices(t *testing.T) {

	tests := []struct {
		slice []string
		want  sliceArgs
	}{
		// Positionals and options
		{
			[]string{"a"},
			sliceArgs{nil, nil, "a", nil},
		},
		{
			[]string{"-b", "a"},
			sliceArgs{[]bool{true}, nil, "a", nil},
		},
		{
			[]string{"-b", "-b", "a"},
			sliceArgs{[]bool{true, true}, nil, "a", nil},
		},
		{
			[]string{"-b", "a", "-b"},
			sliceArgs{[]bool{true, true}, nil, "a", nil},
		},
		{
			[]string{"-b", "a", "-s", "X", "-b"},
			sliceArgs{[]bool{true, true}, []string{"X"}, "a", nil},
		},
		{
			[]string{"-b", "a", "-s", "X", "-b", "u", "v"},
			sliceArgs{[]bool{true, true}, []string{"X"}, "a",
				[]string{"u", "v"}},
		},
		{
			[]string{"-b", "a", "-s", "X", "--", "-b", "u", "v"},
			sliceArgs{[]bool{true}, []string{"X"}, "a",
				[]string{"-b", "u", "v"}},
		},
	}

	for _, test := range tests {
		s := sliceArgs{}

		err := FromSlice(test.slice, &s)
		if err != nil {
			t.Errorf("%s: Unexpected error: %v", test.slice, err)
			continue
		}

		if !equal_slices(&test.want, &s) {
			t.Errorf("%s: Not equal\nwant=%v\ngot=%v",
				test.slice, test.want, s)
		}
	}
}

type defaultArgs struct {
	Flag1    bool   `arg-flag:"-b"`
	Flag2    bool   `arg-flag:"-B" arg-default:"true"` // useless, but recognizd
	Counter1 int    `arg-flag:"+c"`
	Counter2 int    `arg-flag:"+C" arg-default:"7"`
	Name1    string `arg-flag:"-s --name"`
	Name2    string `arg-flag:"-S --Name" arg-default:"unknown"`
	Rest     []string
}

func equal_default(want, got *defaultArgs) bool {
	if want.Flag1 != got.Flag1 ||
		want.Flag2 != got.Flag2 ||
		want.Counter1 != got.Counter1 ||
		want.Counter2 != got.Counter2 ||
		want.Name1 != got.Name1 ||
		want.Name2 != got.Name2 {
		return false
	}

	if want.Rest == nil && got.Rest != nil || len(want.Rest) != len(got.Rest) {
		return false
	}
	for i, w := range want.Rest {
		if w != got.Rest[i] {
			return false
		}
	}

	return true
}

func Test_FromSliceDefaultUnfused(t *testing.T) {

	tests := []struct {
		slice   []string
		want    defaultArgs
		wantErr bool
	}{
		// Positionals and options
		{
			[]string{"1", "a"},
			defaultArgs{false, true, 0, 7, "", "unknown", []string{"1", "a"}},
			false,
		},
		{
			[]string{"-b"},
			defaultArgs{true, true, 0, 7, "", "unknown", []string{}},
			false,
		},
		{
			[]string{"+c"},
			defaultArgs{false, true, 0, 7, "", "unknown", []string{}},
			true,
		},
		{
			[]string{"+c1"},
			defaultArgs{false, true, 1, 7, "", "unknown", []string{}},
			false,
		},
		{
			[]string{"+C"},
			defaultArgs{false, true, 0, 7, "", "unknown", []string{}},
			true,
		},
		{
			[]string{"+C", "9"},
			defaultArgs{false, true, 0, 9, "", "unknown", []string{}},
			false,
		},
		{
			[]string{"--Name", "known"},
			defaultArgs{false, true, 0, 7, "", "known", []string{}},
			false,
		},
		{
			[]string{"--Name", "=known"},
			defaultArgs{false, true, 0, 7, "", "=known", []string{}},
			false,
		},
		{
			[]string{"--Name=known"},
			defaultArgs{false, true, 0, 7, "", "known", []string{}},
			false,
		},
	}

	for _, test := range tests {
		s := defaultArgs{}

		err := FromSlice(test.slice, &s)
		if (err != nil) != test.wantErr {
			t.Errorf("%s: Unexpected error: %v", test.slice, err)
		}

		if err == nil && !equal_default(&test.want, &s) {
			t.Errorf("%s: Not equal\nwant=%v\ngot=%v",
				test.slice, test.want, s)
		}
	}
}

func Test_FromSliceDefaultFused(t *testing.T) {

	tests := []struct {
		slice   []string
		want    defaultArgs
		wantErr bool
	}{
		// Positionals and options
		{
			[]string{"1", "a"},
			defaultArgs{false, false, 0, 0, "", "", []string{"1", "a"}},
			false,
		},
		{
			[]string{"-b"},
			defaultArgs{true, false, 0, 0, "", "", []string{}},
			false,
		},
		{
			[]string{"+c"},
			defaultArgs{false, false, 0, 0, "", "", []string{}},
			true,
		},
		{
			[]string{"+c1"},
			defaultArgs{false, false, 1, 0, "", "", []string{}},
			false,
		},
		{
			[]string{"+c"},
			defaultArgs{false, false, 0, 7, "", "", []string{}},
			true,
		},
		{
			[]string{"+C", "9"},
			defaultArgs{false, false, 0, 7, "", "", []string{"9"}},
			false,
		},
		{
			[]string{"--Name", "known"},
			defaultArgs{false, false, 0, 0, "", "unknown", []string{"known"}},
			false,
		},
		{
			[]string{"--Name", "=known"},
			defaultArgs{false, false, 0, 0, "", "unknown", []string{"=known"}},
			false,
		},
		{
			[]string{"--Name=known"},
			defaultArgs{false, false, 0, 0, "", "known", []string{}},
			false,
		},
	}

	for _, test := range tests {
		s := defaultArgs{}

		err := FromSliceFused(test.slice, &s)
		if (err != nil) != test.wantErr {
			t.Errorf("%s: Unexpected error: %v", test.slice, err)
		}

		if err == nil && !equal_default(&test.want, &s) {
			t.Errorf("%s: Not equal\nwant=%v\ngot=%v",
				test.slice, test.want, s)
		}
	}
}

func Test_FromSliceBad(t *testing.T) {
	s := struct {
		Counter int `arg-flag:"+C" arg-default:"x"`
	}{}

	if err := FromSlice([]string{}, &s); err == nil {
		t.Errorf("Wanted error")
	}
	if err := FromSlice([]string{"+C"}, &s); err == nil {
		t.Errorf("Wanted error")
	}
	if err := FromSlice([]string{"+C1"}, &s); err == nil {
		t.Errorf("Wanted error")
	}

	// In fused mode, the default is only evaluated IFF the flag is present
	// without a value. No flag, OR value present: default is not evaluated!
	if err := FromSliceFused([]string{}, &s); err != nil {
		t.Errorf("Unexpected error")
	}
	if err := FromSliceFused([]string{"+C"}, &s); err == nil {
		t.Errorf("Wanted error")
	}
	if err := FromSliceFused([]string{"+C1"}, &s); err != nil {
		t.Errorf("Unexpected error")
	}
}

func Test_WriteShortUsage(t *testing.T) {
	arg1 := simpleArgs{}
	want1 := "[+c counter] [-b] [-s|--name string] [--time time.Time] [int] [source] [string]+ \n"

	sb := strings.Builder{}
	WriteShortUsage(&sb, &arg1)

	if sb.String() != want1 {
		t.Errorf("want=%s\ngot=%s", want1, sb.String())
	}

	// -----

	arg2 := sliceArgs{}
	want2 := "[-b]+ [-s string]+ [string] [string]+ \n"

	sb = strings.Builder{}
	WriteShortUsage(&sb, &arg2)

	if sb.String() != want2 {
		t.Errorf("want=%s\ngot=%s", want2, sb.String())
	}
}

func Test_WriteUsage(t *testing.T) {
	arg1 := simpleArgs{}
	arg2 := sliceArgs{}

	if false {
		PrintUsage(&arg1)
		PrintUsage(&arg2)
	}
}

func Test_WriteValues(t *testing.T) {

	arg := simpleArgs{}
	FromSlice([]string{"1", "a", "x", "y", "z", "-Y", "-s", "-", "-X", "--time", "2025-02-15 11:33:00", "1", "a", "+c", "7", "1", "a", "+c", "9"}, &arg)

	if false {
		PrintValues(&arg)
		PrintValuesWithTags(&arg)
	}
}
