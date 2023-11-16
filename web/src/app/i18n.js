import i18next from "i18next";
import Backend from "i18next-http-backend";
import LanguageDetector from "i18next-browser-languagedetector";
import { initReactI18next } from "react-i18next";

// Translations using i18next
// - Options: https://www.i18next.com/overview/configuration-options
// - Browser Language Detector: https://github.com/i18next/i18next-browser-languageDetector
// - HTTP Backend (load files via fetch): https://github.com/i18next/i18next-http-backend
//
// See example project here:
// https://github.com/i18next/react-i18next/tree/master/example/react

const initI18n = () =>
  i18next
    .use(Backend)
    .use(LanguageDetector)
    .use(initReactI18next)
    .init({
      fallbackLng: "en",
      debug: true,
      interpolation: {
        escapeValue: false, // not needed for react as it escapes by default
      },
      backend: {
        loadPath: "/static/langs/{{lng}}.json",
      },
    });

export default initI18n;
