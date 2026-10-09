/* eslint-env node */
import { readFileSync } from "node:fs";
import { resolve } from "node:path";
import { createElement } from "react";
import { renderToStaticMarkup } from "react-dom/server";
import i18next from "i18next";
import { I18nextProvider, Trans } from "react-i18next";
import { beforeAll, describe, expect, it, vi } from "vitest";
import initI18n from "./i18n";

// Serve /static/langs/<lng>.json from public/, the way the HTTP backend loads them in the browser
const langsDir = resolve(__dirname, "../../public/static/langs");
const fakeFetch = async (url) => {
  const lng = String(url).match(/\/static\/langs\/([^/]+)\.json$/)?.[1];
  try {
    const body = readFileSync(resolve(langsDir, `${lng}.json`), "utf-8");
    return new Response(body, { status: 200, headers: { "Content-Type": "application/json" } });
  } catch {
    return new Response("not found", { status: 404 });
  }
};

describe("i18n", () => {
  beforeAll(async () => {
    vi.stubGlobal("fetch", fakeFetch);
    window.location = { ...window.location, search: "", pathname: "/", hash: "" }; // Read by the language detector
    window.navigator = { language: "en", languages: ["en"] };
    vi.spyOn(console, "log").mockImplementation(() => {}); // debug: true is noisy
    await initI18n();
  });

  it("loads English and interpolates without escaping", async () => {
    await i18next.changeLanguage("en");
    expect(i18next.t("prefs_appearance_date_format_title")).toBe("Date format");
    expect(i18next.t("account_upgrade_dialog_tier_features_messages", { messages: "1,000", count: 1000 })).toBe("1,000 daily messages");
    expect(i18next.t("account_upgrade_dialog_tier_features_messages", { messages: "<b>1</b>", count: 1 })).toBe("<b>1</b> daily message");
  });

  it("uses each language's plural rules", async () => {
    await i18next.changeLanguage("pl");
    const t = (count) => i18next.t("account_upgrade_dialog_tier_features_reservations", { reservations: count, count });
    expect(t(1)).toBe("1 rezerwacja tematu"); // one
    expect(t(3)).toBe("3 rezerwacje tematów"); // few
    expect(t(5)).toBe("5 rezerwacji tematów"); // many
  });

  it("falls back to English for missing keys", async () => {
    await i18next.changeLanguage("xx");
    expect(i18next.t("prefs_appearance_date_format_title")).toBe("Date format");
  });

  it("renders named components in <Trans>", async () => {
    await i18next.changeLanguage("en");
    const html = renderToStaticMarkup(
      createElement(
        I18nextProvider,
        { i18n: i18next },
        createElement(Trans, {
          i18nKey: "notifications_more_details",
          components: { websiteLink: createElement("a", { href: "https://ntfy.sh" }), docsLink: createElement("a", { href: "/docs" }) },
        }),
      ),
    );
    expect(html).toBe('For more information, check out the <a href="https://ntfy.sh">website</a> or <a href="/docs">documentation</a>.');
  });
});
