import js from "@eslint/js";
import prettier from "eslint-config-prettier";
import importPlugin from "eslint-plugin-import";
import jsxA11y from "eslint-plugin-jsx-a11y";
import react from "eslint-plugin-react";
import reactHooks from "eslint-plugin-react-hooks";
import globals from "globals";

export default [
  {
    ignores: ["src/app/emojis.js", "src/app/emojisMapped.js"],
  },
  js.configs.recommended,
  react.configs.flat.recommended,
  react.configs.flat["jsx-runtime"],
  {
    // Classic hooks rules only; the React Compiler rules in "recommended-latest" need refactoring first
    plugins: { "react-hooks": reactHooks },
    rules: { "react-hooks/rules-of-hooks": "error", "react-hooks/exhaustive-deps": "warn" },
  },
  jsxA11y.flatConfigs.recommended,
  importPlugin.flatConfigs.recommended,
  {
    files: ["**/*.{js,jsx}"],
    languageOptions: {
      ecmaVersion: 2023,
      sourceType: "module",
      globals: {
        ...globals.browser,
        config: "readonly",
      },
    },
    settings: {
      react: { version: "detect" },
      "import/resolver": { node: { extensions: [".js", ".jsx"] } },
    },
    rules: {
      "class-methods-use-this": "off",
      "func-style": ["error", "expression"],
      "no-restricted-syntax": ["error", "ForInStatement", "LabeledStatement", "WithStatement"],
      "no-await-in-loop": "error",
      "import/no-cycle": "warn",
      "react/prop-types": "off",
      "react/jsx-no-duplicate-props": [
        "error",
        {
          ignoreCase: false, // For <TextField>'s [iI]nputProps
        },
      ],
      "react/function-component-definition": [
        "error",
        {
          namedComponents: "arrow-function",
          unnamedComponents: "arrow-function",
        },
      ],

      // Rules we used to get from eslint-config-airbnb, and still want
      "no-unused-vars": ["error", { vars: "all", args: "after-used", ignoreRestSiblings: true, caughtErrors: "none" }],
      "react/jsx-uses-react": "error", // Files still import React, even with the automatic JSX runtime
      "react/display-name": "off", // forwardRef components (ReserveIcons.jsx) are anonymous on purpose
      "import/no-named-as-default": "off", // False positives for Dexie and i18next
      "import/no-named-as-default-member": "off",
      "no-bitwise": "error",
      "no-continue": "error",
      "no-param-reassign": ["error", { props: true }],
      "jsx-a11y/no-autofocus": ["error", { ignoreNonDOM: true }],
      "no-script-url": "error",
      "no-underscore-dangle": "error",
      "max-classes-per-file": ["error", 1],
      "import/no-extraneous-dependencies": "error",
      "prefer-const": "error",
      "no-var": "error",
      eqeqeq: ["error", "always", { null: "ignore" }],
    },
  },
  {
    files: ["src/**/*.test.{js,jsx}", "src/test/**/*.js"],
    languageOptions: { globals: globals.node },
  },
  {
    files: ["public/sw.js"],
    rules: { "no-restricted-globals": "off" },
  },
  prettier,
];
