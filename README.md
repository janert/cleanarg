
# Clean Arg

Minimalistic command-line parser


## Overview

This package implements a minimalistic, zero-dependency command-line parser.

To use this package:
- define a struct type, using _struct tags_ to define command-line flags, 
  help texts, etc
- given a pointer to an instance, the `FromCommandLine()` function will 
  populate the struct instance using the command line arguments
- use `PrintUsage()` and `PrintValues()` to display a pre-formatted
  help text or to show the parsed values for review
  

## Installation

```
go get github.com/janert/cleanarg
```


## Usage

```go
import "github.com/janert/cleanarg"

type Config struct {
    ShowHelp bool    `arg-flag:"-h --help"`
    Counter  int     `arg-flag:"-c --counter" arg-default:"2"`
    Factor   float64 `arg-flag:"--factor" arg-help:"Floating-point factor"`
    Username string  `arg-flag:"-n" arg-default:"nobody"`
    Files    []string
}

func main() {
    c := Config{}

    cleanarg.FromCommandLine(&c)

    if c.ShowHelp {
        cleanarg.PrintUsage(&c)
    }

    cleanarg.PrintValues(&c)
}
```

### Features

- Supports both short and long flags (eg. `-h` and `--help`)
- Infers the type of the field and converts command-line tokens automatically
- Allows to include help text and default values as part of the struct 
  definition
- Supports repeated arguments
- Positional arguments do not need to be indicated explicitly
- Positional arguments and flags can be intermixed on the command line
- Includes pre-formatted help/usage text, and a pre-formatted display of
  the values assigned to the struct


## Reference

### Supported Types

The following data types can be used in the struct, and command-line tokens
will automatically be converted to the appropriate type:

- `bool`
- `int`
- `float64`
- `string`
- `time.Time`
- `time.Duration`

It is also possible to use a _slice_ of any of the above types to allow
for repeated flags, or to allow for a variable and/or unknown number of 
arguments.


### Struct Tags

The following _struct tags_ may be used:

- `arg-flag`: The command-line flags to set this field, as a whitespace
  separated string. (See below for details on permissible flag formats.)
- `arg-help`: A help text that will be displayed by `PrintUsage()`.
- `arg-default`: A default value for this field, in case it is not set
  explicitly on the command line.
- `arg-format`: A custom format string (currently only used for fields
  of type `time.Time`).
- `arg-ignore`: Ignore this field, do not populate it, do not treat it as
  positional argument.

Positional fields do not need to be indicated explicitly.

_Remember that struct fields must be public (ie. upper-case) to be
accessible!_


### Permissible Flag Formats

Both short (single character) and long flags can be used. 

Short flags _must_ begin with either `-` or `+`, long flags _must_
begin with `--`. It is possible to define multiple flags for a single
field (eg: `arg-flag:"-c --counter +C"` defines three different flags). 
All flags for a single field will be treated equally; it is not possible 
to assign different semantics to different flags. (Use separate fields 
for that.)

Digits, lower and upper case characters may be used as flags; long
flags may also contain a hyphen (but not as first character after
the leading `--`).

_Flag names are entirely independent from their associated field names!_
Flag names are not inferred from field names, and must be set explicitly,
but can be chosen arbitrarily (as long as they are well-formatted). (Also
see Limitations, below.)

If a flag takes a value, it may either be separated from the flag by
whitespace (eg. `-c 9` or `--counter 9`) or follow the flag without
whitespace (eg. `-c9` or `--counter=9` &mdash; note that long flags names
require an additional equality sign in this case).

The special token `--` indicates that all following command-line 
arguments should be treated as positionals.


### Slices, Repeated Arguments, and Trailing Positionals

If a struct field is a _slice_ of one of the permitted data types,
the corresponding flag may be repeated on the command line. In this
case, each occurrence appends the supplied value to the slice. 

For example, to allow repeated use of the `-v` to indicate increased
verbosity level, use the following idiom:

```go
import "github.com/janert/cleanarg"

type Config struct {
    VerbosityFlags []bool `arg-flag:"-v"`
    VerbosityLevel int    `arg-ignore:""`
}

func main() {
    c := Config{}
    cleanarg.FromCommandLine(&c)

    c.VerbosityLevel = len(c.VerbosityFlags)
}
```

At most one _positional_ argument may be a slice. In this case,
all command-line tokens that cannot be assigned unambiguously to
another field are collected in this slice. This is useful for
collecting a varying number of command-line arguments, as may 
result from the use of shell wildcards (think `ls *`).

The positional slice need not be the last field in the struct.
For example, in the following situation, the slice will only
hold the values `file2` and `file3`, whereas `file1` and `file4`
will be assigned to `Before` and `After`, respectively.

```go
import "github.com/janert/cleanarg"

type Config struct {
    Before string
    Middle []string
    After  string
}

func main() {
    c := Config{}
    cleanarg.FromSlice([]string{"file1", "file2", "file3", "file4"}, &c)
}
```


## Limitations

Intentional and by design:

- Flag names are not inferred from field names, and have to be set
  explicitly. The primary reason is to avoid any confusion between
  upper- versus lower-case spelling of fields and command-line flags.
  Additional benefits are the ability to define _multiple_ flags for
  a single field (for example, long and short formats), and the 
  freedom to chose both field and flag names to be most convenient
  in their respective usage patterns.
- Although the package provides a formatted help or usage message,
  it does not automatically add a `-h` or `--help` flag. It is up to 
  the user to define such a flag, and to trigger the display of the 
  help message, as appropriate. 
- The package performs type conversion from `string` to one of the
  permitted types, but does no other validation. This keeps the
  package simple to use, and encourages separation of concerns: the
  package reads the command line, but the package has no way of 
  knowing what the application considers valid input!
- The package does not break up sub-options such as `--pages 1,2,17-25` 
  into slices. Again, it is up to the application to define the 
  semantics (is `17-25` a range or an arithmetic expression?).
  
Also remember:

- Struct fields must be public (upper-case) to be accessible.
- Positional arguments (unless slice) are mandatory on the command line.


## Bugs

Likely to change in the future:

- The value supplied to an option must not contain whitespace.

- It is not possible to combine multiple short options into one
  (ie. `-a -b -c` cannot be written as `-abc`).


## Why Another One?

The primary motivation was to develop a solution that is _easy to use
for the developer_, without the need to digest lengthy documentation
(after all, all I want is parse a command line!), or requiring the
developer to provide information that the computer already has (such 
as information about data types in a struct), while at the same time
preserving maximal flexibility in the permissible format of the 
command-line arguments.

All of the extant packages seemed to fall short on at least _one_ of
these criteria, hence there seemed to be room for one more "better 
mousetrap".


## Version Support

This module requires Go 1.21 or later. (In fact, the actual logic works
with Go 1.18 or possibly earlier, but the unit testing code requires 
features of the standard library that were not added until Go 1.21.)


## Contributing

Comments, suggestions, fixes, enhancements welcome!


## Acknowledgments

This project was inspired by the wonderfully simple and practical
[cleanenv](https://github.com/ilyakaznacheev/cleanenv) package.

This project was also influenced by the 
[argparse](https://docs.python.org/3/library/argparse.html)
package in the Python standard library, while trying to avoid
some of that package's complexity.
