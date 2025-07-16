# String Functions

Sprig has a number of string manipulation functions.

## trim

The `trim` function removes space from either side of a string:

```
trim "   hello    "
```

The above produces `hello`

## trimAll

Remove given characters from the front or back of a string:

```
trimAll "$" "$5.00"
```

The above returns `5.00` (as a string).

## trimSuffix

Trim just the suffix from a string:

```
trimSuffix "-" "hello-"
```

The above returns `hello`

## trimPrefix

Trim just the prefix from a string:

```
trimPrefix "-" "-hello"
```

The above returns `hello`

## upper

Convert the entire string to uppercase:

```
upper "hello"
```

The above returns `HELLO`

## lower

Convert the entire string to lowercase:

```
lower "HELLO"
```

The above returns `hello`

## title

Convert to title case:

```
title "hello world"
```

The above returns `Hello World`

## repeat

Repeat a string multiple times:

```
repeat 3 "hello"
```

The above returns `hellohellohello`

## substr

Get a substring from a string. It takes three parameters:

- start (int)
- end (int)
- string (string)

```
substr 0 5 "hello world"
```

The above returns `hello`

## trunc

Truncate a string (and add no suffix)

```
trunc 5 "hello world"
```

The above produces `hello`.

```
trunc -5 "hello world"
```

The above produces `world`.

## contains

Test to see if one string is contained inside of another:

```
contains "cat" "catch"
```

The above returns `true` because `catch` contains `cat`.

## hasPrefix and hasSuffix

The `hasPrefix` and `hasSuffix` functions test whether a string has a given
prefix or suffix:

```
hasPrefix "cat" "catch"
```

The above returns `true` because `catch` has the prefix `cat`.

## quote and squote

These functions wrap a string in double quotes (`quote`) or single quotes
(`squote`).

## cat

The `cat` function concatenates multiple strings together into one, separating
them with spaces:

```
cat "hello" "beautiful" "world"
```

The above produces `hello beautiful world`

## indent

The `indent` function indents every line in a given string to the specified
indent width. This is useful when aligning multi-line strings:

```
indent 4 $lots_of_text
```

The above will indent every line of text by 4 space characters.

## nindent

The `nindent` function is the same as the indent function, but prepends a new
line to the beginning of the string.

```
nindent 4 $lots_of_text
```

The above will indent every line of text by 4 space characters and add a new
line to the beginning.

## replace

Perform simple string replacement.

It takes three arguments:

- string to replace
- string to replace with
- source string

```
"I Am Henry VIII" | replace " " "-"
```

The above will produce `I-Am-Henry-VIII`

## plural

Pluralize a string.

```
len $fish | plural "one anchovy" "many anchovies"
```

In the above, if the length of the string is 1, the first argument will be
printed (`one anchovy`). Otherwise, the second argument will be printed
(`many anchovies`).

The arguments are:

- singular string
- plural string
- length integer

NOTE: Sprig does not currently support languages with more complex pluralization
rules. And `0` is considered a plural because the English language treats it
as such (`zero anchovies`). The Sprig developers are working on a solution for
better internationalization.

## regexMatch, mustRegexMatch

Returns true if the input string contains any match of the regular expression.

```
regexMatch "^[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\\.[A-Za-z]{2,}$" "test@acme.com"
```

The above produces `true`

`regexMatch` panics if there is a problem and `mustRegexMatch` returns an error to the
template engine if there is a problem.

## regexFindAll, mustRegexFindAll

Returns a slice of all matches of the regular expression in the input string.
The last parameter n determines the number of substrings to return, where -1 means return all matches

```
regexFindAll "[2,4,6,8]" "123456789" -1
```

The above produces `[2 4 6 8]`

`regexFindAll` panics if there is a problem and `mustRegexFindAll` returns an error to the
template engine if there is a problem.

## regexFind, mustRegexFind

Return the first (left most) match of the regular expression in the input string

```
regexFind "[a-zA-Z][1-9]" "abcd1234"
```

The above produces `d1`

`regexFind` panics if there is a problem and `mustRegexFind` returns an error to the
template engine if there is a problem.

## regexReplaceAll, mustRegexReplaceAll

Returns a copy of the input string, replacing matches of the Regexp with the replacement string replacement.
Inside string replacement, $ signs are interpreted as in Expand, so for instance $1 represents the text of the first submatch

