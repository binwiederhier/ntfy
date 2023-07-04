import React from "react";

import rtlPlugin from "stylis-plugin-rtl";
import { CacheProvider } from "@emotion/react";
import createCache from "@emotion/cache";
import { prefixer } from "stylis";
import { useTranslation } from "react-i18next";

// https://mui.com/material-ui/guides/right-to-left

const cacheRtl = createCache({
  key: "muirtl",
  stylisPlugins: [prefixer, rtlPlugin],
});

const RTLCacheProvider = ({ children }) => {
  const { i18n } = useTranslation();

  return i18n.dir() === "rtl" ? <CacheProvider value={cacheRtl}>{children}</CacheProvider> : children;
};

export default RTLCacheProvider;
