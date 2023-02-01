// THIS FILE IS JUST AN EXAMPLE
//
// It is removed during the build process. The actual config is dynamically
// generated server-side and served by the ntfy server.
//
// During web development, you may change values here for rapid testing.

var config = {
    base_url: "https://127.0.0.1", // window.location.origin FIXME update before merging
    app_root: "/app",
    enable_login: true,
    enable_signup: true,
    enable_payments: true,
    enable_reservations: true,
    disallowed_topics: ["docs", "static", "file", "app", "account", "settings", "pricing", "signup", "login", "reset-password"]
};
