/*
Package cleanarg implements a minimal command-line parser. Given a
struct definition with appropriate struct tags, the package will
populate the fields of the struct with data read from the command
line.


# Supported Data Types

The following data types can be used as struct fields, and command-line
tokens will automatically be converted to the appropriate type:

  bool
  int
  float64
  string
  time.Time
  time.Duration

It is also possible to use a slice of any of the above types to allow
for repeated flags, or to allow for a variable and/or unknown number of
arguments.


# Struct Tags

The following struct tags may be used:

  arg-flag    : The command-line flags to set this field, as a whitespace separated string.
  arg-help    : A help text that will be displayed by PrintUsage().
  arg-default : A default value for this field, in case it is not set explicitly on the command line.
  arg-format  : A custom format string (only used for fields of type time.Time).
  arg-ignore  : Ignore this field, do not populate it, do not treat it as positional argument.

Positional fields do not need to be indicated explicitly.

The default date format is "YYYY-MM-DD hh:mm:ss" ("2006-01-02 15:04:05"),
without timezone indicator. To support a different date format, set the
arg-format tag to a value that is recognized by the time.Parse() function.

If the help text contains a substring enclosed by a pair of "*", then the
first occurrence of such a substring will be substituted for the field's
type in the usage messages created by PrintUsage() and related functions.

Remember that struct fields must be public (ie. upper-case) to be
accessible!


# Permissible Flag Formats and Command-Line Processing

Both short (single character) and long flags can be used.

Short flags must begin with either "-" or "-", long flags must
begin with "--". It is possible to define multiple flags for a single
field as white-space separated string, following the arg-flag tag.
All flags for a single field will be treated equally; it is not possible
to assign different semantics to different flags.

Digits, lower and upper case characters may be used as flags; long
flags may also contain a hyphen (but not as first character after
the leading "--").

Flag names are entirely independent from their associated field names!
Flag names are not inferred from field names, and must be set explicitly,
but can be chosen arbitrarily (as long as they are well-formatted). This
avoids confusion about upper- vs lower-case field names and their
associated flags.

Unrecognized flags are treated as positional arguments.


# Flag Processing

All flags, except those belonging boolean fields, require an argument.

If a flag takes an argument, the argument may normally either be separated
from the flag by whitespace (eg. "-c 9" or "--counter 9"") or follow the
flag without whitespace (eg. "-c9" or "--counter=9"). Note that long flags
names require an additional equality sign in the latter case. If a flag does
not appear in the slice of tokens, its corresponding field will be set to the
value defined by the arg-default tag, or to the null value of its type.

There is an alternative, "fused" mode of processing tokens. In fused mode,
a flag's argument must be fused to the flag without intervening whitespace
(eg. "-c9" or "--counter=9"). The default value is handled differently in
this case: only if the flag is present without a fused value (eg. "-c" or
"--counter"), is the corresponding field is set the default value. If the
flag is absent, the field is set to its null value; if a fused value is
present, the field is set to that value.

The special token "--" indicates that all following command-line
arguments should be treated as positionals. If more than one "--"
is present, the left-most one prevails.

Short flags (like "-a -b -c") may be combined into compound flags
(like "-abc") on the command-line. All flags, except the last one,
must be boolean. Compound flags like `-abc` are processed left-to-right;
as soon as a non-boolean flag is encountered, processing stops, and the
remaining characters are considered the argument to this non-boolean flag.


# Slices, Repeated Arguments, and Trailing Positionals

If a struct field is a slice of one of the permitted data types,
the corresponding flag may be repeated on the command line. In this
case, each occurrence appends the supplied value to the slice.

For example, to allow repeated use of the "-v" to indicate increased
verbosity level, use the following idiom:

    import "github.com/janert/cleanarg"

    type Config struct {
        VerbosityFlags []bool `arg-flag:"-v"`
    	VerbosityLevel int    `arg-ignore:""`
    }

    c := Config{}
    cleanarg.FromCommandLine(&c)

    c.VerbosityLevel = len(c.VerbosityFlags)

At most one positional argument may be a slice. In this case,
all command-line tokens that cannot be assigned unambiguously to
another field are collected in this slice. This is useful for
collecting a varying number of command-line arguments, as may
result from the use of shell wildcards (think "ls *").

The positional slice need not be the last field in the struct.
Command-line tokens will be assigned to the non-slice fields
before and after the slice first, starting from the beginning
or the end of the command line, respectively. Any remaining
tokens in the middle will be assigned to the slice.
*/
package cleanarg
