// THIS FILE IS JUST AN EXAMPLE
//
// It is removed during the build process. The actual config is dynamically
// generated server-side and served by the ntfy server.
//
// During web development, you may change values here for rapid testing.

var config = {
    baseUrl: "http://localhost:2586", // window.location.origin FIXME update before merging
    appRoot: "/app",
    enableLogin: true,
    enableSignup: true,
    enableResetPassword: false,
    disallowedTopics: ["docs", "static", "file", "app", "account", "settings", "pricing", "signup", "login", "reset-password"]
};
