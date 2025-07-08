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
