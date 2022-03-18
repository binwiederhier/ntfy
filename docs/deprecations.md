# Deprecation notices
This page is used to list deprecation notices for ntfy. Deprecated commands and options will be 
**removed after ~3 months** from the time they were deprecated.

## Active deprecations

### Android app: WebSockets will become the default connection protocol  
> Active since 2022-03-13, behavior will change in **June 2022**

In future versions of the Android app, instant delivery connections and connections to self-hosted servers will
be using the WebSockets protocol. This potentially requires [configuration changes in your proxy](https://ntfy.sh/docs/config/#nginxapache2caddy).

### Android app: Using `since=<timestamp>` instead of `since=<id>`
> Active since 2022-02-27, behavior will change in **May 2022**

In about 3 months, the Android app will start using `since=<id>` instead of `since=<timestamp>`, which means that it will
not work with servers older than v1.16.0 anymore. This is to simplify handling of deduplication in the Android app.

The `since=<timestamp>` endpoint will continue to work. This is merely a notice that the Android app behavior will change.

## Previous deprecations

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

