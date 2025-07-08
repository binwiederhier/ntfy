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
