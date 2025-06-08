package cleanarg

import (
	"fmt"
	"io"
	"os"
	"reflect"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	tagFlag    = "arg-flag"
	tagHelp    = "arg-help"
	tagDefault = "arg-default"
	tagFormat  = "arg-format"
	tagIgnore  = "arg-ignore"
)

const (
	shortFlag = "^[-+][0-9A-Za-z]$"
	longFlag  = "^--[0-9A-Za-z][0-9A-Za-z-]+$" // first char must not be '-'
)

const (
	defaultTimeFormat = "2006-01-02 15:04:05" // no TimeZone!
)

const (
	helpArgument  = `\*.+?\*`
	helpDelimiter = "*"
)

const (
	endFlagsIndicator = "--"
)

var shortFlagRE, longFlagRE, helpArgumentRE *regexp.Regexp

var allowedTypes map[reflect.Type]struct{}

func init() {
	allowedTypes = map[reflect.Type]struct{}{}

	allowedTypes[reflect.TypeOf(string(""))] = struct{}{}
	allowedTypes[reflect.TypeOf(false)] = struct{}{}
	allowedTypes[reflect.TypeOf(int(0))] = struct{}{}
	allowedTypes[reflect.TypeOf(float64(0.0))] = struct{}{}
	allowedTypes[reflect.TypeOf(time.Now())] = struct{}{}
	allowedTypes[reflect.TypeOf(time.Duration(0))] = struct{}{}

	shortFlagRE = regexp.MustCompile(shortFlag)
	longFlagRE = regexp.MustCompile(longFlag)

	helpArgumentRE = regexp.MustCompile(helpArgument)
}

// -----

type sortableFlags []string

func (s sortableFlags) Len() int      { return len(s) }
func (s sortableFlags) Swap(i, j int) { s[i], s[j] = s[j], s[i] }
func (s sortableFlags) Less(a, b int) bool {
	if len(s[a]) != len(s[b]) {
		return len(s[a]) < len(s[b])
	}
	return s[a] < s[b]
}

// -----

type fieldInfo struct {
	reflect.StructField

	// Command line values
	flag  string
	value string

	// Tags
	help       string
	defaultval string
	format     string

	// Inferred
	isSlice  bool
	baseType reflect.Type

	allFlags []string // all flags for this option, used by printUsage
}

// -----

// Unwrap takes an argument, which must be a pointer to a struct, and
// returns a reflect.Value of the pointed to struct. It returns an error
// if the argument is not a pointer to a struct.
func unwrap(s any) (reflect.Value, error) {
	// Unwrap interface
	v := reflect.ValueOf(s)

	// Unwrap pointer (if any)
	if v.Kind() != reflect.Pointer {
		return reflect.Value{}, fmt.Errorf("arg must be ptr to struct")
	}
	v = v.Elem()

	// Handle only structs
	if v.Kind() != reflect.Struct {
		return reflect.Value{}, fmt.Errorf("arg must be ptr to struct")
	}

	// v is ptr to struct!
	return v, nil
}

// AnalyzeStruct takes reflect.Value, which must represent a struct, and
// returns a description of its fields, parsing both type info and build
// tags. Returns a map of fieldInfo, keyed on flags, for options, and a
// slice of fieldInfo, in order, for positional arguments.
//
// Returns an error if one of the fields is improper, or more than one
// positional arg is a slice.
func analyzeStruct(v reflect.Value) (map[string]fieldInfo, []fieldInfo, error) {
	typeInfo := v.Type()

	options := map[string]fieldInfo{}
	positionals := []fieldInfo{}

	slices := 0

	for i := 0; i < v.NumField(); i++ {
		field := typeInfo.Field(i)

		if _, ok := field.Tag.Lookup(tagIgnore); ok {
			continue
		}

		info, err := makeFieldInfo(field)
		if err != nil {
			return nil, nil, err
		}

		if flag, ok := field.Tag.Lookup(tagFlag); ok {
			// Field has tag "arg-flag": treat as options field

			// Extract flags from tag entry
			flags, err := extractFlagsSorted(flag)
			if err != nil {
				return nil, nil, err
			}

			// Store all valid flags for crr field in info
			info.allFlags = flags

			// For each flag, create a separate entry in map
			for _, f := range flags {
				options[f] = info
			}

		} else {
			// If not flag/option, treat field as positional

			positionals = append(positionals, info)

			// Count positional slices; more than one is an error
			if info.isSlice {
				slices += 1
				if slices > 1 {
					return nil, nil,
						fmt.Errorf("At most one positional field may be slice")
				}
			}
		}
	}

	return options, positionals, nil
}

