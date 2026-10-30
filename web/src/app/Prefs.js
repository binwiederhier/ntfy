import db from "./db";

export const THEME = {
  DARK: "dark",
  LIGHT: "light",
  SYSTEM: "system",
};

// How the date component is displayed. SYSTEM follows the browser's locale (i.e. the OS region/
// format settings, not the chosen UI translation language); ISO8601 forces YYYY-MM-DD; DMY and MDY
// force the day/month order regardless of locale (e.g. for an English UI that defaults to US order).
export const DATE_FORMAT = {
  SYSTEM: "system",
  ISO8601: "iso8601",
  DMY: "dmy",
  DMY_DOT: "dmy_dot",
  MDY: "mdy",
};

// How the time component (clock) is displayed. Orthogonal to DATE_FORMAT and to the UI language --
// SYSTEM lets the browser locale decide, H12/H24 force a 12- or 24-hour clock. ISO 8601 dates are
// always 24-hour, so this setting is ignored in that mode.
export const TIME_FORMAT = {
  SYSTEM: "system",
  H12: "12h",
  H24: "24h",
};

// Default values the getters return when a pref is unset; also used by PrefCache (PrefCache.jsx).
export const PREF_DEFAULTS = {
  sound: "ding",
  minPriority: 1,
  deleteAfter: 604800, // one week
  theme: THEME.SYSTEM,
  dateFormat: DATE_FORMAT.SYSTEM,
  timeFormat: TIME_FORMAT.SYSTEM,
  webPushEnabled: false,
  notificationAlertDismissed: false,
};

// Guards against values the current build doesn't know about (e.g. a pref synced from a newer
// client, or a renamed enum). An unknown value would leave the Settings <Select> with no matching
// option, so we fall back to the default instead.
const isKnownValue = (constants, value) => Object.values(constants).includes(value);

export class Prefs {
  constructor(dbImpl) {
    this.db = dbImpl;
  }

  async setSound(sound) {
    this.db.prefs.put({ key: "sound", value: sound.toString() });
  }

  async sound() {
    const sound = await this.db.prefs.get("sound");
    return sound ? sound.value : PREF_DEFAULTS.sound;
  }

  async setMinPriority(minPriority) {
    this.db.prefs.put({ key: "minPriority", value: minPriority.toString() });
  }

  async minPriority() {
    const minPriority = await this.db.prefs.get("minPriority");
    return minPriority ? Number(minPriority.value) : PREF_DEFAULTS.minPriority;
  }

  async setDeleteAfter(deleteAfter) {
    await this.db.prefs.put({ key: "deleteAfter", value: deleteAfter.toString() });
  }

  async deleteAfter() {
    const deleteAfter = await this.db.prefs.get("deleteAfter");
    return deleteAfter ? Number(deleteAfter.value) : PREF_DEFAULTS.deleteAfter;
  }

  async webPushEnabled() {
    const webPushEnabled = await this.db.prefs.get("webPushEnabled");
    return webPushEnabled?.value ?? PREF_DEFAULTS.webPushEnabled;
  }

  async setWebPushEnabled(enabled) {
    await this.db.prefs.put({ key: "webPushEnabled", value: enabled });
  }

  // Whether the "Notifications are disabled/blocked" alert in the navigation was closed. Per browser,
  // since one machine may want desktop notifications and another not.
  async notificationAlertDismissed() {
    const dismissed = await this.db.prefs.get("notificationAlertDismissed");
    return dismissed?.value ?? PREF_DEFAULTS.notificationAlertDismissed;
  }

  async setNotificationAlertDismissed(dismissed) {
    await this.db.prefs.put({ key: "notificationAlertDismissed", value: dismissed });
  }

  async theme() {
    const theme = await this.db.prefs.get("theme");
    return theme?.value ?? PREF_DEFAULTS.theme;
  }

  async setTheme(mode) {
    await this.db.prefs.put({ key: "theme", value: mode });
  }

  async dateFormat() {
    const dateFormat = await this.db.prefs.get("dateFormat");
    return isKnownValue(DATE_FORMAT, dateFormat?.value) ? dateFormat.value : PREF_DEFAULTS.dateFormat;
  }

  async setDateFormat(format) {
    await this.db.prefs.put({ key: "dateFormat", value: format });
  }

  async timeFormat() {
    const timeFormat = await this.db.prefs.get("timeFormat");
    return isKnownValue(TIME_FORMAT, timeFormat?.value) ? timeFormat.value : PREF_DEFAULTS.timeFormat;
  }

  async setTimeFormat(format) {
    await this.db.prefs.put({ key: "timeFormat", value: format });
  }
}

const prefs = new Prefs(db());
export default prefs;
