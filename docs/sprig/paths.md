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