// MakeFieldInfo analyses the struct field supplied as argument,
// reading both the field's type and build tags. Returns a populated
// fieldInfo on success, or an error if it encounters a forbidden
// field type.
func makeFieldInfo(field reflect.StructField) (fieldInfo, error) {
	info := fieldInfo{
		StructField: field, // note: NOT reflect.StructField

		// tag.Get() returns "" when tag not found!
		help:       field.Tag.Get(tagHelp),
		defaultval: field.Tag.Get(tagDefault),
		format:     field.Tag.Get(tagFormat),
	}

	// Disallows pointers
	if field.Type.Kind() == reflect.Pointer {
		return fieldInfo{},
			fmt.Errorf("pointers not permitted in struct, maybe use %s tag",
				tagIgnore)
	}

	info.baseType = field.Type

	// Unwrap the base type of slice elements
	if field.Type.Kind() == reflect.Slice {
		info.isSlice = true
		info.baseType = field.Type.Elem()
	}

	// Check for permissible base types
	if _, ok := allowedTypes[info.baseType]; !ok {
		return fieldInfo{},
			fmt.Errorf("%s not permitted in struct, maybe use %s tag",
				info.baseType.String(), tagIgnore)
	}

	return info, nil
}

// ExtractFlagsSorted parses its argument, which should be an arg-flag tag,
// extracts all flags, validates their format, and returns a sorted slice of
// flags. Returns an error if one of the tokens is misformed.
func extractFlagsSorted(s string) (sortableFlags, error) {
	out := sortableFlags{}

	// Extract flags
	for _, token := range strings.Fields(s) {

		if shortFlagRE.MatchString(token) || longFlagRE.MatchString(token) {
			out = append(out, token)
		} else {
			return nil, fmt.Errorf("malformed flag: %s", token)
		}
	}

	sort.Sort(out)

	return out, nil
}

// PopulateFromSlice takes a slice of tokens, a pointer to a struct, and
// a flag that indicates whether values MUST be fused to their flags, and
// populates the struct from the tokens.
//
// Returns an error if the struct or its tags are malformed, if the number
// of tokens does not match the struct, or if one of the tokens (or one of
// the default values) cannot be converted to the required data type.
//
// The token '--' indicates that all subsequent tokens should be treated as
// positionals.
// Unrecognized flags (tokens like -X, --XX, +X, but without matching tag
// entries) are treated as positionals.
func populateFromSlice(tokens []string, data any, isFused bool) error {
	v, err := unwrap(data)
	if err != nil {
		return err
	}

	options, positionals, err := analyzeStruct(v)
	if err != nil {
		return err
	}

	// If not fused mode, populate non-slice options w/ default values
	if !isFused {
		if err := populateDefaults(options, v); err != nil {
			return err
		}
	}

	// Extract options and positional tokens from slice
	retainedOpts, posTokens, err := processTokens(options, tokens, isFused)
	if err != nil {
		return err
	}

	// ... use results to populate struct
	if err := populateOptions(retainedOpts, v); err != nil {
		return err
	}
	if err := populatePositionals(positionals, posTokens, v); err != nil {
		return err
	}

	return nil
}

// Given a map of options, and a reflect.Value representing a pointer to the
// struct to populate, populate all non-slice options with their default
// values (if any). Returns an error if default value conversion fails.
func populateDefaults(options map[string]fieldInfo, v reflect.Value) error {
	defaultOptions := []fieldInfo{}

	for _, info := range options {
		if !info.isSlice && info.defaultval != "" {
			defaultOptions = append(defaultOptions, info)
		}
	}
	if err := populateOptions(defaultOptions, v); err != nil {
		return err
	}

	return nil
}

func processTokens(options map[string]fieldInfo, tokens []string,
	isFused bool) ([]fieldInfo, []string, error) {
	// return processTokens1(options, tokens, isFused)
	// return processTokens2(options, tokens, isFused)
	return processTokens3(options, tokens, isFused)
}

