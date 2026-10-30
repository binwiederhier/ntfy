import path from "node:path";

// Shared by the config and the specs, so both agree on which server to test.
//
// By default, a throwaway local server is started from build/ntfy-e2e (see server.mjs and
// "make web-e2e"). To test an existing server instead, set NTFY_E2E_BASE_URL, plus
// NTFY_E2E_ADMIN_TOKEN (to create and delete test users) and NTFY_E2E_TIER (code of an
// existing tier with reservations).
const root = path.resolve(__dirname, "../..");

export const remote = !!process.env.NTFY_E2E_BASE_URL;
export const port = 12586;
export const baseURL = process.env.NTFY_E2E_BASE_URL || `http://127.0.0.1:${port}`;
export const binary = path.join(root, "build", "ntfy-e2e");
export const configFile = path.join(root, "build", "e2e", "server.yml");
export const localTier = "e2e";
export const password = "secret12345";

// Read lazily: for the local server, Playwright sets this once server.mjs reports it
export const adminToken = () => process.env.NTFY_E2E_ADMIN_TOKEN;
export const tier = () => (remote ? process.env.NTFY_E2E_TIER : localTier);

// Random suffix so specs never share topics or users, even across reruns.
export const unique = (prefix) => `${prefix}${Math.random().toString(36).slice(2, 10)}`;
