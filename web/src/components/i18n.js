import i18n from 'i18next';
import Backend from 'i18next-http-backend';
import LanguageDetector from 'i18next-browser-languagedetector';
import { initReactI18next } from 'react-i18next';

// init i18next
// for all options read: https://www.i18next.com/overview/configuration-options
// learn more: https://github.com/i18next/i18next-browser-languageDetector
// learn more: https://github.com/i18next/i18next-http-backend

i18n
    .use(Backend)
    .use(LanguageDetector)
    .use(initReactI18next)
    .init({
        fallbackLng: 'en',
        debug: true,
        interpolation: {
            escapeValue: false, // not needed for react as it escapes by default
        },
        backend: {
            loadPath: '/static/langs/{{lng}}.json',
        }
    });

export default i18n;