// ProcessTokens takes a map of fieldInfo, describing the known flags and a
// slice of tokens. Returns a slice of fieldInfo containing the recognized,
// retained options, with the fieldInfo.value being set to the supplied
// value. Tokens that are not flags or flag-values are returned as a slice
// of strings. The special token "--" indicates that all following tokens
// should be treated as positionals.
// Returns an error if there are not enough tokens.
//
// In fused mode, if a flag does not have a fused value, the default value
// for that field is used. No additional token is consumed.
func processTokens1(options map[string]fieldInfo, tokens []string,
	isFused bool) ([]fieldInfo, []string, error) {

	retainedOptions, positionalTokens := []fieldInfo{}, []string{}

	// Note:
	// - For options, the "value" to assign is stored in the fieldInfo itself.
	// - For positionals, the value tokens are kept separate.
	// Options take zero or one values; in fact, values may be affixed to the
	// flag. It makes sense to keep flag and value together in fieldInfo.
	// For positionals, it is not clear which field they will be assigned to
	// until all tokens have been seen. If a slice is present, assignment of
	// values to field is even more complicated: done in a separate routine.

	noMoreFlags := false
	for i := 0; i < len(tokens); i++ {

		if tokens[i] == endFlagsIndicator {
			noMoreFlags = true
			continue
		}

		// Treat all subsequent tokens as positionals
		if noMoreFlags == true {
			positionalTokens = append(positionalTokens, tokens[i])
			continue
		}

		// Now, noMoreFlags == false. Must check: is token flag?
		info, ok := lookupFlag(tokens[i], options)

		// Not recognized as flag: treat as positional
		if !ok {
			positionalTokens = append(positionalTokens, tokens[i])
			continue
		}

		// Token is flag. Finished if boolean or value is not empty:
		if info.baseType == reflect.TypeOf(true) || info.value != "" {
			retainedOptions = append(retainedOptions, info)
			continue
		}

		// If we get here, flag still needs value. If fused mode, use default
		if isFused {
			info.value = info.defaultval

		} else { // otherwise, consume next token (if it exists!)
			i += 1
			if len(tokens) > i {
				info.value = tokens[i]
			} else {
				return nil, nil, fmt.Errorf("not enough values")
			}
		}
		retainedOptions = append(retainedOptions, info)
	}

	return retainedOptions, positionalTokens, nil
}

// An alternative version, which searches for "--" FIRST, thus slightly
// altering the semantics (to achieve somewhat cleaner code).
func processTokens2(options map[string]fieldInfo, tokens []string,
	isFused bool) ([]fieldInfo, []string, error) {

	retainedOptions, positionalTokens := []fieldInfo{}, []string{}

	// Note:
	// - For options, the "value" to assign is stored in the fieldInfo itself.
	// - For positionals, the value tokens are kept separate.
	// Options take zero or one values; in fact, values may be affixed to the
	// flag. It makes sense to keep flag and value together in fieldInfo.
	// For positionals, it is not clear which field they will be assigned to
	// until all tokens have been seen. If a slice is present, assignment of
	// values to field is even more complicated: done in a separate routine.

	// Check if "--" is present: if so, handle following tokens separately
	endFlags := len(tokens)
	for i, token := range tokens {
		if token == endFlagsIndicator {
			endFlags = i
			break
		}
	}

	// Handle all tokens that may be flags
	for i := 0; i < endFlags; i++ {
		info, ok := lookupFlag(tokens[i], options)

		// Not recognized as flag: treat as positional
		if !ok {
			positionalTokens = append(positionalTokens, tokens[i])
			continue
		}

		// Token is flag. Finished if boolean or value is not empty:
		if info.baseType == reflect.TypeOf(true) || info.value != "" {
			retainedOptions = append(retainedOptions, info)
			continue
		}

		// If we get here, flag still needs value. If fused mode, use default
		if isFused {
			info.value = info.defaultval

		} else { // otherwise, consume next token (if it exists!)
			i += 1
			if len(tokens) > i {
				info.value = tokens[i]
			} else {
				return nil, nil, fmt.Errorf("not enough values")
			}
		}
		retainedOptions = append(retainedOptions, info)
	}

	// Finally, handle tokens following the "--": all positional
	for i := endFlags + 1; i < len(tokens); i++ {
		positionalTokens = append(positionalTokens, tokens[i])
	}

	return retainedOptions, positionalTokens, nil
}

