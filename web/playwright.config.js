import { defineConfig, devices } from "@playwright/test";
import { baseURL, binary, configFile, localTier, port, remote } from "./e2e/env.js";

// End-to-end tests drive a real browser against a real ntfy server, started from
// e2e/server.mjs with a fresh database, or an existing one (see e2e/env.js). Run with
// "make web-e2e".
export default defineConfig({
  testDir: "./e2e",
  fullyParallel: true,
  timeout: 30_000,
  expect: { timeout: 10_000 },
  reporter: [["list"]],
  use: {
    ...devices["Desktop Chrome"],
    baseURL,
    permissions: ["notifications"],
    trace: "retain-on-failure",
  },
  webServer: remote
    ? undefined
    : {
        command: `node e2e/server.mjs ${binary} ${configFile} ${port} ${localTier}`,
        wait: { stdout: /admin token (?<ntfy_e2e_admin_token>tk_\w+)/ },
        reuseExistingServer: false,
        stdout: "ignore",
      },
});
