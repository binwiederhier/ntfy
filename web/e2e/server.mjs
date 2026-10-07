import { execFileSync, spawn } from "node:child_process";
import fs from "node:fs";
import path from "node:path";

// Starts a throwaway ntfy server for the e2e run; Playwright's webServer launches this
// script (with the paths from env.js) and kills it when the run is over. State is wiped
// on every start.
const [binary, configFile, port, tier] = process.argv.slice(2);
const runDir = path.dirname(configFile);
const baseURL = `http://127.0.0.1:${port}`;

if (!fs.existsSync(binary)) {
  console.error(`${binary} not found; run "make web-e2e" to build it`);
  process.exit(1);
}
fs.rmSync(runDir, { recursive: true, force: true });
fs.mkdirSync(runDir, { recursive: true });

// behind-proxy lets every test pose as its own visitor via X-Forwarded-For (see fixtures.js),
// so per-IP limits like the 6/day account creation limit don't leak between tests
const config = {
  "base-url": baseURL,
  "listen-http": `127.0.0.1:${port}`,
  "cache-file": path.join(runDir, "cache.db"),
  "attachment-cache-dir": path.join(runDir, "attachments"),
  "auth-file": path.join(runDir, "user.db"),
  "auth-default-access": "read-write",
  "enable-signup": true,
  "enable-login": true,
  "enable-reservations": true,
  "behind-proxy": true,
  "log-file": path.join(runDir, "ntfy.log"),
};
fs.writeFileSync(
  configFile,
  Object.entries(config)
    .map(([k, v]) => `${k}: ${v}\n`)
    .join(""),
);

const server = spawn(binary, ["serve", "--config", configFile], { stdio: "inherit" });
server.on("exit", (code) => process.exit(code ?? 1));
for (const signal of ["SIGINT", "SIGTERM"]) {
  process.on(signal, () => server.kill(signal));
}

// Wait for the server to create its database; the CLI refuses to run before that
while (!(await fetch(`${baseURL}/v1/health`).catch(() => null))?.ok) {
  await new Promise((resolve) => setTimeout(resolve, 100));
}

// Provision what a remote server is expected to already have: an admin token (specs create
// their users through the admin API) and a tier with reservations
const ntfy = (args, env = {}) => execFileSync(binary, args, { env: { ...process.env, ...env, NTFY_CONFIG_FILE: configFile } }).toString();
ntfy(["user", "add", "--role=admin", "e2eadmin"], { NTFY_PASSWORD: "e2eadmin-password" });
const token = ntfy(["token", "add", "e2eadmin"]).match(/token (tk_\w+) created/)[1];
ntfy(["tier", "add", "--name", "E2E", "--reservation-limit", "10", "--message-limit", "10000", tier]);

// Playwright waits for this line and exports the token as NTFY_E2E_ADMIN_TOKEN (see config)
console.log(`ntfy e2e server ready, admin token ${token}`);