// Lookup flag takes a string, which should be a flag from the command line,
// and a map of fieldInfo entries. Separates flag and value (if present),
// then looks up and returns fieldInfo for flag in map. Populates the flag
// and value fields in returned fieldInfo. The boolean return value
// indicates whether the flag was found in the map; if false, the returned
// fieldInfo is empty.
// The input string should not contain whitespace.
func lookupFlag(s string, options map[string]fieldInfo) (fieldInfo, bool) {
	flag, val := "", ""

	switch {
	case strings.ContainsAny(s, " "):
		// s contains whitespace: should not happen
		return fieldInfo{}, false

	case s == "-", s == "--":
		// Not a flag: should not happen
		return fieldInfo{}, false

	case strings.HasPrefix(s, "--"):
		flag, val, _ = strings.Cut(s, "=")

	case strings.HasPrefix(s, "-"), strings.HasPrefix(s, "+"):
		flag, val = s[0:2], s[2:]

	default:
		// Not a flag
		return fieldInfo{}, false
	}

	if info, ok := options[flag]; ok {
		info.flag = flag
		info.value = val

		return info, true
	}

	// Flag not recognized
	return fieldInfo{}, false
}

// **************************************************
// with processTokens3, both processTokens1/2 AND lookupFlag can go!
// **************************************************

// ProcessTokens takes a map of fieldInfo, describing the known flags and a
// slice of tokens. Returns a slice of fieldInfo containing the recognized,
// retained options, with the fieldInfo.value being set to the supplied
// value. Tokens that are not flags or flag-values are returned as a slice
// of strings. The special token "--" indicates that all following tokens
// should be treated as positionals.
// Returns an error if there are not enough tokens, or if a compound flag
// contains an unrecognized flag.
//
// In fused mode, if a flag does not have a fused value, the default value
// for that field is used, and no additional token is consumed.
//
// Mainly, processTokens is a wrapper that handles the "--" argument.
// All the work is done by processMaybeFlags().
func processTokens3(options map[string]fieldInfo, tokens []string,
	isFused bool) ([]fieldInfo, []string, error) {

	// Note: Flags and positionals are treated differently.
	// - For flags, the "value" to assign is stored in the fieldInfo itself.
	// - For positionals, the value tokens are kept separate.
	// Options take zero or one values; in fact, values may be affixed to the
	// flag. It makes sense to keep flag and value together in fieldInfo.
	// For positionals, it is not clear which field they will be assigned to
	// until all tokens have been seen. If a slice is present, assignment of
	// values to field is even more complicated: done in a separate routine.

	// Split tokens on "--" if present: following tokens must be positionals,
	// handle separately after processing flags
	endFlags := len(tokens)
	for i, token := range tokens {
		if token == endFlagsIndicator {
			endFlags = i
			break
		}
	}

	// Process those tokens that might be flags (and positionals)
	flags, positionals, err := processMaybeFlags(tokens[:endFlags],
		options, isFused)

	// Finally, handle tokens following the "--": all positional
	for i := endFlags + 1; i < len(tokens); i++ {
		positionals = append(positionals, tokens[i])
	}

	return flags, positionals, err
}

// If the token looks like a flag (ie, has flag prefix), chop the flag part
// from the rest, and return both; otherwise, return token and empty string.
func chopToken(s string) (string, string) {
	switch {
	case s == "--", s == "-", s == "+":
		// Not a flag: should not happen
		return s, ""

	case strings.HasPrefix(s, "--"):
		flag, rest, _ := strings.Cut(s, "=")
		return flag, rest

	case strings.HasPrefix(s, "-"), strings.HasPrefix(s, "+"):
		return s[0:2], s[2:]

	default:
		// Not a flag
		return s, ""
	}
}

