# Deprecation notices
This page is used to list deprecation notices for ntfy. Deprecated commands and options will be 
**removed after ~3 months** from the time they were deprecated.

## Active deprecations

### Running server via `ntfy` (instead of `ntfy serve`)
> since 2021-12-17

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

