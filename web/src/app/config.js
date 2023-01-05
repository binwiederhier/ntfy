const config = window.config;

if (config.base_url === "") {
    config.base_url = window.location.origin;
}

export default config;
