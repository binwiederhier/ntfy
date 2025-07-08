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
