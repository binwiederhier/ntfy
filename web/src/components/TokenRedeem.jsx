import * as React from "react";
import { useEffect, useRef, useState } from "react";
import { Typography, Button, Box, CircularProgress } from "@mui/material";
import ErrorOutlineIcon from "@mui/icons-material/ErrorOutlineOutlined";
import { useNavigate } from "react-router-dom";
import { useTranslation } from "react-i18next";
import AvatarBox from "./AvatarBox";
import routes from "./routes";
import { EnrollCodeInvalidError } from "../app/errors";
import { fadeReload } from "../app/transition";
import { parseTokenRedeemFragment, redeemLink, SignedInAsOtherUserError } from "../app/tokenRedeem";

// TokenRedeem is the landing page for enrollment links (/account/token/redeem#code=..&user=..&subscribe=..):
// it signs the web app in with the one-time code and subscribes to the listed topics. The link is
// stripped from the URL on load to keep the code out of browser history.
const TokenRedeem = () => {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const [error, setError] = useState(null);
  const ran = useRef(false);

  useEffect(() => {
    if (ran.current) {
      return; // Guard against double-invoke (e.g. React StrictMode) consuming the code twice
    }
    ran.current = true;
    const link = parseTokenRedeemFragment(window.location.hash);
    window.history.replaceState(null, "", routes.tokenRedeem);
    (async () => {
      try {
        await redeemLink(link);
        fadeReload(routes.app);
      } catch (e) {
        console.log(`[TokenRedeem] Redeeming link failed`, e);
        if (e instanceof EnrollCodeInvalidError) {
          setError(t("token_redeem_error_invalid_description"));
        } else if (e instanceof SignedInAsOtherUserError) {
          setError(t("token_redeem_error_other_user_description", { username: e.username }));
        } else {
          setError(e.message);
        }
      }
    })();
  }, [t]);

  return (
    <AvatarBox>
      {!error && (
        <Box sx={{ display: "flex", alignItems: "center", gap: 1 }}>
          <CircularProgress size={24} />
          <Typography sx={{ typography: "h6" }}>{t("token_redeem_progress_title")}</Typography>
        </Box>
      )}
      {error && (
        <>
          <Box sx={{ display: "flex", alignItems: "center", gap: 1 }}>
            <ErrorOutlineIcon color="error" sx={{ fontSize: 28 }} />
            <Typography sx={{ typography: "h6" }}>{t("token_redeem_error_title")}</Typography>
          </Box>
          <Typography sx={{ mt: 1, textAlign: "center" }}>{error}</Typography>
          <Button onClick={() => navigate(routes.app)} variant="contained" sx={{ mt: 2 }}>
            {t("token_redeem_button_app")}
          </Button>
        </>
      )}
    </AvatarBox>
  );
};

export default TokenRedeem;
