const { config } = window;

// The backend returns an empty base_url for the config struct,
// so the frontend (hey, that's us!) can use the current location.
if (!config.base_url || config.base_url === "") {
  config.base_url = window.location.origin;
}

export default config;
