/**
 * VersionChecker polls the /v1/config endpoint to detect new server versions
 * or configuration changes, prompting users to refresh the page.
 */

const intervalMillis = 5 * 60 * 1000; // 5 minutes

class VersionChecker {
  constructor() {
    this.initialConfigHash = null;
    this.listener = null;
    this.timer = null;
  }

  /**
   * Starts the version checker worker. It stores the initial config hash
   * from the config.js and polls the server every 5 minutes.
   */
  startWorker() {
    // Store initial config hash from the config loaded at page load
    this.initialConfigHash = window.config?.config_hash || "";
    console.log("[VersionChecker] Starting version checker");
    this.timer = setInterval(() => this.checkVersion(), intervalMillis);
  }

  stopWorker() {
    if (this.timer) {
      clearInterval(this.timer);
      this.timer = null;
    }
    console.log("[VersionChecker] Stopped version checker");
  }

  registerListener(listener) {
    this.listener = listener;
  }

  resetListener() {
    this.listener = null;
  }

  async checkVersion() {
    if (!this.initialConfigHash) {
      return;
    }

    try {
      const response = await fetch(`${window.config?.base_url || ""}/v1/config`);
      if (!response.ok) {
        console.log("[VersionChecker] Failed to fetch config:", response.status);
        return;
      }

      const data = await response.json();
      const currentHash = data.config_hash;

      if (currentHash && currentHash !== this.initialConfigHash) {
        console.log("[VersionChecker] Version or config changed, showing banner");
        if (this.listener) {
          this.listener();
        }
      } else {
        console.log("[VersionChecker] No version change detected");
      }
    } catch (error) {
      console.log("[VersionChecker] Error checking config:", error);
    }
  }
}

const versionChecker = new VersionChecker();
export default versionChecker;