// ProcessMaybeFlags takes a slice of tokens, which may be a mix of flags,
// their associated values, and positional arguments, and a map of fieldInfo,
// keyed on the flag. Returns a slice of fieldInfo containing the recognized,
// retained options, with the fieldInfo.value being set to the supplied
// value. Tokens that are not flags or flag-values are returned as a slice
// of strings.
// Returns an error if there are not enough tokens, or if a compound flag
// contains an unrecognized flag.
//
// In fused mode, if a flag does not have a fused value, the default value
// for that field is used. No additional token is consumed.
func processMaybeFlags(tokens []string, options map[string]fieldInfo,
	isFused bool) ([]fieldInfo, []string, error) {

	flags, positionals := []fieldInfo{}, []string{}

	if len(tokens) == 0 {
		return flags, positionals, nil
	}

	isCompound := false
	for token := ""; len(tokens) > 0 || token != ""; {

		if token == "" {
			token, tokens = tokens[0], tokens[1:]
			isCompound = false
		}

		flag, rest := chopToken(token)
		info, ok := options[flag]

		// When parsing compound flag, all flags should be recognized
		if !ok && isCompound {
			return nil, nil, fmt.Errorf("Unexpected %s in compound flag", token)
		}

		// Not recognized as flag (known or not); treat as positional
		if !ok {
			positionals = append(positionals, token)
			token = ""
			continue
		}

		// Now: flag is a known flag. Is it complete? Is it compound?
		// Complete: boolean and rest empty OR not boolean and rest not empty
		//           this means isBoolean and isRestEmpty must be equal!
		// Compound: boolean and rest not empty
		// Incomplete: not boolean and rest empty

		// Two variables to make the switch below more readable
		isFlagBoolean := info.baseType == reflect.TypeOf(true)
		isRestEmpty := rest == ""

		switch {
		case isFlagBoolean == isRestEmpty: // Complete
			info.flag = flag
			info.value = rest
			token = ""

		case isFlagBoolean && !isRestEmpty: // Compound
			// Do NOT discard token; instead use rest to form new token!
			info.flag = flag
			info.value = ""
			token = "-" + rest

			// If compound, then all following flags must be recognized!
			isCompound = true

		case !isFlagBoolean && isRestEmpty: // Incomplete
			// If fused, use default value; otherwise use next token
			info.flag = flag

			if isFused {
				info.value = info.defaultval
				token = ""
			} else {
				if len(tokens) > 0 {
					info.value = tokens[0]
					token, tokens = "", tokens[1:]
				} else {
					return nil, nil, fmt.Errorf("not enough tokens: %s", flag)
				}
			}

		default:
			// Cannot happen, because all options are covered!
		}

		flags = append(flags, info)
	}

	return flags, positionals, nil
}

// PopulateField takes a fieldInfo and a reflect.Value, which must
// represent a pointer to the struct that is to be populated, and
// populates the struct field indicated by fieldInfo with the value
// in fieldInfo.
// The field may be a scalar or a slice.
// If the field is a slice and is nil, a new slice is created, before
// the value in fieldInfo is inserted into the slice.
// Returns an error if the value in fieldInfo can not be converted to
// the type of the field.
// Behavior undefined (may panic) if fieldInfo does not refer to an
// existing, publicly accessible field.
func populateField(info fieldInfo, v reflect.Value) error {
	// Convert the input value to the appropriate baseType,
	// then wrap the result into a reflect.Value again (also pointer)
	vv, err := convertToType(info)
	if err != nil {
		return err
	}

	field := v.FieldByName(info.Name) // field is reflect.Value

	// If field is slice and not assigned yet, create a slice of proper type
	if info.isSlice && field.IsNil() {
		field.Set(reflect.MakeSlice(reflect.SliceOf(info.baseType), 0, 0))
	}

	if info.isSlice {
		field.Set(reflect.Append(field, vv))
	} else {
		field.Set(vv)
	}

	return nil
}

// ConvertToType takes a fieldInfo, and converts its (string) value field
// into the appropriate type. If the value field is the empty string, it
// uses the default value instead.
// Conversion to time.Time type uses the given format, unless it is empty.
// Returns a reflect.Value of the converted value.
// Returns an error if the conversion fails.
func convertToType(info fieldInfo) (reflect.Value, error) {

	// Pull in default value
	value := info.value
	if value == "" {
		value = info.defaultval
	}

	switch info.baseType {
	case reflect.TypeOf(true):
		t := true
		return reflect.ValueOf(t), nil

	case reflect.TypeOf(string("")):
		return reflect.ValueOf(value), nil

	case reflect.TypeOf(int(0)):
		i, err := strconv.Atoi(value)
		if err != nil {
			return reflect.Value{}, err
		}
		return reflect.ValueOf(i), nil

	case reflect.TypeOf(float64(0.0)):
		f, err := strconv.ParseFloat(value, 64)
		if err != nil {
			return reflect.Value{}, err
		}
		return reflect.ValueOf(f), nil

	case reflect.TypeOf(time.Now()):
		format := defaultTimeFormat
		if info.format != "" {
			format = info.format
		}
		t, err := time.Parse(format, value)
		if err != nil {
			return reflect.Value{}, err
		}
		return reflect.ValueOf(t), nil

	case reflect.TypeOf(time.Duration(0)):
		d, err := time.ParseDuration(value)
		if err != nil {
			return reflect.Value{}, err
		}
		return reflect.ValueOf(d), nil

	default:
		// Never get here
		return reflect.Value{}, fmt.Errorf("invalid type")
	}
}

