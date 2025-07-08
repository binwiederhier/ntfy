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
