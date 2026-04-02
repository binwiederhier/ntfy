// Config Generator for ntfy
//
// Warning, AI code
// ----------------
// This code is entirely AI generated, but this very comment is not. Phil wrote this. Hi!
// I felt like the Config Generator was a great feature to have, but it would have taken me forever
// to write this code without AI. I reviewed the code manually, and it doesn't do anything dangerous.
// It's not the greatest code, but it works well enough to deliver value, and that's what it's all about.
//
// End of human comment. ;)
//
// How it works
// ------------
// The generator is a modal with a left panel (form inputs) and a right panel (live output).
// On every input change, the update cycle runs: updateVisibility() syncs the UI state, then
// updateOutput() collects values from the form and renders them as server.yml, docker-compose.yml,
// or env vars.
//
// The CONFIG array is the source of truth for which config keys exist, their env var names,
// which section they belong to, and optional defaults. collectValues() walks CONFIG, reads each
// matching DOM element, skips anything in a hidden panel or section, and returns a plain
// {key: value} object. The three generators (generateServerYml, generateDockerCompose,
// generateEnvVars) each iterate CONFIG in order and format the collected values. Provisioned
// users, ACLs, and tokens are collected separately from repeatable rows and stored as arrays
// under "_auth-users", "_auth-acls", "_auth-tokens". The formatAuthUsers/Acls/Tokens() helpers
// turn those arrays into "user:pass:role" strings shared by all three generators.
//
// Visibility is managed by updateVisibility(), which delegates to five helpers:
// syncRadiosToHiddenInputs() copies user-facing radios and selects to hidden inputs that CONFIG
// knows about (e.g. login mode radio → enable-login + require-login checkboxes).
// updateFeatureVisibility() shows/hides nav tabs, configure buttons, and email sections based
// on which feature checkboxes are checked. updatePostgresFields() swaps file-path inputs for
// "Using PostgreSQL" labels when PostgreSQL is selected. prefillDefaults() sets sensible values
// (file paths, addresses) when a feature is first enabled, tracked via a data-cleared attribute
// so user edits are respected. autoDetectServerType() flips the server-type radio to "custom"
// if the user's access/login settings no longer match "open" or "private".
//
// Event listeners are grouped into setup functions (setupModalEvents, setupAuthEvents,
// setupServerTypeEvents, setupUnifiedPushEvents, setupFormListeners, setupWebPushEvents)
// called from initGenerator().
// A general listener on all inputs calls the update cycle. Specific listeners handle cleanup
// logic, e.g. unchecking auth resets all auth-related fields and provisioned rows.
//
// Frequently-used DOM elements are queried once in cacheElements() and passed around as an
// `els` object, avoiding repeated querySelector calls.
//
// Field inter-dependencies
// ------------------------
// Several UI fields don't map 1:1 to config keys. Instead, user-friendly controls drive
// hidden inputs that CONFIG knows about. The sync happens in syncRadiosToHiddenInputs(),
// called on every change via updateVisibility().
//
//   Server type (Open / Private / Custom)
//     "Open"    → unchecks auth, sets default-access to read-write, login to disabled
//     "Private" → checks auth, sets default-access to deny-all, login to required
//     "Custom"  → no automatic changes; also auto-selected when the user manually
//                  changes access/login to values that don't match Open or Private
//
//   Auth checkbox (#cg-feat-auth)
//     When unchecked → resets: default-access to read-write, login to disabled,
//       signup to no, UnifiedPush to no, removes all provisioned users/ACLs/tokens,
//       clears auth-file, switches server type back to Open.
//       Also explicitly unchecks hidden enable-login, require-login, enable-signup.
//     When checked by PostgreSQL auto-enable → no reset, just enables the tab.
//
//   Login mode (Disabled / Enabled / Required) — three-way radio
//     Maps to two hidden checkboxes:
//       enable-login  = checked when Enabled OR Required
//       require-login = checked when Required only
//
//   Signup (Yes / No) — radio pair
//     Maps to hidden enable-signup checkbox.
//
//   Proxy (Yes / No) — radio pair
//     Maps to hidden behind-proxy checkbox.
//
//   iOS support (Yes / No) — radio pair
//     Sets upstream-base-url to "https://ntfy.sh" when Yes, clears when No.
//
//   UnifiedPush (Yes / No) — radio pair
//     When Yes, enables auth (if not already on) and adds a disabled "*:up*:write-only"
//     ACL row to the Users tab. The row's fields are grayed out and non-editable. It is
//     collected like any other ACL row. Clicking its [x] removes the row and toggles
//     UnifiedPush back to No.
//
//   Database type (SQLite / PostgreSQL)
//     When PostgreSQL is selected:
//       - Auto-enables auth if not already on
//       - Hides file-path fields (auth-file, cache-file, web-push-file) and shows
//         "Using PostgreSQL" labels instead
//       - Shows the Database nav tab for the database-url field
//       - Prefills database-url with a postgres:// template
//     The database question itself only appears when a DB-dependent feature
//     (auth, cache, or web push) is enabled.
//
//   Feature checkboxes (auth, cache, attachments, web push, email out, email in)
//     Each shows/hides its nav tab and "Configure" button.
//     When first enabled, prefillDefaults() fills in sensible paths/values.
//     The prefill is skipped if the user has already typed (or cleared) the field
//     (tracked via data-cleared attribute).
//
(function() {
  "use strict";

  const CONFIG = [
    { key: "base-url", env: "NTFY_BASE_URL", section: "basic" },
    { key: "behind-proxy", env: "NTFY_BEHIND_PROXY", section: "basic", type: "bool" },
    { key: "database-url", env: "NTFY_DATABASE_URL", section: "database" },
    { key: "auth-file", env: "NTFY_AUTH_FILE", section: "auth" },
    { key: "auth-default-access", env: "NTFY_AUTH_DEFAULT_ACCESS", section: "auth", def: "read-write" },
    { key: "enable-login", env: "NTFY_ENABLE_LOGIN", section: "auth", type: "bool" },
    { key: "require-login", env: "NTFY_REQUIRE_LOGIN", section: "auth", type: "bool" },
    { key: "enable-signup", env: "NTFY_ENABLE_SIGNUP", section: "auth", type: "bool" },
    { key: "attachment-cache-dir", env: "NTFY_ATTACHMENT_CACHE_DIR", section: "attach" },
    { key: "attachment-file-size-limit", env: "NTFY_ATTACHMENT_FILE_SIZE_LIMIT", section: "attach", def: "15M" },
    { key: "attachment-total-size-limit", env: "NTFY_ATTACHMENT_TOTAL_SIZE_LIMIT", section: "attach", def: "5G" },
    { key: "attachment-expiry-duration", env: "NTFY_ATTACHMENT_EXPIRY_DURATION", section: "attach", def: "3h" },
    { key: "cache-file", env: "NTFY_CACHE_FILE", section: "cache" },
    { key: "cache-duration", env: "NTFY_CACHE_DURATION", section: "cache", def: "12h" },
    { key: "web-push-public-key", env: "NTFY_WEB_PUSH_PUBLIC_KEY", section: "webpush" },
    { key: "web-push-private-key", env: "NTFY_WEB_PUSH_PRIVATE_KEY", section: "webpush" },
    { key: "web-push-file", env: "NTFY_WEB_PUSH_FILE", section: "webpush" },
    { key: "web-push-email-address", env: "NTFY_WEB_PUSH_EMAIL_ADDRESS", section: "webpush" },
    { key: "smtp-sender-addr", env: "NTFY_SMTP_SENDER_ADDR", section: "smtp-out" },
    { key: "smtp-sender-from", env: "NTFY_SMTP_SENDER_FROM", section: "smtp-out" },
    { key: "smtp-sender-user", env: "NTFY_SMTP_SENDER_USER", section: "smtp-out" },
    { key: "smtp-sender-pass", env: "NTFY_SMTP_SENDER_PASS", section: "smtp-out" },
    { key: "smtp-sender-verify", env: "NTFY_SMTP_SENDER_VERIFY", section: "smtp-out", type: "bool" },
    { key: "smtp-server-listen", env: "NTFY_SMTP_SERVER_LISTEN", section: "smtp-in" },
    { key: "smtp-server-domain", env: "NTFY_SMTP_SERVER_DOMAIN", section: "smtp-in" },
    { key: "smtp-server-addr-prefix", env: "NTFY_SMTP_SERVER_ADDR_PREFIX", section: "smtp-in" },
    { key: "upstream-base-url", env: "NTFY_UPSTREAM_BASE_URL", section: "upstream" }
  ];

  // Feature checkbox → nav tab ID
  const NAV_MAP = {
    "cg-feat-auth": "cg-nav-auth",
    "cg-feat-cache": "cg-nav-cache",
    "cg-feat-attach": "cg-nav-attach",
    "cg-feat-webpush": "cg-nav-webpush"
  };

  const SECTION_COMMENTS = {
    basic: "# Server",
    database: "# Database",
    auth: "# Access control",
    attach: "# Attachments",
    cache: "# Message cache",
    webpush: "# Web push",
    "smtp-out": "# Email notifications (outgoing)",
    "smtp-in": "# Email publishing (incoming)",
    upstream: "# Upstream"
  };

  const durationRegex = /^(\d+)\s*(d|days?|h|hours?|m|mins?|minutes?|s|secs?|seconds?)$/i;
  const sizeRegex = /^(\d+)([tgmkb])?$/i;

  // --- DOM cache ---

  function cacheElements(modal) {
    return {
      modal,
      authCheckbox: modal.querySelector("#cg-feat-auth"),
      cacheCheckbox: modal.querySelector("#cg-feat-cache"),
      attachCheckbox: modal.querySelector("#cg-feat-attach"),
      webpushCheckbox: modal.querySelector("#cg-feat-webpush"),
      smtpOutCheckbox: modal.querySelector("#cg-feat-smtp-out"),
      smtpInCheckbox: modal.querySelector("#cg-feat-smtp-in"),
      accessSelect: modal.querySelector("#cg-default-access-select"),
      accessHidden: modal.querySelector("input[type=\"hidden\"][data-key=\"auth-default-access\"]"),
      loginHidden: modal.querySelector("#cg-enable-login-hidden"),
      requireLoginHidden: modal.querySelector("#cg-require-login-hidden"),
      signupHidden: modal.querySelector("#cg-enable-signup-hidden"),
      proxyCheckbox: modal.querySelector("#cg-behind-proxy"),
      smtpSenderVerifyHidden: modal.querySelector("#cg-smtp-sender-verify-hidden"),
      dbStep: modal.querySelector("#cg-wizard-db"),
      navDb: modal.querySelector("#cg-nav-database"),
      navEmail: modal.querySelector("#cg-nav-email"),
      emailOutSection: modal.querySelector("#cg-email-out-section"),
      emailInSection: modal.querySelector("#cg-email-in-section"),
      codeEl: modal.querySelector("#cg-code"),
      warningsEl: modal.querySelector("#cg-warnings")
    };
  }

  // --- Collect values ---

  function collectValues(els) {
    const { modal } = els;
    const values = {};

    CONFIG.forEach((c) => {
      const el = modal.querySelector(`[data-key="${c.key}"]`);
      if (!el) return;

      // Skip fields in hidden panels (feature not enabled)
      const panel = el.closest(".cg-panel");
      if (panel) {
        // Panel hidden directly
        if (panel.style.display === "none" || panel.classList.contains("cg-hidden")) return;
        // Panel with a nav tab that is hidden (feature not enabled)
        if (!panel.classList.contains("active")) {
          const panelId = panel.id;
          const navTab = modal.querySelector(`[data-panel="${panelId}"]`);
          if (!navTab || navTab.classList.contains("cg-hidden")) return;
        }
      }

      // Skip file inputs replaced by PostgreSQL
      if (el.dataset.pgDisabled) return;

      // Skip hidden individual fields or sections
      let ancestor = el.parentElement;
      while (ancestor && ancestor !== modal) {
        if (ancestor.style.display === "none" || ancestor.classList.contains("cg-hidden")) return;
        ancestor = ancestor.parentElement;
      }

      let val;
      if (c.type === "bool") {
        if (el.checked) val = "true";
      } else {
        val = el.value.trim();
        if (!val) return;
      }
      if (val && c.def && val === c.def) return;
      if (val) values[c.key] = val;
    });

    // Provisioned users
    const users = collectRepeatableRows(modal, ".cg-auth-user-row", (row) => {
      const u = row.querySelector("[data-field=\"username\"]");
      const p = row.querySelector("[data-field=\"password\"]");
      const r = row.querySelector("[data-field=\"role\"]");
      if (u && p && u.value.trim() && p.value.trim()) {
        return { username: u.value.trim(), password: p.value.trim(), role: r ? r.value : "user" };
      }
      return null;
    });
    if (users.length) values["_auth-users"] = users;

    // Provisioned ACLs
    const acls = collectRepeatableRows(modal, ".cg-auth-acl-row", (row) => {
      const u = row.querySelector("[data-field=\"username\"]");
      const t = row.querySelector("[data-field=\"topic\"]");
      const p = row.querySelector("[data-field=\"permission\"]");
      if (u && t && t.value.trim()) {
        return { user: u.value.trim(), topic: t.value.trim(), permission: p ? p.value : "read-write" };
      }
      return null;
    });
    if (acls.length) values["_auth-acls"] = acls;

    // Provisioned tokens
    const tokens = collectRepeatableRows(modal, ".cg-auth-token-row", (row) => {
      const u = row.querySelector("[data-field=\"username\"]");
      const t = row.querySelector("[data-field=\"token\"]");
      const l = row.querySelector("[data-field=\"label\"]");
      if (u && t && u.value.trim() && t.value.trim()) {
        return { user: u.value.trim(), token: t.value.trim(), label: l ? l.value.trim() : "" };
      }
      return null;
    });
    if (tokens.length) values["_auth-tokens"] = tokens;

    return values;
  }

  function collectRepeatableRows(modal, selector, extractor) {
    const results = [];
    modal.querySelectorAll(selector).forEach((row) => {
      const item = extractor(row);
      if (item) results.push(item);
    });
    return results;
  }

  // --- Shared auth formatting ---

  const bcryptCache = {};

  function hashPassword(username, password) {
    if (password.startsWith("$2")) return password; // already a bcrypt hash
    const cacheKey = username + "\0" + password;
    if (bcryptCache[cacheKey]) return bcryptCache[cacheKey];
    const hash = (typeof bcrypt !== "undefined") ? bcrypt.hashSync(password, 10) : password;
    bcryptCache[cacheKey] = hash;
    return hash;
  }

  function formatAuthUsers(values) {
    if (!values["_auth-users"]) return null;
    return values["_auth-users"].map((u) => `${u.username}:${hashPassword(u.username, u.password)}:${u.role}`);
  }

  function formatAuthAcls(values) {
    if (!values["_auth-acls"]) return null;
    return values["_auth-acls"].map((a) => `${a.user || "*"}:${a.topic}:${a.permission}`);
  }

  function formatAuthTokens(values) {
    if (!values["_auth-tokens"]) return null;
    return values["_auth-tokens"].map((t) => t.label ? `${t.user}:${t.token}:${t.label}` : `${t.user}:${t.token}`);
  }

  // --- Output generators ---

  function generateServerYml(values) {
    const lines = [];
    let lastSection = "";
    let hadAuth = false;

    CONFIG.forEach((c) => {
      if (!(c.key in values)) return;
      if (c.section !== lastSection) {
        if (lines.length) lines.push("");
        if (SECTION_COMMENTS[c.section]) lines.push(SECTION_COMMENTS[c.section]);
        lastSection = c.section;
      }
      if (c.section === "auth") hadAuth = true;
      const val = values[c.key];
      lines.push(c.type === "bool" ? `${c.key}: true` : `${c.key}: "${escapeYamlValue(val)}"`);
    });

    // Find insertion point for auth-users/auth-access/auth-tokens:
    // right after the last "auth-" prefixed line, before enable-*/require-* lines
    let authInsertIdx = lines.length;
    if (hadAuth) {
      for (let i = 0; i < lines.length; i++) {
        if (lines[i] === "# Access control") {
          // Find the last auth-* prefixed key in this section
          let lastAuthKey = i;
          for (let j = i + 1; j < lines.length; j++) {
            if (lines[j].startsWith("# ")) break;
            if (lines[j].startsWith("auth-")) lastAuthKey = j;
          }
          authInsertIdx = lastAuthKey + 1;
          break;
        }
      }
    }

    const authExtra = [];
    const users = formatAuthUsers(values);
    if (users) {
      if (!hadAuth) {
        authExtra.push("");
        authExtra.push("# Access control");
        hadAuth = true;
      }
      authExtra.push("auth-users:");
      users.forEach((entry) => authExtra.push(`  - "${escapeYamlValue(entry)}"`));
    }

    const acls = formatAuthAcls(values);
    if (acls) {
      if (!hadAuth) {
        authExtra.push("");
        authExtra.push("# Access control");
        hadAuth = true;
      }
      authExtra.push("auth-access:");
      acls.forEach((entry) => authExtra.push(`  - "${escapeYamlValue(entry)}"`));
    }

    const tokens = formatAuthTokens(values);
    if (tokens) {
      if (!hadAuth) {
        authExtra.push("");
        authExtra.push("# Access control");
        hadAuth = true;
      }
      authExtra.push("auth-tokens:");
      tokens.forEach((entry) => authExtra.push(`  - "${escapeYamlValue(entry)}"`));
    }

    // Splice auth extras into the right position
    if (authExtra.length) {
      lines.splice(authInsertIdx, 0, ...authExtra);
    }

    return lines.join("\n");
  }

  function generateDockerCompose(values) {
    const lines = [
      "services:",
      "  ntfy:",
      "    image: binwiederhier/ntfy",
      "    command: serve",
      "    environment:"
    ];

    let hasDollarNote = false;
    CONFIG.forEach((c) => {
      if (!(c.key in values)) return;
      let val = c.type === "bool" ? "true" : values[c.key];
      if (val.includes("$")) {
        val = val.replace(/\$/g, "$$$$");
        hasDollarNote = true;
      }
      lines.push(`      ${c.env}: "${escapeYamlValue(val)}"`);
    });

    const users = formatAuthUsers(values);
    if (users) {
      let usersVal = users.join(",");
      usersVal = usersVal.replace(/\$/g, "$$$$");
      hasDollarNote = true;
      lines.push(`      NTFY_AUTH_USERS: "${escapeYamlValue(usersVal)}"`);
    }

    const acls = formatAuthAcls(values);
    if (acls) {
      lines.push(`      NTFY_AUTH_ACCESS: "${escapeYamlValue(acls.join(","))}"`);
    }

    const tokens = formatAuthTokens(values);
    if (tokens) {
      lines.push(`      NTFY_AUTH_TOKENS: "${escapeYamlValue(tokens.join(","))}"`);
    }

    if (hasDollarNote) {
      // Insert note after "environment:" line
      const envIdx = lines.indexOf("    environment:");
      if (envIdx !== -1) {
        lines.splice(envIdx + 1, 0, "      # Note: $ is doubled to $$ for docker-compose");
      }
    }

    // Derive volumes from configured file/directory paths
    const dirs = new Set();
    ["auth-file", "cache-file", "web-push-file"].forEach((key) => {
      if (values[key]) {
        const dir = values[key].substring(0, values[key].lastIndexOf("/"));
        if (dir) dirs.add(dir);
      }
    });
    if (values["attachment-cache-dir"]) {
      dirs.add(values["attachment-cache-dir"]);
    }

    if (dirs.size) {
      lines.push("    volumes:");
      [...dirs].sort().forEach((dir) => {
        lines.push(`      - ${dir}:${dir}`);
      });
    }

    lines.push(
      "    ports:",
      "      - \"80:80\"",
      "    restart: unless-stopped"
    );

    return lines.join("\n");
  }

  function generateEnvVars(values) {
    const lines = [];

    CONFIG.forEach((c) => {
      if (!(c.key in values)) return;
      const val = c.type === "bool" ? "true" : values[c.key];
      lines.push(`${c.env}=${escapeShellValue(val)}`);
    });

    const users = formatAuthUsers(values);
    if (users) {
      lines.push(`NTFY_AUTH_USERS=${escapeShellValue(users.join(","))}`);
    }

    const acls = formatAuthAcls(values);
    if (acls) {
      lines.push(`NTFY_AUTH_ACCESS=${escapeShellValue(acls.join(","))}`);
    }

    const tokens = formatAuthTokens(values);
    if (tokens) {
      lines.push(`NTFY_AUTH_TOKENS=${escapeShellValue(tokens.join(","))}`);
    }

    return lines.join("\n");
  }

  // --- Web Push VAPID key generation (P-256 ECDH) ---

  function generateVAPIDKeys() {
    return crypto.subtle.generateKey(
      { name: "ECDH", namedCurve: "P-256" },
      true,
      ["deriveBits"]
    ).then((keyPair) => {
      return Promise.all([
        crypto.subtle.exportKey("raw", keyPair.publicKey),
        crypto.subtle.exportKey("pkcs8", keyPair.privateKey)
      ]);
    }).then((keys) => {
      const pubBytes = new Uint8Array(keys[0]);
      const privPkcs8 = new Uint8Array(keys[1]);
      // Extract raw 32-byte private key from PKCS#8 (last 32 bytes of the DER)
      const privBytes = privPkcs8.slice(privPkcs8.length - 32);
      return {
        publicKey: arrayToBase64Url(pubBytes),
        privateKey: arrayToBase64Url(privBytes)
      };
    });
  }

  function arrayToBase64Url(arr) {
    let str = "";
    for (let i = 0; i < arr.length; i++) {
      str += String.fromCharCode(arr[i]);
    }
    return btoa(str).replace(/\+/g, "-").replace(/\//g, "_").replace(/=+$/, "");
  }

  // --- Output + validation ---

  function updateOutput(els) {
    const { modal, codeEl, warningsEl } = els;
    if (!codeEl) return;

    const values = collectValues(els);
    const activeTab = modal.querySelector(".cg-output-tab.active");
    const format = activeTab ? activeTab.getAttribute("data-format") : "server-yml";

    const hasValues = Object.keys(values).length > 0;
    if (!hasValues) {
      codeEl.innerHTML = "<span class=\"cg-empty-msg\">Configure options on the left to generate your config...</span>";
      setHidden(warningsEl, true);
      return;
    }

    let output;
    if (format === "docker-compose") {
      output = generateDockerCompose(values);
    } else if (format === "env-vars") {
      output = generateEnvVars(values);
    } else {
      output = generateServerYml(values);
    }

    codeEl.textContent = output;

    // Validation warnings
    const warnings = validate(values);
    if (warningsEl) {
      if (warnings.length) {
        warningsEl.innerHTML = warnings.map((w) => `<div class="cg-warning">${w}</div>`).join("");
      }
      setHidden(warningsEl, !warnings.length);
    }
  }

  function validate(values) {
    const warnings = [];
    const baseUrl = values["base-url"] || "";

    // base-url format
    if (baseUrl) {
      if (!baseUrl.startsWith("http://") && !baseUrl.startsWith("https://")) {
        warnings.push("base-url must start with http:// or https://");
      } else {
        try {
          const u = new URL(baseUrl);
          if (u.pathname !== "/" && u.pathname !== "") {
            warnings.push("base-url must not have a path, ntfy does not support sub-paths");
          }
        } catch (e) {
          warnings.push("base-url is not a valid URL");
        }
      }
    }

    // database-url must start with postgres://
    if (values["database-url"] && !values["database-url"].startsWith("postgres://")) {
      warnings.push("database-url must start with postgres://");
    }

    // Web push requires all fields + base-url
    const wpPublic = values["web-push-public-key"];
    const wpPrivate = values["web-push-private-key"];
    const wpEmail = values["web-push-email-address"];
    const wpFile = values["web-push-file"];
    const dbUrl = values["database-url"];
    if (wpPublic || wpPrivate || wpEmail) {
      const missing = [];
      if (!wpPublic) missing.push("web-push-public-key");
      if (!wpPrivate) missing.push("web-push-private-key");
      if (!wpFile && !dbUrl) missing.push("web-push-file or database-url");
      if (!wpEmail) missing.push("web-push-email-address");
      if (!baseUrl) missing.push("base-url");
      if (missing.length) {
        warnings.push(`Web push requires: ${missing.join(", ")}`);
      }
    }

    // SMTP sender requires base-url and smtp-sender-from
    if (values["smtp-sender-addr"]) {
      const smtpMissing = [];
      if (!baseUrl) smtpMissing.push("base-url");
      if (!values["smtp-sender-from"]) smtpMissing.push("smtp-sender-from");
      if (smtpMissing.length) {
        warnings.push(`Email sending requires: ${smtpMissing.join(", ")}`);
      }
    }

    // SMTP server requires domain
    if (values["smtp-server-listen"] && !values["smtp-server-domain"]) {
      warnings.push("Email publishing requires smtp-server-domain");
    }

    // Attachments require base-url
    if (values["attachment-cache-dir"] && !baseUrl) {
      warnings.push("Attachments require base-url to be set");
    }

    // Upstream requires base-url and can't equal it
    if (values["upstream-base-url"]) {
      if (!baseUrl) {
        warnings.push("Upstream server requires base-url to be set");
      } else if (baseUrl === values["upstream-base-url"]) {
        warnings.push("base-url and upstream-base-url cannot be the same");
      }
    }

    // enable-signup requires enable-login
    if (values["enable-signup"] && !values["enable-login"]) {
      warnings.push("Enable signup requires enable-login to also be set");
    }

    // Duration field validation
    [
      { key: "cache-duration", label: "Cache duration" },
      { key: "attachment-expiry-duration", label: "Attachment expiry duration" }
    ].forEach((f) => {
      if (values[f.key] && !durationRegex.test(values[f.key])) {
        warnings.push(`${f.label} must be a valid duration (e.g. 12h, 3d, 30m, 60s)`);
      }
    });

    // Size field validation
    [
      { key: "attachment-file-size-limit", label: "Attachment file size limit" },
      { key: "attachment-total-size-limit", label: "Attachment total size limit" }
    ].forEach((f) => {
      if (values[f.key] && !sizeRegex.test(values[f.key])) {
        warnings.push(`${f.label} must be a valid size (e.g. 15M, 5G, 100K)`);
      }
    });

    return warnings;
  }

  // --- Helpers ---

  function secureRandomInt(max) {
    const arr = new Uint32Array(1);
    crypto.getRandomValues(arr);
    return arr[0] % max;
  }

  function generateToken() {
    const chars = "abcdefghijklmnopqrstuvwxyz0123456789";
    let token = "tk_";
    for (let i = 0; i < 29; i++) {
      token += chars.charAt(secureRandomInt(chars.length));
    }
    return token;
  }

  function generatePassword() {
    const chars = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789";
    let password = "";
    for (let i = 0; i < 16; i++) {
      password += chars.charAt(secureRandomInt(chars.length));
    }
    return password;
  }

  function escapeHtml(str) {
    return str.replace(/&/g, "&amp;").replace(/"/g, "&quot;").replace(/</g, "&lt;").replace(/>/g, "&gt;");
  }

  function escapeYamlValue(str) {
    return str.replace(/\\/g, "\\\\").replace(/"/g, '\\"');
  }

  function escapeShellValue(val) {
    // Use single quotes for values with $, double quotes otherwise
    // Escape the chosen quote character within the value
    if (val.includes("$")) {
      return "'" + val.replace(/'/g, "'\\''") + "'";
    }
    return '"' + val.replace(/\\/g, "\\\\").replace(/"/g, '\\"') + '"';
  }

  function prefill(modal, key, value) {
    const el = modal.querySelector(`[data-key="${key}"]`);
    if (el && !el.value.trim() && !el.dataset.cleared) el.value = value;
  }

  function switchPanel(modal, panelId) {
    modal.querySelectorAll(".cg-nav-tab").forEach((t) => t.classList.remove("active"));
    modal.querySelectorAll(".cg-panel").forEach((p) => p.classList.remove("active"));

    const navTab = modal.querySelector(`[data-panel="${panelId}"]`);
    const panel = modal.querySelector(`#${panelId}`);
    if (navTab) navTab.classList.add("active");
    if (panel) panel.classList.add("active");
  }

  function setHidden(el, hidden) {
    if (!el) return;
    if (hidden) {
      el.classList.add("cg-hidden");
    } else {
      el.classList.remove("cg-hidden");
    }
  }

  // --- Visibility: broken into focused helpers ---

  function syncRadiosToHiddenInputs(els) {
    const { modal, accessSelect, accessHidden, loginHidden, requireLoginHidden, signupHidden, proxyCheckbox } = els;

    // Proxy radio → hidden checkbox
    const proxyYes = modal.querySelector("input[name=\"cg-proxy\"][value=\"yes\"]");
    if (proxyYes && proxyCheckbox) {
      proxyCheckbox.checked = proxyYes.checked;
    }

    // Default access select → hidden input
    if (accessSelect && accessHidden) {
      accessHidden.value = accessSelect.value;
    }

    // Login mode three-way toggle → hidden checkboxes
    const loginMode = modal.querySelector("input[name=\"cg-login-mode\"]:checked");
    const loginModeVal = loginMode ? loginMode.value : "disabled";
    if (loginHidden) loginHidden.checked = (loginModeVal === "enabled" || loginModeVal === "required");
    if (requireLoginHidden) requireLoginHidden.checked = (loginModeVal === "required");

    const signupYes = modal.querySelector("input[name=\"cg-enable-signup\"][value=\"yes\"]");
    if (signupYes && signupHidden) signupHidden.checked = signupYes.checked;

    // SMTP sender verify radio → hidden checkbox
    const smtpVerifyYes = modal.querySelector("input[name=\"cg-smtp-sender-verify\"][value=\"yes\"]");
    if (smtpVerifyYes && els.smtpSenderVerifyHidden) els.smtpSenderVerifyHidden.checked = smtpVerifyYes.checked;

    return loginModeVal;
  }

  function updateFeatureVisibility(els, flags) {
    const { modal, dbStep, navDb, navEmail, emailOutSection, emailInSection } = els;
    const { authEnabled, cacheEnabled, webpushEnabled, smtpOutEnabled, smtpInEnabled, needsDb, isPostgres } = flags;

    // Show database question only if a DB-dependent feature is selected
    setHidden(dbStep, !needsDb);

    // Nav tabs for features
    for (const featId in NAV_MAP) {
      const checkbox = modal.querySelector(`#${featId}`);
      const navTab = modal.querySelector(`#${NAV_MAP[featId]}`);
      if (checkbox && navTab) {
        setHidden(navTab, !checkbox.checked);
      }
    }

    // Email tab — show if either outgoing or incoming is enabled
    setHidden(navEmail, !smtpOutEnabled && !smtpInEnabled);
    setHidden(emailOutSection, !smtpOutEnabled);
    setHidden(emailInSection, !smtpInEnabled);

    // Show/hide configure buttons next to feature checkboxes
    modal.querySelectorAll(".cg-btn-configure").forEach((btn) => {
      const row = btn.closest(".cg-feature-row");
      if (!row) return;
      const cb = row.querySelector("input[type=\"checkbox\"]");
      setHidden(btn, !(cb && cb.checked));
    });

    // If active nav tab got hidden, switch to General
    const activeNav = modal.querySelector(".cg-nav-tab.active");
    if (activeNav && activeNav.classList.contains("cg-hidden")) {
      switchPanel(modal, "cg-panel-general");
    }

    // Database tab — show only when PostgreSQL is selected and a DB-dependent feature is on
    setHidden(navDb, !(needsDb && isPostgres));
  }

  function updatePostgresFields(modal, isPostgres) {
    // Show "Using PostgreSQL" instead of file inputs when PostgreSQL is selected
    ["auth-file", "web-push-file", "cache-file"].forEach((key) => {
      const input = modal.querySelector(`[data-key="${key}"]`);
      if (!input) return;
      const field = input.closest(".cg-field");
      if (!field) return;
      input.style.display = isPostgres ? "none" : "";
      if (isPostgres) {
        input.dataset.pgDisabled = "1";
      } else {
        delete input.dataset.pgDisabled;
      }
      let pgLabel = field.querySelector(".cg-pg-label");
      if (isPostgres) {
        if (!pgLabel) {
          pgLabel = document.createElement("span");
          pgLabel.className = "cg-pg-label";
          pgLabel.textContent = "Using PostgreSQL";
          input.parentNode.insertBefore(pgLabel, input.nextSibling);
        }
        pgLabel.style.display = "";
      } else if (pgLabel) {
        pgLabel.style.display = "none";
      }
    });

    // iOS question → upstream-base-url
    const iosYes = modal.querySelector("input[name=\"cg-ios\"][value=\"yes\"]");
    const upstreamInput = modal.querySelector("[data-key=\"upstream-base-url\"]");
    if (iosYes && upstreamInput) {
      upstreamInput.value = iosYes.checked ? "https://ntfy.sh" : "";
    }
  }

  function prefillDefaults(modal, flags) {
    const {
      isPostgres,
      authEnabled,
      cacheEnabled,
      attachEnabled,
      webpushEnabled,
      smtpOutEnabled,
      smtpInEnabled
    } = flags;

    if (isPostgres) {
      prefill(modal, "database-url", "postgres://user:pass@host:5432/ntfy");
    }

    if (authEnabled) {
      if (!isPostgres) prefill(modal, "auth-file", "/var/lib/ntfy/auth.db");
    }

    if (cacheEnabled) {
      if (!isPostgres) prefill(modal, "cache-file", "/var/cache/ntfy/cache.db");
    }

    if (attachEnabled) {
      prefill(modal, "attachment-cache-dir", "/var/cache/ntfy/attachments");
    }

    if (webpushEnabled) {
      if (!isPostgres) prefill(modal, "web-push-file", "/var/lib/ntfy/webpush.db");
      prefill(modal, "web-push-email-address", "admin@example.com");
    }

    if (smtpOutEnabled) {
      prefill(modal, "smtp-sender-addr", "smtp.example.com:587");
      prefill(modal, "smtp-sender-from", "ntfy@example.com");
      prefill(modal, "smtp-sender-user", "yoursmtpuser");
      prefill(modal, "smtp-sender-pass", "yoursmtppass");
    }

    if (smtpInEnabled) {
      prefill(modal, "smtp-server-listen", ":25");
      prefill(modal, "smtp-server-domain", "ntfy.example.com");
    }
  }

  function autoDetectServerType(els, loginModeVal) {
    const { modal, accessSelect } = els;
    const serverTypeRadio = modal.querySelector("input[name=\"cg-server-type\"]:checked");
    const serverType = serverTypeRadio ? serverTypeRadio.value : "open";

    if (serverType !== "custom") {
      const currentAccess = accessSelect ? accessSelect.value : "read-write";
      const currentLoginEnabled = loginModeVal !== "disabled";
      const matchesOpen = currentAccess === "read-write" && !currentLoginEnabled;
      const matchesPrivate = currentAccess === "deny-all" && currentLoginEnabled;
      if (!matchesOpen && !matchesPrivate) {
        const customRadio = modal.querySelector("input[name=\"cg-server-type\"][value=\"custom\"]");
        if (customRadio) customRadio.checked = true;
      }
    }
  }

  function updateVisibility(els) {
    const {
      modal,
      authCheckbox,
      cacheCheckbox,
      attachCheckbox,
      webpushCheckbox,
      smtpOutCheckbox,
      smtpInCheckbox
    } = els;

    const isPostgresRadio = modal.querySelector("input[name=\"cg-db-type\"][value=\"postgres\"]");
    const isPostgres = isPostgresRadio && isPostgresRadio.checked;

    // Auto-enable auth when PostgreSQL is selected
    if (isPostgres && authCheckbox && !authCheckbox.checked) {
      authCheckbox.checked = true;
    }

    const authEnabled = authCheckbox && authCheckbox.checked;
    const cacheEnabled = cacheCheckbox && cacheCheckbox.checked;
    const attachEnabled = attachCheckbox && attachCheckbox.checked;
    const webpushEnabled = webpushCheckbox && webpushCheckbox.checked;
    const smtpOutEnabled = smtpOutCheckbox && smtpOutCheckbox.checked;
    const smtpInEnabled = smtpInCheckbox && smtpInCheckbox.checked;
    const needsDb = authEnabled || cacheEnabled || webpushEnabled;

    const flags = {
      isPostgres,
      authEnabled,
      cacheEnabled,
      attachEnabled,
      webpushEnabled,
      smtpOutEnabled,
      smtpInEnabled,
      needsDb
    };

    const loginModeVal = syncRadiosToHiddenInputs(els);
    updateFeatureVisibility(els, flags);
    updatePostgresFields(modal, isPostgres);
    prefillDefaults(modal, flags);
    autoDetectServerType(els, loginModeVal);
  }

  // --- Repeatable rows ---

  function addRepeatableRow(container, type, onUpdate) {
    const row = document.createElement("div");
    row.className = `cg-repeatable-row cg-auth-${type}-row`;

    if (type === "user") {
      const username = `newuser${secureRandomInt(100) + 1}`;
      row.innerHTML =
        `<input type="text" data-field="username" placeholder="Username" value="${escapeHtml(username)}">` +
        `<input type="text" data-field="password" placeholder="Password" value="${escapeHtml(generatePassword())}">` +
        "<select data-field=\"role\"><option value=\"user\">User</option><option value=\"admin\">Admin</option></select>" +
        "<button type=\"button\" class=\"cg-btn-remove\" title=\"Remove\">&times;</button>";
    } else if (type === "acl") {
      let aclUser = `someuser${secureRandomInt(100) + 1}`;
      const modal = container.closest(".cg-modal");
      if (modal) {
        const userRows = modal.querySelectorAll(".cg-auth-user-row");
        for (const ur of userRows) {
          const role = ur.querySelector("[data-field=\"role\"]");
          const name = ur.querySelector("[data-field=\"username\"]");
          if (role && role.value !== "admin" && name && name.value.trim()) {
            aclUser = name.value.trim();
            break;
          }
        }
      }
      row.innerHTML =
        `<input type="text" data-field="username" placeholder="Username (* for everyone)" value="${escapeHtml(aclUser)}">` +
        "<input type=\"text\" data-field=\"topic\" placeholder=\"Topic pattern\" value=\"sometopic*\">" +
        "<select data-field=\"permission\"><option value=\"read-write\">Read &amp; Write</option><option value=\"read-only\">Read Only</option><option value=\"write-only\">Write Only</option><option value=\"deny\">Deny</option></select>" +
        "<button type=\"button\" class=\"cg-btn-remove\" title=\"Remove\">&times;</button>";
    } else if (type === "token") {
      let tokenUser = "";
      const modal = container.closest(".cg-modal");
      if (modal) {
        const firstRow = modal.querySelector(".cg-auth-user-row");
        const name = firstRow ? firstRow.querySelector("[data-field=\"username\"]") : null;
        if (name && name.value.trim()) tokenUser = name.value.trim();
      }
      row.innerHTML =
        `<input type="text" data-field="username" placeholder="Username" value="${escapeHtml(tokenUser)}">` +
        `<input type="text" data-field="token" placeholder="Token" value="${escapeHtml(generateToken())}">` +
        "<input type=\"text\" data-field=\"label\" placeholder=\"Label (optional)\">" +
        "<button type=\"button\" class=\"cg-btn-remove\" title=\"Remove\">&times;</button>";
    }

    row.querySelector(".cg-btn-remove").addEventListener("click", () => {
      row.remove();
      onUpdate();
    });
    row.querySelectorAll("input, select").forEach((el) => {
      el.addEventListener("input", onUpdate);
    });

    container.appendChild(row);
  }

  // --- Modal functions (module-level) ---

  function openModal(els) {
    els.modal.style.display = "";
    document.body.style.overflow = "hidden";
    updateVisibility(els);
    updateOutput(els);
  }

  function closeModal(els) {
    els.modal.style.display = "none";
    document.body.style.overflow = "";
  }

  function resetAll(els) {
    const { modal } = els;

    // Reset all text/password inputs and clear flags
    modal.querySelectorAll("input[type=\"text\"], input[type=\"password\"]").forEach((el) => {
      el.value = "";
      delete el.dataset.cleared;
    });
    // Uncheck all checkboxes
    modal.querySelectorAll("input[type=\"checkbox\"]").forEach((el) => {
      el.checked = false;
      el.disabled = false;
    });
    // Reset radio buttons to first option
    const radioGroups = {};
    modal.querySelectorAll("input[type=\"radio\"]").forEach((el) => {
      if (!radioGroups[el.name]) {
        radioGroups[el.name] = true;
        const first = modal.querySelector(`input[type="radio"][name="${el.name}"]`);
        if (first) first.checked = true;
      } else {
        el.checked = false;
      }
    });
    // Reset selects to first option
    modal.querySelectorAll("select").forEach((el) => {
      el.selectedIndex = 0;
    });
    // Remove all repeatable rows
    modal.querySelectorAll(".cg-auth-user-row, .cg-auth-acl-row, .cg-auth-token-row").forEach((row) => {
      row.remove();
    });
    // Re-prefill base-url
    const baseUrlInput = modal.querySelector("[data-key=\"base-url\"]");
    if (baseUrlInput) {
      baseUrlInput.value = "https://ntfy.example.com";
    }
    // Reset to General tab
    switchPanel(modal, "cg-panel-general");
    updateVisibility(els);
    updateOutput(els);
  }

  function fillVAPIDKeys(els) {
    const { modal } = els;
    generateVAPIDKeys().then((keys) => {
      const pubInput = modal.querySelector("[data-key=\"web-push-public-key\"]");
      const privInput = modal.querySelector("[data-key=\"web-push-private-key\"]");
      if (pubInput) pubInput.value = keys.publicKey;
      if (privInput) privInput.value = keys.privateKey;
      updateOutput(els);
    });
  }

  // --- Event setup (grouped) ---

  function setupModalEvents(els) {
    const { modal } = els;
    const openBtn = document.getElementById("cg-open-btn");
    const closeBtn = document.getElementById("cg-close-btn");
    const backdrop = modal.querySelector(".cg-modal-backdrop");
    const resetBtn = document.getElementById("cg-reset-btn");

    if (openBtn) openBtn.addEventListener("click", () => openModal(els));
    if (closeBtn) closeBtn.addEventListener("click", () => closeModal(els));
    if (resetBtn) resetBtn.addEventListener("click", () => resetAll(els));
    if (backdrop) backdrop.addEventListener("click", () => closeModal(els));

    document.addEventListener("keydown", (e) => {
      if (e.key === "Escape" && modal.style.display !== "none") {
        closeModal(els);
      }
    });

    // Mobile toggle between Edit and Preview panels
    const toggleBtns = modal.querySelectorAll(".cg-mobile-toggle-btn");
    const leftPanel = document.getElementById("cg-left");
    const rightPanel = document.getElementById("cg-right");
    toggleBtns.forEach((btn) => {
      btn.addEventListener("click", () => {
        toggleBtns.forEach((b) => b.classList.remove("active"));
        btn.classList.add("active");
        if (btn.dataset.show === "right") {
          leftPanel.classList.add("cg-mobile-hidden");
          rightPanel.classList.add("cg-mobile-active");
        } else {
          leftPanel.classList.remove("cg-mobile-hidden");
          rightPanel.classList.remove("cg-mobile-active");
        }
      });
    });
  }

  function setupAuthEvents(els) {
    const { modal, authCheckbox, accessSelect } = els;
    if (!authCheckbox) return;

    // Auth checkbox: clean up when unchecked
    authCheckbox.addEventListener("change", () => {
      if (!authCheckbox.checked) {
        // Clear auth-file
        const authFile = modal.querySelector("[data-key=\"auth-file\"]");
        if (authFile) {
          authFile.value = "";
          delete authFile.dataset.cleared;
        }
        // Reset default access
        if (accessSelect) accessSelect.value = "read-write";
        // Reset login mode to Disabled and unset hidden checkboxes
        const loginDisabled = modal.querySelector("input[name=\"cg-login-mode\"][value=\"disabled\"]");
        if (loginDisabled) loginDisabled.checked = true;
        if (els.loginHidden) els.loginHidden.checked = false;
        if (els.requireLoginHidden) els.requireLoginHidden.checked = false;
        const signupNo = modal.querySelector("input[name=\"cg-enable-signup\"][value=\"no\"]");
        if (signupNo) signupNo.checked = true;
        if (els.signupHidden) els.signupHidden.checked = false;
        // Reset UnifiedPush to No
        const upNo = modal.querySelector("input[name=\"cg-unifiedpush\"][value=\"no\"]");
        if (upNo) upNo.checked = true;
        // Remove provisioned users/ACLs/tokens
        modal.querySelectorAll(".cg-auth-user-row, .cg-auth-acl-row, .cg-auth-token-row").forEach((row) => {
          row.remove();
        });
        // Switch server type to Open
        const openRadio = modal.querySelector("input[name=\"cg-server-type\"][value=\"open\"]");
        if (openRadio) openRadio.checked = true;
      }
    });
  }

  function setupServerTypeEvents(els) {
    const { modal, authCheckbox, accessSelect } = els;

    modal.querySelectorAll("input[name=\"cg-server-type\"]").forEach((radio) => {
      radio.addEventListener("change", () => {
        const loginDisabledRadio = modal.querySelector("input[name=\"cg-login-mode\"][value=\"disabled\"]");
        const loginRequiredRadio = modal.querySelector("input[name=\"cg-login-mode\"][value=\"required\"]");
        if (radio.value === "open") {
          if (accessSelect) accessSelect.value = "read-write";
          if (loginDisabledRadio) loginDisabledRadio.checked = true;
          if (authCheckbox) authCheckbox.checked = false;
          // Trigger the auth cleanup
          authCheckbox.dispatchEvent(new Event("change"));
        } else if (radio.value === "private") {
          // Enable auth with required login
          if (authCheckbox) authCheckbox.checked = true;
          if (accessSelect) accessSelect.value = "deny-all";
          if (loginRequiredRadio) loginRequiredRadio.checked = true;
          if (els.loginHidden) els.loginHidden.checked = true;
          if (els.requireLoginHidden) els.requireLoginHidden.checked = true;
          // Add default admin user if no users exist
          const usersContainer = modal.querySelector("#cg-auth-users-container");
          if (usersContainer && !usersContainer.querySelector(".cg-auth-user-row")) {
            const onUpdate = () => {
              updateVisibility(els);
              updateOutput(els);
            };
            addRepeatableRow(usersContainer, "user", onUpdate);
            const adminRow = usersContainer.querySelector(".cg-auth-user-row:last-child");
            if (adminRow) {
              const u = adminRow.querySelector("[data-field=\"username\"]");
              const p = adminRow.querySelector("[data-field=\"password\"]");
              const r = adminRow.querySelector("[data-field=\"role\"]");
              if (u) u.value = "ntfyadmin";
              if (p) p.value = generatePassword();
              if (r) r.value = "admin";
            }
            addRepeatableRow(usersContainer, "user", onUpdate);
            const userRow = usersContainer.querySelector(".cg-auth-user-row:last-child");
            if (userRow) {
              const u = userRow.querySelector("[data-field=\"username\"]");
              const p = userRow.querySelector("[data-field=\"password\"]");
              if (u) u.value = "ntfyuser";
              if (p) p.value = generatePassword();
            }
          }
        }
        // "custom" doesn't change anything
      });
    });
  }

  function setupUnifiedPushEvents(els) {
    const { modal } = els;
    const onUpdate = () => {
      updateVisibility(els);
      updateOutput(els);
    };

    modal.querySelectorAll("input[name=\"cg-unifiedpush\"]").forEach((radio) => {
      radio.addEventListener("change", () => {
        const aclsContainer = modal.querySelector("#cg-auth-acls-container");
        if (!aclsContainer) return;
        const existing = aclsContainer.querySelector(".cg-auth-acl-row-up");
        if (radio.value === "yes" && radio.checked && !existing) {
          // Enable auth if not already enabled
          if (els.authCheckbox && !els.authCheckbox.checked) {
            els.authCheckbox.checked = true;
          }
          // Add a disabled UnifiedPush ACL row
          const row = document.createElement("div");
          row.className = "cg-repeatable-row cg-auth-acl-row cg-auth-acl-row-up";
          row.innerHTML =
            "<input type=\"text\" data-field=\"username\" value=\"*\" disabled>" +
            "<input type=\"text\" data-field=\"topic\" value=\"up*\" disabled>" +
            "<select data-field=\"permission\" disabled><option value=\"write-only\">Write Only</option></select>" +
            "<button type=\"button\" class=\"cg-btn-remove\" title=\"Removing this ACL entry will disable UnifiedPush support\">&times;</button>";
          row.querySelector(".cg-btn-remove").addEventListener("click", () => {
            row.remove();
            const upNo = modal.querySelector("input[name=\"cg-unifiedpush\"][value=\"no\"]");
            if (upNo) upNo.checked = true;
            onUpdate();
          });
          // Insert at the beginning
          aclsContainer.insertBefore(row, aclsContainer.firstChild);
          onUpdate();
        } else if (radio.value === "no" && radio.checked && existing) {
          existing.remove();
          onUpdate();
        }
      });
    });
  }

  function setupFormListeners(els) {
    const { modal } = els;
    const onUpdate = () => {
      updateVisibility(els);
      updateOutput(els);
    };

    // Left nav tab switching
    modal.querySelectorAll(".cg-nav-tab").forEach((tab) => {
      tab.addEventListener("click", () => {
        const panelId = tab.getAttribute("data-panel");
        switchPanel(modal, panelId);
      });
    });

    // Configure buttons in feature grid
    modal.querySelectorAll(".cg-btn-configure").forEach((btn) => {
      btn.addEventListener("click", () => {
        const panelId = btn.getAttribute("data-panel");
        if (panelId) switchPanel(modal, panelId);
      });
    });

    // Output format tab switching
    modal.querySelectorAll(".cg-output-tab").forEach((tab) => {
      tab.addEventListener("click", () => {
        modal.querySelectorAll(".cg-output-tab").forEach((t) => t.classList.remove("active"));
        tab.classList.add("active");
        updateOutput(els);
      });
    });

    // All form inputs trigger update
    modal.querySelectorAll("input, select").forEach((el) => {
      const evt = (el.type === "checkbox" || el.type === "radio") ? "change" : "input";
      el.addEventListener(evt, () => {
        // Mark text fields as cleared when user empties them
        if ((el.type === "text" || el.type === "password") && el.dataset.key && !el.value.trim()) {
          el.dataset.cleared = "1";
        } else if ((el.type === "text" || el.type === "password") && el.dataset.key && el.value.trim()) {
          delete el.dataset.cleared;
        }
        onUpdate();
      });
    });

    // Add buttons for repeatable rows
    modal.querySelectorAll(".cg-btn-add[data-add-type]").forEach((btn) => {
      btn.addEventListener("click", () => {
        const type = btn.getAttribute("data-add-type");
        let container = btn.previousElementSibling;
        if (!container) container = btn.parentElement.querySelector(".cg-repeatable-container");
        addRepeatableRow(container, type, onUpdate);
        onUpdate();
      });
    });

    // Copy button
    const copyBtn = modal.querySelector("#cg-copy-btn");
    if (copyBtn) {
      const copyIcon = "<svg xmlns=\"http://www.w3.org/2000/svg\" width=\"14\" height=\"14\" viewBox=\"0 0 24 24\" fill=\"none\" stroke=\"currentColor\" stroke-width=\"2\" stroke-linecap=\"round\" stroke-linejoin=\"round\"><rect x=\"9\" y=\"9\" width=\"13\" height=\"13\" rx=\"2\" ry=\"2\"></rect><path d=\"M5 15H4a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2h9a2 2 0 0 1 2 2v1\"></path></svg>";
      const checkIcon = "<svg xmlns=\"http://www.w3.org/2000/svg\" width=\"14\" height=\"14\" viewBox=\"0 0 24 24\" fill=\"none\" stroke=\"currentColor\" stroke-width=\"2\" stroke-linecap=\"round\" stroke-linejoin=\"round\"><polyline points=\"20 6 9 17 4 12\"></polyline></svg>";
      copyBtn.addEventListener("click", () => {
        const code = modal.querySelector("#cg-code");
        if (code && code.textContent) {
          navigator.clipboard.writeText(code.textContent).then(() => {
            copyBtn.innerHTML = checkIcon;
            copyBtn.style.color = "var(--md-primary-fg-color)";
            setTimeout(() => {
              copyBtn.innerHTML = copyIcon;
              copyBtn.style.color = "";
            }, 2000);
          });
        }
      });
    }
  }

  function setupWebPushEvents(els) {
    const { modal } = els;
    let vapidKeysGenerated = false;
    const regenBtn = modal.querySelector("#cg-regen-keys");

    if (regenBtn) {
      regenBtn.addEventListener("click", () => fillVAPIDKeys(els));
    }

    // Auto-generate keys when web push is first enabled
    const webpushFeat = modal.querySelector("#cg-feat-webpush");
    if (webpushFeat) {
      webpushFeat.addEventListener("change", () => {
        if (webpushFeat.checked && !vapidKeysGenerated) {
          vapidKeysGenerated = true;
          fillVAPIDKeys(els);
        }
      });
    }
  }

  // --- Init ---

  function initGenerator() {
    const modal = document.getElementById("cg-modal");
    if (!modal) return;

    const els = cacheElements(modal);

    setupModalEvents(els);
    setupAuthEvents(els);
    setupServerTypeEvents(els);
    setupUnifiedPushEvents(els);
    setupFormListeners(els);
    setupWebPushEvents(els);

    // Pre-fill base-url
    const baseUrlInput = modal.querySelector("[data-key=\"base-url\"]");
    if (baseUrlInput && !baseUrlInput.value.trim()) {
      baseUrlInput.value = "https://ntfy.example.com";
    }

    // Auto-open if URL hash points to config generator
    if (window.location.hash === "#config-generator") {
      openModal(els);
    }
  }

  if (document.readyState === "loading") {
    document.addEventListener("DOMContentLoaded", initGenerator);
  } else {
    initGenerator();
  }
})();
