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
