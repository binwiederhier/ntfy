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
