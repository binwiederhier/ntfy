# Flow Control Functions

## fail

Unconditionally returns an empty `string` and an `error` with the specified
text. This is useful in scenarios where other conditionals have determined that
template rendering should fail.

```
fail "Please accept the end user license agreement"
```
