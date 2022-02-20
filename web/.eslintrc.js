module.exports = {
    parser: "babel-eslint",
    env: {
        browser: true,
        commonjs: true,
        es6: true,
        node: true,
        jest: true,
    },
    parserOptions: {
        ecmaVersion: 2020,
        ecmaFeatures: {
            impliedStrict: true,
            jsx: true,
        },
        sourceType: "module",
    },
    plugins: ["react", "react-hooks"],
    extends: [
        "eslint:recommended",
        "plugin:react/recommended",
        "plugin:react-hooks/recommended",
    ],
    settings: {
        react: {
            version: "detect",
        },
    },
    rules: {
        // You can do your customizations here...
        // For example, if you don't want to use the prop-types package,
        // you can turn off that recommended rule with: 'react/prop-types': ['off']
    },
};