// PopulateOptions takes a slice of fieldInfo and a reflect.Value,
// which must represent a pointer to the struct that is to be populated,
// and populates the struct fields indicated by fieldInfo with the value
// in fieldInfo.
// Returns an error if a field cannot be populated.
// Behavior is undefined (may panic) if the indicated field can not be found.
func populateOptions(options []fieldInfo, v reflect.Value) error {
	for _, info := range options {
		if err := populateField(info, v); err != nil {
			return err
		}
	}

	return nil
}

// PopulatePositionals takes a slice of fieldInfo, a slice of string
// tokens (representing the values to be assigned to the positional fields),
// and reflect.Value, which must represent a pointer to the struct to
// be populated, and populates the positional fields in the struct with
// the values from the slice of strings.
// At most one of the positional fields can be a slice. If a slice is
// present, tokens are assigned to the non-slice positional fields before
// and after the slice (starting from the beginning or end of the slice of
// tokens, respectively). Any remaining tokens are assigned to the slice.
// Returns an error if
// - any one of the tokens cannot be converted to the required data type
// - more than one slice is present in the list of positional fields
// - if the number of tokens does not equal the number of positional fields
//   (in case no slice is present)
// - if there are fewer tokens than fields, even if the slice is left empty
//   (in case there is a slice)
// Positional arguments should not be pointers (semantics are not clear!)
func populatePositionals(positionals []fieldInfo, tokens []string,
	v reflect.Value) error {

	// Find position of slice, if any, among positional fields
	pos, cnt := 0, 0
	for i, p := range positionals {
		if p.isSlice {
			pos = i
			cnt += 1
		}
	}
	if cnt > 1 {
		return fmt.Errorf("at most one positional may be slice")
	}

	// No slice
	if cnt == 0 {
		if len(positionals) != len(tokens) {
			s := "number of positional fields does not match number of tokens"
			return fmt.Errorf(s)
		}

		for i, t := range tokens {
			positionals[i].value = t
			if err := populateField(positionals[i], v); err != nil {
				return fmt.Errorf("error populating positional field %d", i)
			}
		}

		return nil
	}

	// One slice
	before := pos                           // fields before: positionals[:pos]
	after := len(positionals) - (pos + 1)   // fields after: positionals[pos+1:]
	between := len(tokens) - before - after // tokens (!) to put into slice

	if between < 0 {
		return fmt.Errorf("not enough tokens to fill all positional fields")
	}

	for i := 0; i < before; i++ {
		positionals[i].value = tokens[i]
		if err := populateField(positionals[i], v); err != nil {
			return fmt.Errorf("error populating positional field %d", i)
		}
	}

	for i := 0; i < between; i++ {
		positionals[pos].value = tokens[pos+i]
		if err := populateField(positionals[pos], v); err != nil {
			return fmt.Errorf("error populating slice of positionals")
		}
	}

	src, dst := len(tokens)-after, pos+1 // offsets
	for i := 0; i < after; i++ {
		positionals[dst+i].value = tokens[src+i]
		if err := populateField(positionals[dst+i], v); err != nil {
			return fmt.Errorf("error populating positional field %d", dst+i)
		}
	}

	return nil
}

// FromSlice takes a pointer to a struct and populates the struct by
// processing a slice of string tokens.
// The tokens may be a mix of command-line flags and their assigned
// values (if any), as well as positional arguments.
// Returns an error if the struct contains unsupported data types, if
// the number of tokens does not match the number of fields in the struct,
// or if any of the type conversions fails.
func FromSlice(tokens []string, data any) error {
	return populateFromSlice(tokens, data, false)
}