```
regexReplaceAll "a(x*)b" "-ab-axxb-" "${1}W"
```

The above produces `-W-xxW-`

`regexReplaceAll` panics if there is a problem and `mustRegexReplaceAll` returns an error to the
template engine if there is a problem.

## regexReplaceAllLiteral, mustRegexReplaceAllLiteral

Returns a copy of the input string, replacing matches of the Regexp with the replacement string replacement
The replacement string is substituted directly, without using Expand

```
regexReplaceAllLiteral "a(x*)b" "-ab-axxb-" "${1}"
```

The above produces `-${1}-${1}-`

`regexReplaceAllLiteral` panics if there is a problem and `mustRegexReplaceAllLiteral` returns an error to the
template engine if there is a problem.

## regexSplit, mustRegexSplit

Slices the input string into substrings separated by the expression and returns a slice of the substrings between those expression matches. The last parameter `n` determines the number of substrings to return, where `-1` means return all matches

```
regexSplit "z+" "pizza" -1
```

The above produces `[pi a]`

`regexSplit` panics if there is a problem and `mustRegexSplit` returns an error to the
template engine if there is a problem.

## regexQuoteMeta

Returns a string that escapes all regular expression metacharacters inside the argument text;
the returned string is a regular expression matching the literal text.

```
regexQuoteMeta "1.2.3"
```

The above produces `1\.2\.3`

## See Also...

The [Conversion Functions](conversion.md) contain functions for converting strings. The [String List Functions](string_slice.md) contains
functions for working with an array of strings.
# String List Functions

These function operate on or generate slices of strings. In Go, a slice is a
growable array. In Sprig, it's a special case of a `list`.

## join

Join a list of strings into a single string, with the given separator.

```
list "hello" "world" | join "_"
```

The above will produce `hello_world`

`join` will try to convert non-strings to a string value:

```
list 1 2 3 | join "+"
```

The above will produce `1+2+3`

## splitList and split

Split a string into a list of strings:

```
splitList "$" "foo$bar$baz"
```

The above will return `[foo bar baz]`

The older `split` function splits a string into a `dict`. It is designed to make
it easy to use template dot notation for accessing members:

```
$a := split "$" "foo$bar$baz"
```

The above produces a map with index keys. `{_0: foo, _1: bar, _2: baz}`

```
$a._0
```

The above produces `foo`

## splitn

`splitn` function splits a string into a `dict` with `n` keys. It is designed to make
it easy to use template dot notation for accessing members:

```
$a := splitn "$" 2 "foo$bar$baz"
```

The above produces a map with index keys. `{_0: foo, _1: bar$baz}`

```
$a._0
```

The above produces `foo`

## sortAlpha

The `sortAlpha` function sorts a list of strings into alphabetical (lexicographical)
order.

It does _not_ sort in place, but returns a sorted copy of the list, in keeping
with the immutability of lists.
# Integer Math Functions

The following math functions operate on `int64` values.

## add

Sum numbers with `add`. Accepts two or more inputs.

```
add 1 2 3
```

## add1

To increment by 1, use `add1`

## sub

To subtract, use `sub`

## div

Perform integer division with `div`

## mod

Modulo with `mod`

## mul

Multiply with `mul`. Accepts two or more inputs.

```
mul 1 2 3
```

## max

Return the largest of a series of integers:

This will return `3`:

```
max 1 2 3
```

## min

Return the smallest of a series of integers.

`min 1 2 3` will return `1`

## floor

Returns the greatest float value less than or equal to input value

`floor 123.9999` will return `123.0`

## ceil

Returns the greatest float value greater than or equal to input value

`ceil 123.001` will return `124.0`

## round

Returns a float value with the remainder rounded to the given number to digits after the decimal point.

`round 123.555555 3` will return `123.556`

## randInt
Returns a random integer value from min (inclusive) to max (exclusive).

```
randInt 12 30
```

The above will produce a random number in the range [12,30].
# Integer List Functions

## until

The `until` function builds a range of integers.

```
until 5
```

The above generates the list `[0, 1, 2, 3, 4]`.

This is useful for looping with `range $i, $e := until 5`.

## untilStep

Like `until`, `untilStep` generates a list of counting integers. But it allows
you to define a start, stop, and step:

```
untilStep 3 6 2
```

The above will produce `[3 5]` by starting with 3, and adding 2 until it is equal
or greater than 6. This is similar to Python's `range` function.

