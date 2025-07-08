# Template Functions

ntfy includes a (reduced) version of [Sprig](https://github.com/Masterminds/sprig) to add functions that can be used
when you are using the [message template](publish.md#message-templating) feature.

Below are the functions that are available to use inside your message/title templates.

* [String Functions](./sprig/strings.md): `trim`, `trunc`, `substr`, `plural`, etc.
    * [String List Functions](./sprig/string_slice.md): `splitList`, `sortAlpha`, etc.
* [Integer Math Functions](./sprig/math.md): `add`, `max`, `mul`, etc.
    * [Integer List Functions](./sprig/integer_slice.md): `until`, `untilStep`
* [Date Functions](./sprig/date.md): `now`, `date`, etc.
* [Defaults Functions](./sprig/defaults.md): `default`, `empty`, `coalesce`, `fromJSON`, `toJSON`, `toPrettyJSON`, `toRawJSON`, `ternary`
* [Encoding Functions](./sprig/encoding.md): `b64enc`, `b64dec`, etc.
* [Lists and List Functions](./sprig/lists.md): `list`, `first`, `uniq`, etc.
* [Dictionaries and Dict Functions](./sprig/dicts.md): `get`, `set`, `dict`, `hasKey`, `pluck`, `dig`, etc.
* [Type Conversion Functions](./sprig/conversion.md): `atoi`, `int64`, `toString`, etc.
* [Path and Filepath Functions](./sprig/paths.md): `base`, `dir`, `ext`, `clean`, `isAbs`, `osBase`, `osDir`, `osExt`, `osClean`, `osIsAbs`
* [Flow Control Functions](./sprig/flow_control.md): `fail`
* Advanced Functions
    * [UUID Functions](./sprig/uuid.md): `uuidv4`
    * [Reflection](./sprig/reflection.md): `typeOf`, `kindIs`, `typeIsLike`, etc.
    * [Cryptographic and Security Functions](./sprig/crypto.md): `sha256sum`, etc.
    * [URL](./sprig/url.md): `urlParse`, `urlJoin`