// FromCommandLine takes a pointer to a struct and populates the struct
// with the command-line arguments.
// The tokens may be a mix of command-line flags and their assigned
// values (if any), as well as positional arguments.
// Returns an error if the struct contains unsupported data types, if
// the number of tokens does not match the number of fields in the struct,
// or if any of the type conversions fails.
func FromCommandLine(data any) error {
	return populateFromSlice(os.Args[1:], data, false)
}

// FromSliceFused takes a pointer to a struct and populates the struct
// by processing the slice of string tokens in fused mode.
// In fused mode, all flag arguments must be fused to their flag, without
// intervening whitespace. The tokens may be a mix of command-line flags
// and positional arguments.
// Returns an error if the struct contains unsupported data types, if
// the number of tokens does not match the number of fields in the struct,
// or if any of the type conversions fails.
func FromSliceFused(tokens []string, data any) error {
	return populateFromSlice(tokens, data, true)
}

// FromSliceFused takes a pointer to a struct and populates the struct
// with the command-line arguments.
// In fused mode, all flag arguments must be fused to their flag, without
// intervening whitespace. The tokens may be a mix of command-line flags
// and positional arguments.
// Returns an error if the struct contains unsupported data types, if
// the number of tokens does not match the number of fields in the struct,
// or if any of the type conversions fails.
func FromCommandLineFused(data any) error {
	return populateFromSlice(os.Args[1:], data, true)
}

// PrintShortUsage takes a pointer to a struct and writes a one-line
// description of the identified options and positional fields to
// standard error.
// Returns an error if the struct contains unsupported types.
func PrintShortUsage(data any) error {
	return WriteShortUsage(os.Stderr, data)
}

// WriteShortUsage takes a pointer to a struct and writes a one-line
// description of the identified options and positional fields to w.
// Returns an error if the struct contains unsupported types.
func WriteShortUsage(w io.Writer, data any) error {
	v, err := unwrap(data)
	if err != nil {
		return err
	}

	options, positionals, err := analyzeStruct(v)
	if err != nil {
		return err
	}

	keys := sortableFlags{}
	for k, _ := range options {
		keys = append(keys, k)
	}
	sort.Sort(keys)

	// Options
	seen := map[string]struct{}{}
	for _, k := range keys {
		if _, ok := seen[k]; ok {
			continue
		}

		info := options[k]

		// Print all flags as one line, space-separated (also: remember!)
		for _, f := range info.allFlags {
			seen[f] = struct{}{}
		}
		fmt.Fprintf(w, "[%s", strings.Join(info.allFlags, "|"))

		_, argname := formatHelp(info, false)

		// Don't print argument for booleans; otherwise, print arg
		if info.baseType != reflect.TypeOf(true) {
			fmt.Fprintf(w, " %s", argname)
		}
		fmt.Fprintf(w, "]")
		if info.isSlice {
			fmt.Fprintf(w, "+")
		}
		fmt.Fprintf(w, " ")
	}

	// Positionals
	for _, p := range positionals {
		_, argname := formatHelp(p, true)

		fmt.Fprintf(w, "[%s]", argname)
		if p.isSlice {
			fmt.Fprintf(w, "+")
		}
		fmt.Fprintf(w, " ")
	}

	fmt.Fprintf(w, "\n")

	return nil
}

// PrintUsage takes a pointer to a struct and writes a detailed description
// of the identified options and positional fields, including the help text
// provided by the arg-help tag, to standard error.
// Returns an error if the struct contains unsupported types.
func PrintUsage(data any) error {
	return WriteUsage(os.Stderr, data)
}

