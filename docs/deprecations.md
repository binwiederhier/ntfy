# Deprecations and breaking changes
This page is used to list deprecation notices for ntfy. Deprecated commands and options will be 
**removed after 1-3 months** from the time they were deprecated. How long the feature is deprecated
before the behavior is changed depends on the severity of the change, and how prominent the feature is.

## Active deprecations

### ntfy CLI: `ntfy user/access/token/tier` without a running server
> Active since 2026-10-22 (v2.33.0), behavior will change after 2026-11-22

The `ntfy user`, `ntfy access`, `ntfy token` and `ntfy tier` commands now talk to the running ntfy server through
its local [admin socket](config.md#admin-socket), so changes take effect immediately. If no server answers on the
socket, they still edit the database directly, like they used to, and print a deprecation warning. This fallback
will be removed: after that, these commands need a running server, either through the admin socket or with
`--server` and `--token`.

If you see the warning, make sure the server can create its admin socket (see [admin socket](config.md#admin-socket)
for what to change depending on how you installed ntfy). To set up users before the server ever ran, use
[`auth-users`](config.md#users-via-the-config) in the config file instead.

=== "Before"
    ```
    $ systemctl stop ntfy
    $ ntfy user add phil
    **********************************************************************
    DEPRECATION WARNING: No ntfy server answered on the admin socket
    /run/ntfy/admin.sock (no such file or directory), writing to the database
    directly. A running server may not see this change right away.
    ...
    user phil added with role user
    ```

=== "After"
    ```
    $ systemctl stop ntfy
    $ ntfy user add phil
    ntfy error: no ntfy server answered on the admin socket /run/ntfy/admin.sock
    ```

## Previous deprecations

### ntfy CLI: `ntfy publish --env-topic` will be removed
> Active since 2022-06-20, behavior changed with v1.30.1

The `ntfy publish --env-topic` option will be removed. It'll still be possible to specify a topic via the
`NTFY_TOPIC` environment variable, but it won't be necessary anymore to specify the `--env-topic` flag.

=== "Before"
    ```
    $ NTFY_TOPIC=mytopic ntfy publish --env-topic "this is the message"
    ```

=== "After"
    ```
    $ NTFY_TOPIC=mytopic ntfy publish "this is the message"
    ```

### <del>Android app: WebSockets will become the default connection protocol</del>
> Active since 2022-03-13, behavior will not change (deprecation removed 2022-06-20)

Instant delivery connections and connections to self-hosted servers in the Android app were going to switch
to use the WebSockets protocol by default. It was decided to keep JSON stream as the most compatible default
and add a notice banner in the Android app instead.

### Android app: Using `since=<timestamp>` instead of `since=<id>`
> Active since 2022-02-27, behavior changed with v1.14.0

The Android app started using `since=<id>` instead of `since=<timestamp>`, which means as of Android app v1.14.0, 
it will not work with servers older than v1.16.0 anymore. This is to simplify handling of deduplication in the Android app.

The `since=<timestamp>` endpoint will continue to work. This is merely a notice that the Android app behavior will change.

### Running server via `ntfy` (instead of `ntfy serve`)
> Deprecated 2021-12-17, behavior changed with v1.10.0

As more commands are added to the `ntfy` CLI tool, using just `ntfy` to run the server is not practical
anymore. Please use `ntfy serve` instead. This also applies to Docker images, as they can also execute more than
just the server.

=== "Before"
    ```
    $ ntfy
    2021/12/17 08:16:01 Listening on :80/http
    ```

=== "After"
    ```
    $ ntfy serve
    2021/12/17 08:16:01 Listening on :80/http
    ```