## seq

Works like the bash `seq` command.
* 1 parameter  (end) - will generate all counting integers between 1 and `end` inclusive.
* 2 parameters (start, end) - will generate all counting integers between `start` and `end` inclusive incrementing or decrementing by 1.
* 3 parameters (start, step, end) - will generate all counting integers between `start` and `end` inclusive incrementing or decrementing by `step`.

```
seq 5       => 1 2 3 4 5
seq -3      => 1 0 -1 -2 -3
seq 0 2     => 0 1 2
seq 2 -2    => 2 1 0 -1 -2
seq 0 2 10  => 0 2 4 6 8 10
seq 0 -2 -5 => 0 -2 -4
```
# Date Functions

## now

The current date/time. Use this in conjunction with other date functions.

## ago

The `ago` function returns duration from time.Now in seconds resolution.

```
ago .CreatedAt
```

returns in `time.Duration` String() format

```
2h34m7s
```

## date

The `date` function formats a date.

Format the date to YEAR-MONTH-DAY:

```
now | date "2006-01-02"
```

Date formatting in Go is a [little bit different](https://pauladamsmith.com/blog/2011/05/go_time.html).

In short, take this as the base date:

```
Mon Jan 2 15:04:05 MST 2006
```

Write it in the format you want. Above, `2006-01-02` is the same date, but
in the format we want.

## dateInZone

Same as `date`, but with a timezone.

```
dateInZone "2006-01-02" (now) "UTC"
```

## duration

Formats a given amount of seconds as a `time.Duration`.

This returns 1m35s

```
duration "95"
```

## durationRound

Rounds a given duration to the most significant unit. Strings and `time.Duration`
gets parsed as a duration, while a `time.Time` is calculated as the duration since.

This return 2h

```
durationRound "2h10m5s"
```

This returns 3mo

```
durationRound "2400h10m5s"
```

## unixEpoch

Returns the seconds since the unix epoch for a `time.Time`.

```
now | unixEpoch
```

## dateModify, mustDateModify

The `dateModify` takes a modification and a date and returns the timestamp.

Subtract an hour and thirty minutes from the current time:

```
now | date_modify "-1.5h"
```

If the modification format is wrong `dateModify` will return the date unmodified. `mustDateModify` will return an error otherwise.

## htmlDate

The `htmlDate` function formats a date for inserting into an HTML date picker
input field.

```
now | htmlDate
```

## htmlDateInZone

Same as htmlDate, but with a timezone.

```
htmlDateInZone (now) "UTC"
```

## toDate, mustToDate

`toDate` converts a string to a date. The first argument is the date layout and
the second the date string. If the string can't be convert it returns the zero
value.
`mustToDate` will return an error in case the string cannot be converted.

This is useful when you want to convert a string date to another format
(using pipe). The example below converts "2017-12-31" to "31/12/2017".

```
toDate "2006-01-02" "2017-12-31" | date "02/01/2006"
```
# Default Functions

Sprig provides tools for setting default values for templates.

## default

To set a simple default value, use `default`:

```
default "foo" .Bar
```

In the above, if `.Bar` evaluates to a non-empty value, it will be used. But if
it is empty, `foo` will be returned instead.

The definition of "empty" depends on type:

- Numeric: 0
- String: ""
- Lists: `[]`
- Dicts: `{}`
- Boolean: `false`
- And always `nil` (aka null)

For structs, there is no definition of empty, so a struct will never return the
default.

## empty

The `empty` function returns `true` if the given value is considered empty, and
`false` otherwise. The empty values are listed in the `default` section.

```
empty .Foo
```

Note that in Go template conditionals, emptiness is calculated for you. Thus,
you rarely need `if empty .Foo`. Instead, just use `if .Foo`.

## coalesce

The `coalesce` function takes a list of values and returns the first non-empty
one.

```
coalesce 0 1 2
```

The above returns `1`.

This function is useful for scanning through multiple variables or values:

```
coalesce .name .parent.name "Matt"
```

The above will first check to see if `.name` is empty. If it is not, it will return
that value. If it _is_ empty, `coalesce` will evaluate `.parent.name` for emptiness.
Finally, if both `.name` and `.parent.name` are empty, it will return `Matt`.

## all

The `all` function takes a list of values and returns true if all values are non-empty.

```
all 0 1 2
```

The above returns `false`.

This function is useful for evaluating multiple conditions of variables or values:

```
all (eq .Request.TLS.Version 0x0304) (.Request.ProtoAtLeast 2 0) (eq .Request.Method "POST")
```

The above will check http.Request is POST with tls 1.3 and http/2.

## any

The `any` function takes a list of values and returns true if any value is non-empty.

```
any 0 1 2
```

The above returns `true`.

This function is useful for evaluating multiple conditions of variables or values:

```
any (eq .Request.Method "GET") (eq .Request.Method "POST") (eq .Request.Method "OPTIONS")
```

The above will check http.Request method is one of GET/POST/OPTIONS.

## fromJSON, mustFromJSON

`fromJSON` decodes a JSON document into a structure. If the input cannot be decoded as JSON the function will return an empty string.
`mustFromJSON` will return an error in case the JSON is invalid.

```
fromJSON "{\"foo\": 55}"
```

## toJSON, mustToJSON

The `toJSON` function encodes an item into a JSON string. If the item cannot be converted to JSON the function will return an empty string.
`mustToJSON` will return an error in case the item cannot be encoded in JSON.

```
toJSON .Item
```

The above returns JSON string representation of `.Item`.

## toPrettyJSON, mustToPrettyJSON

The `toPrettyJSON` function encodes an item into a pretty (indented) JSON string.

```
toPrettyJSON .Item
```

The above returns indented JSON string representation of `.Item`.

## toRawJSON, mustToRawJSON

The `toRawJSON` function encodes an item into JSON string with HTML characters unescaped.

```
toRawJSON .Item
```

The above returns unescaped JSON string representation of `.Item`.

## ternary

The `ternary` function takes two values, and a test value. If the test value is
true, the first value will be returned. If the test value is empty, the second
value will be returned. This is similar to the c ternary operator.

### true test value

```
ternary "foo" "bar" true
```

or

```
true | ternary "foo" "bar"
```

The above returns `"foo"`.

### false test value

```
ternary "foo" "bar" false
```

or

```
false | ternary "foo" "bar"
```

The above returns `"bar"`.
# Encoding Functions

Sprig has the following encoding and decoding functions:

- `b64enc`/`b64dec`: Encode or decode with Base64
- `b32enc`/`b32dec`: Encode or decode with Base32
# Lists and List Functions

Sprig provides a simple `list` type that can contain arbitrary sequential lists
of data. This is similar to arrays or slices, but lists are designed to be used
as immutable data types.

Create a list of integers:

```
$myList := list 1 2 3 4 5
```

The above creates a list of `[1 2 3 4 5]`.

## first, mustFirst

To get the head item on a list, use `first`.

`first $myList` returns `1`

`first` panics if there is a problem while `mustFirst` returns an error to the
template engine if there is a problem.

## rest, mustRest

To get the tail of the list (everything but the first item), use `rest`.

`rest $myList` returns `[2 3 4 5]`

`rest` panics if there is a problem while `mustRest` returns an error to the
template engine if there is a problem.

## last, mustLast

To get the last item on a list, use `last`:

`last $myList` returns `5`. This is roughly analogous to reversing a list and
then calling `first`.

`last` panics if there is a problem while `mustLast` returns an error to the
template engine if there is a problem.

## initial, mustInitial

This compliments `last` by returning all _but_ the last element.
`initial $myList` returns `[1 2 3 4]`.

`initial` panics if there is a problem while `mustInitial` returns an error to the
template engine if there is a problem.

## append, mustAppend

Append a new item to an existing list, creating a new list.

```
$new = append $myList 6
```

The above would set `$new` to `[1 2 3 4 5 6]`. `$myList` would remain unaltered.

`append` panics if there is a problem while `mustAppend` returns an error to the
template engine if there is a problem.

## prepend, mustPrepend

Push an element onto the front of a list, creating a new list.

```
prepend $myList 0
```

The above would produce `[0 1 2 3 4 5]`. `$myList` would remain unaltered.

`prepend` panics if there is a problem while `mustPrepend` returns an error to the
template engine if there is a problem.

## concat

Concatenate arbitrary number of lists into one.

```
concat $myList ( list 6 7 ) ( list 8 )
```

The above would produce `[1 2 3 4 5 6 7 8]`. `$myList` would remain unaltered.

## reverse, mustReverse

Produce a new list with the reversed elements of the given list.

```
reverse $myList
```

The above would generate the list `[5 4 3 2 1]`.

`reverse` panics if there is a problem while `mustReverse` returns an error to the
template engine if there is a problem.

## uniq, mustUniq

Generate a list with all of the duplicates removed.

```
list 1 1 1 2 | uniq
```

The above would produce `[1 2]`

`uniq` panics if there is a problem while `mustUniq` returns an error to the
template engine if there is a problem.

## without, mustWithout

The `without` function filters items out of a list.

```
without $myList 3
```

The above would produce `[1 2 4 5]`

Without can take more than one filter:

```
without $myList 1 3 5
```

That would produce `[2 4]`

`without` panics if there is a problem while `mustWithout` returns an error to the
template engine if there is a problem.

## has, mustHas

Test to see if a list has a particular element.

```
has 4 $myList
```

The above would return `true`, while `has "hello" $myList` would return false.

`has` panics if there is a problem while `mustHas` returns an error to the
template engine if there is a problem.

## compact, mustCompact

Accepts a list and removes entries with empty values.

```
$list := list 1 "a" "foo" ""
$copy := compact $list
```

`compact` will return a new list with the empty (i.e., "") item removed.

`compact` panics if there is a problem and `mustCompact` returns an error to the
template engine if there is a problem.

## slice, mustSlice

To get partial elements of a list, use `slice list [n] [m]`. It is
equivalent of `list[n:m]`.

- `slice $myList` returns `[1 2 3 4 5]`. It is same as `myList[:]`.
- `slice $myList 3` returns `[4 5]`. It is same as `myList[3:]`.
- `slice $myList 1 3` returns `[2 3]`. It is same as `myList[1:3]`.
- `slice $myList 0 3` returns `[1 2 3]`. It is same as `myList[:3]`.

`slice` panics if there is a problem while `mustSlice` returns an error to the
template engine if there is a problem.

## chunk

To split a list into chunks of given size, use `chunk size list`. This is useful for pagination.

```
chunk 3 (list 1 2 3 4 5 6 7 8)
```

This produces list of lists `[ [ 1 2 3 ] [ 4 5 6 ] [ 7 8 ] ]`.

## A Note on List Internals

A list is implemented in Go as a `[]interface{}`. For Go developers embedding
Sprig, you may pass `[]interface{}` items into your template context and be
able to use all of the `list` functions on those items.
# Dictionaries and Dict Functions

Sprig provides a key/value storage type called a `dict` (short for "dictionary",
as in Python). A `dict` is an _unorder_ type.

The key to a dictionary **must be a string**. However, the value can be any
type, even another `dict` or `list`.

Unlike `list`s, `dict`s are not immutable. The `set` and `unset` functions will
modify the contents of a dictionary.

## dict

Creating dictionaries is done by calling the `dict` function and passing it a
list of pairs.

The following creates a dictionary with three items:

```
$myDict := dict "name1" "value1" "name2" "value2" "name3" "value 3"
```

## get

Given a map and a key, get the value from the map.

```
get $myDict "name1"
```

The above returns `"value1"`

Note that if the key is not found, this operation will simply return `""`. No error
will be generated.

## set

Use `set` to add a new key/value pair to a dictionary.

```
$_ := set $myDict "name4" "value4"
```

Note that `set` _returns the dictionary_ (a requirement of Go template functions),
so you may need to trap the value as done above with the `$_` assignment.

## unset

Given a map and a key, delete the key from the map.

```
$_ := unset $myDict "name4"
```

As with `set`, this returns the dictionary.

Note that if the key is not found, this operation will simply return. No error
will be generated.

## hasKey

The `hasKey` function returns `true` if the given dict contains the given key.

```
hasKey $myDict "name1"
```

If the key is not found, this returns `false`.

## pluck

The `pluck` function makes it possible to give one key and multiple maps, and
get a list of all of the matches:

```
pluck "name1" $myDict $myOtherDict
```

The above will return a `list` containing every found value (`[value1 otherValue1]`).

If the give key is _not found_ in a map, that map will not have an item in the
list (and the length of the returned list will be less than the number of dicts
in the call to `pluck`.

If the key is _found_ but the value is an empty value, that value will be
inserted.

A common idiom in Sprig templates is to uses `pluck... | first` to get the first
matching key out of a collection of dictionaries.

## dig

The `dig` function traverses a nested set of dicts, selecting keys from a list
of values. It returns a default value if any of the keys are not found at the
associated dict.

```
dig "user" "role" "humanName" "guest" $dict
```

Given a dict structured like
```
{
  user: {
    role: {
      humanName: "curator"
    }
  }
}
```

the above would return `"curator"`. If the dict lacked even a `user` field,
the result would be `"guest"`.

Dig can be very useful in cases where you'd like to avoid guard clauses,
especially since Go's template package's `and` doesn't shortcut. For instance
`and a.maybeNil a.maybeNil.iNeedThis` will always evaluate
`a.maybeNil.iNeedThis`, and panic if `a` lacks a `maybeNil` field.)

`dig` accepts its dict argument last in order to support pipelining.

## keys

The `keys` function will return a `list` of all of the keys in one or more `dict`
types. Since a dictionary is _unordered_, the keys will not be in a predictable order.
They can be sorted with `sortAlpha`.

```
keys $myDict | sortAlpha
```

When supplying multiple dictionaries, the keys will be concatenated. Use the `uniq`
function along with `sortAlpha` to get a unqiue, sorted list of keys.

```
keys $myDict $myOtherDict | uniq | sortAlpha
```

## pick

The `pick` function selects just the given keys out of a dictionary, creating a
new `dict`.

```
$new := pick $myDict "name1" "name2"
```

The above returns `{name1: value1, name2: value2}`

## omit

The `omit` function is similar to `pick`, except it returns a new `dict` with all
the keys that _do not_ match the given keys.

```
$new := omit $myDict "name1" "name3"
```

The above returns `{name2: value2}`

## values

The `values` function is similar to `keys`, except it returns a new `list` with
all the values of the source `dict` (only one dictionary is supported).

```
$vals := values $myDict
```

The above returns `list["value1", "value2", "value 3"]`. Note that the `values`
function gives no guarantees about the result ordering- if you care about this,
then use `sortAlpha`.
# Type Conversion Functions

The following type conversion functions are provided by Sprig:

- `atoi`: Convert a string to an integer.
- `float64`: Convert to a `float64`.
- `int`: Convert to an `int` at the system's width.
- `int64`: Convert to an `int64`.
- `toDecimal`: Convert a unix octal to a `int64`.
- `toString`: Convert to a string.
- `toStrings`: Convert a list, slice, or array to a list of strings.

Only `atoi` requires that the input be a specific type. The others will attempt
to convert from any type to the destination type. For example, `int64` can convert
floats to ints, and it can also convert strings to ints.

## toStrings

Given a list-like collection, produce a slice of strings.

```
list 1 2 3 | toStrings
```

The above converts `1` to `"1"`, `2` to `"2"`, and so on, and then returns
them as a list.

## toDecimal

Given a unix octal permission, produce a decimal.

```
"0777" | toDecimal
```

The above converts `0777` to `511` and returns the value as an int64.
# Path and Filepath Functions

While Sprig does not grant access to the filesystem, it does provide functions
for working with strings that follow file path conventions.

## Paths

Paths separated by the slash character (`/`), processed by the `path` package.

Examples:

* The [Linux](https://en.wikipedia.org/wiki/Linux) and
  [MacOS](https://en.wikipedia.org/wiki/MacOS)
  [filesystems](https://en.wikipedia.org/wiki/File_system):
  `/home/user/file`, `/etc/config`;
* The path component of
  [URIs](https://en.wikipedia.org/wiki/Uniform_Resource_Identifier):
  `https://example.com/some/content/`, `ftp://example.com/file/`.

### base

Return the last element of a path.

```
base "foo/bar/baz"
```

The above prints "baz".

### dir

Return the directory, stripping the last part of the path. So `dir "foo/bar/baz"`
returns `foo/bar`.

### clean

Clean up a path.

```
clean "foo/bar/../baz"
```

The above resolves the `..` and returns `foo/baz`.

### ext

Return the file extension.

```
ext "foo.bar"
```

The above returns `.bar`.

### isAbs

To check whether a path is absolute, use `isAbs`.

## Filepaths

Paths separated by the `os.PathSeparator` variable, processed by the `path/filepath` package.

These are the recommended functions to use when parsing paths of local filesystems, usually when dealing with local files, directories, etc.

Examples:

* Running on Linux or MacOS the filesystem path is separated by the slash character (`/`):
  `/home/user/file`, `/etc/config`;
* Running on [Windows](https://en.wikipedia.org/wiki/Microsoft_Windows)
  the filesystem path is separated by the backslash character (`\`):
  `C:\Users\Username\`, `C:\Program Files\Application\`;

### osBase

Return the last element of a filepath.

```
osBase "/foo/bar/baz"
osBase "C:\\foo\\bar\\baz"
```

The above prints "baz" on Linux and Windows, respectively.

### osDir

Return the directory, stripping the last part of the path. So `osDir "/foo/bar/baz"`
returns `/foo/bar` on Linux, and `osDir "C:\\foo\\bar\\baz"`
returns `C:\\foo\\bar` on Windows.

### osClean

Clean up a path.

```
osClean "/foo/bar/../baz"
osClean "C:\\foo\\bar\\..\\baz"
```

The above resolves the `..` and returns `foo/baz` on Linux and `C:\\foo\\baz` on Windows.

### osExt

Return the file extension.

```
osExt "/foo.bar"
osExt "C:\\foo.bar"
```

The above returns `.bar` on Linux and Windows, respectively.

### osIsAbs

To check whether a file path is absolute, use `osIsAbs`.
# Flow Control Functions

## fail

Unconditionally returns an empty `string` and an `error` with the specified
text. This is useful in scenarios where other conditionals have determined that
template rendering should fail.

```
fail "Please accept the end user license agreement"
```
# UUID Functions

Sprig can generate UUID v4 universally unique IDs.

```
uuidv4
```

The above returns a new UUID of the v4 (randomly generated) type.
# Reflection Functions

Sprig provides rudimentary reflection tools. These help advanced template
developers understand the underlying Go type information for a particular value.

Go has several primitive _kinds_, like `string`, `slice`, `int64`, and `bool`.

Go has an open _type_ system that allows developers to create their own types.

Sprig provides a set of functions for each.

## Kind Functions

There are two Kind functions: `kindOf` returns the kind of an object.

```
kindOf "hello"
```

The above would return `string`. For simple tests (like in `if` blocks), the
`kindIs` function will let you verify that a value is a particular kind:

```
kindIs "int" 123
```

The above will return `true`

## Type Functions

Types are slightly harder to work with, so there are three different functions:

- `typeOf` returns the underlying type of a value: `typeOf $foo`
- `typeIs` is like `kindIs`, but for types: `typeIs "*io.Buffer" $myVal`
- `typeIsLike` works as `typeIs`, except that it also dereferences pointers.

**Note:** None of these can test whether or not something implements a given
interface, since doing so would require compiling the interface in ahead of time.

## deepEqual

`deepEqual` returns true if two values are ["deeply equal"](https://golang.org/pkg/reflect/#DeepEqual)

Works for non-primitive types as well (compared to the built-in `eq`).

```
deepEqual (list 1 2 3) (list 1 2 3)
```

The above will return `true`
# Cryptographic and Security Functions

Sprig provides a couple of advanced cryptographic functions.

## sha1sum

The `sha1sum` function receives a string, and computes it's SHA1 digest.

```
sha1sum "Hello world!"
```

## sha256sum

The `sha256sum` function receives a string, and computes it's SHA256 digest.

```
sha256sum "Hello world!"
```

The above will compute the SHA 256 sum in an "ASCII armored" format that is
safe to print.

## sha512sum

The `sha512sum` function receives a string, and computes it's SHA512 digest.

```
sha512sum "Hello world!"
```

The above will compute the SHA 512 sum in an "ASCII armored" format that is
safe to print.

## adler32sum

The `adler32sum` function receives a string, and computes its Adler-32 checksum.

```
adler32sum "Hello world!"
```
# URL Functions

## urlParse
Parses string for URL and produces dict with URL parts

```
urlParse "http://admin:secret@server.com:8080/api?list=false#anchor"
```

The above returns a dict, containing URL object:
```yaml
scheme:   'http'
host:     'server.com:8080'
path:     '/api'
query:    'list=false'
opaque:   nil
fragment: 'anchor'
userinfo: 'admin:secret'
```

For more info, check https://golang.org/pkg/net/url/#URL

## urlJoin
Joins map (produced by `urlParse`) to produce URL string

```
urlJoin (dict "fragment" "fragment" "host" "host:80" "path" "/path" "query" "query" "scheme" "http")
```

The above returns the following string:
```
proto://host:80/path?query#fragment
```
