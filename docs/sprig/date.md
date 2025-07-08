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
