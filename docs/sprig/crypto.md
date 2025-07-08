# Cryptographic and Security Functions

Sprig provides a couple of advanced cryptographic functions.

## sha1sum

The `sha1sum` function receives a string, and computes it's SHA1 digest.

```
sha1sum "Hello world!"
```

## sha256sum

The `sha256sum` function receives a string, and computes it's SHA256 digest.

```
sha256sum "Hello world!"
```

The above will compute the SHA 256 sum in an "ASCII armored" format that is
safe to print.

## sha512sum

The `sha512sum` function receives a string, and computes it's SHA512 digest.

```
sha512sum "Hello world!"
```

The above will compute the SHA 512 sum in an "ASCII armored" format that is
safe to print.

## adler32sum

The `adler32sum` function receives a string, and computes its Adler-32 checksum.

```
adler32sum "Hello world!"
```