// WriteUsage takes a pointer to a struct and writes a detailed description
// of the identified options and positional fields, including the help text
// provided by the arg-help tag, to w.
// Returns an error if the struct contains unsupported types.
func WriteUsage(w io.Writer, data any) error {
	v, err := unwrap(data)
	if err != nil {
		return err
	}

	options, positionals, err := analyzeStruct(v)
	if err != nil {
		return err
	}

	keys := sortableFlags{}
	for k, _ := range options {
		keys = append(keys, k)
	}
	sort.Sort(keys)

	// Options
	seen := map[string]struct{}{}
	for _, k := range keys {
		if _, ok := seen[k]; ok {
			continue
		}

		info := options[k]

		// Indent
		fmt.Fprintf(w, "    ")

		// Print all flags as one line, space-separated (also: remember!)
		for _, f := range info.allFlags {
			seen[f] = struct{}{}
			fmt.Fprintf(w, "%s ", f)
		}

		help, argname := formatHelp(info, false)
		defval := ""
		if info.defaultval != "" {
			defval = "=" + info.defaultval
		}

		// Don't print argument for booleans; otherwise, print arg
		if info.baseType != reflect.TypeOf(true) {
			fmt.Fprintf(w, "[%s%s]", argname, defval)
		}
		if info.isSlice {
			fmt.Fprintf(w, " (repeatable)")
		}

		// Print actual help text (if any!), on new line, indented
		if help != "" {
			fmt.Fprintf(w, "\n       %s", help)
		}

		// Newline
		fmt.Fprintf(w, "\n")
	}

	// Positionals
	for _, p := range positionals {
		help, argname := formatHelp(p, true)

		fmt.Fprintf(w, "    [%s] ", argname)
		if p.isSlice {
			fmt.Fprintf(w, "(repeatable) ")
		}
		fmt.Fprintf(w, "%s\n", help)
	}

	return nil
}

// FormatHelp extracts the help text (if any) from the tag values of the
// supplied field info. If the help text contains a term inclosed by special
// delimiters, that term is extracted and the the delimiters removed from the
// help text. The modified help text, and the extracted term, are returned.
// If the help text is empty or no term was identified in the text, the base
// type for the field is returned instead. If the help text is empty and the
// useName flag is true the field name of the field is substituted for the
// help text.
func formatHelp(info fieldInfo, useName bool) (string, string) {
	help, argname := info.help, info.baseType.String()

	if limits := helpArgumentRE.FindStringIndex(info.help); limits != nil {
		argname = help[limits[0]+1 : limits[1]-1]
		help = strings.ReplaceAll(help, helpDelimiter, "")
	}

	if help == "" && useName {
		help = info.Name
	}

	return help, argname
}

// PrintValues takes a pointer to a populated struct and writes the names
// and types of its fields, together with their current values, to standard
// error.
// Returns an error if the struct contains non-ignored unsupported types.
func PrintValues(data any) error {
	return writeValues(os.Stderr, data, false)
}

// WriteValues takes a pointer to a populated struct and writes the names
// and types of its fields, together with their current values, to w.
// Returns an error if the struct contains non-ignored unsupported types.
func WriteValues(w io.Writer, data any) error {
	return writeValues(w, data, false)
}

// PrintValuesWithTags takes a pointer to a populated struct and writes the
// names, types, and struct tags of its fields, together with their current
// values, to standard error.
// Returns an error if the struct contains non-ignored unsupported types.
func PrintValuesWithTags(data any) error {
	return writeValues(os.Stderr, data, true)
}

// WriteValuesWithTags takes a pointer to a populated struct and writes the
// names, types, and struct tags of its fields, together with their current
// values, to w.
// Returns an error if the struct contains non-ignored unsupported types.
func WriteValuesWithTags(w io.Writer, data any) error {
	return writeValues(w, data, true)
}

func writeValues(w io.Writer, data any, withTags bool) error {
	v, err := unwrap(data)
	if err != nil {
		return err
	}

	typeInfo := v.Type()

	// Find max length of field names, types, and values
	mxName, mxType, mxVal := 0, 0, 0
	for i := 0; i < v.NumField(); i++ {
		field := typeInfo.Field(i)

		if len(field.Name) > mxName {
			mxName = len(field.Name)
		}

		if len(field.Type.String()) > mxType {
			mxType = len(field.Type.String())
		}

		tmp := len(fmt.Sprintf("%v", v.Field(i)))
		if tmp > mxVal {
			mxVal = tmp
		}
	}

	for i := 0; i < v.NumField(); i++ {
		field := typeInfo.Field(i)

		tag := ""
		if withTags {
			tag = string(field.Tag)
		}

		fmt.Fprintf(w, "%-*s   %-*s   %-*s   %s\n",
			mxName, field.Name, mxType, field.Type.String(),
			mxVal, fmt.Sprintf("%v", v.Field(i)), tag)
	}

	return nil
}
