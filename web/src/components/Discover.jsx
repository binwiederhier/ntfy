import * as React from "react";
import { useCallback, useContext, useEffect, useState } from "react";
import { Box, Button, Card, CardContent, CircularProgress, Container, IconButton, Stack, Tooltip, Typography } from "@mui/material";
import RefreshIcon from "@mui/icons-material/Refresh";
import { useTranslation } from "react-i18next";
import { useOutletContext } from "react-router-dom";
import accountApi from "../app/AccountApi";
import session from "../app/Session";
import config from "../app/config";
import { subscribeTopic } from "./SubscribeDialog";
import poller from "../app/Poller";
import toast from "../app/Toast";
import AccountContext from "./AccountContext";
import { Paragraph, VerticallyCenteredContainer } from "./styles";
import { unsubscribedSharedTopics } from "../app/utils";

/**
 * Discover view: lists the topics other users on this server have marked as shared
 * (GET /v1/topics?visibility=shared) and lets the user subscribe to them with one click, reusing the
 * normal subscribe flow. The list also refreshes itself when a new shared topic is announced on the
 * ~directory feed (see hooks.js + app/Toast.js).
 */
const Discover = () => {
  const { t } = useTranslation();
  const { account } = useContext(AccountContext);
  const { subscriptions } = useOutletContext();
  const [topics, setTopics] = useState(undefined); // undefined: still loading
  const [error, setError] = useState("");
  const loggedIn = session.exists();

  const load = useCallback(async () => {
    if (!loggedIn) {
      setTopics([]);
      return;
    }
    try {
      setError("");
      setTopics(await accountApi.sharedTopics());
    } catch (e) {
      console.log(`[Discover] Error loading shared topics`, e);
      setError(e.message);
      setTopics([]);
    }
  }, [loggedIn]);

  // Reload on mount and whenever the account changes (e.g. after a visibility change on this account).
  useEffect(() => {
    load(); // Dangle!
  }, [load, account]);

  // Reload when a new shared topic is announced on the directory feed.
  useEffect(() => {
    const listener = () => load();
    toast.registerListener(listener);
    return () => toast.resetListener(listener);
  }, [load]);

  // Discover only ever shows topics the user can still subscribe to. The subscription list is a live
  // Dexie query (see App.jsx), so subscribing here removes the item immediately, no reload required.
  const visibleTopics = unsubscribedSharedTopics(topics, subscriptions, config.base_url);

  const handleSubscribe = async (topic) => {
    console.log(`[Discover] Subscribing to shared topic ${topic}`);
    const subscription = await subscribeTopic(config.base_url, topic, {});
    poller.pollInBackground(subscription); // Dangle!
  };

  if (!loggedIn) {
    return (
      <VerticallyCenteredContainer maxWidth="xs">
        <Typography variant="h5" align="center" sx={{ paddingBottom: 1 }}>
          {t("discover_login_required")}
        </Typography>
      </VerticallyCenteredContainer>
    );
  }

  return (
    <Container maxWidth="md" sx={{ marginTop: 3, marginBottom: 3 }}>
      <Box sx={{ display: "flex", alignItems: "flex-start", justifyContent: "space-between", gap: 2, marginBottom: 2 }}>
        <Box sx={{ minWidth: 0 }}>
          <Typography variant="h5">{t("discover_title")}</Typography>
          <Paragraph>{t("discover_description")}</Paragraph>
        </Box>
        <Tooltip title={t("discover_refresh")}>
          <IconButton onClick={() => load()} aria-label={t("discover_refresh")}>
            <RefreshIcon />
          </IconButton>
        </Tooltip>
      </Box>
      {topics === undefined && (
        <Box sx={{ display: "flex", justifyContent: "center", padding: 4 }}>
          <CircularProgress disableShrink />
        </Box>
      )}
      {topics !== undefined && error !== "" && (
        <Typography color="error">
          {t("discover_error")}: {error}
        </Typography>
      )}
      {topics !== undefined && error === "" && topics.length === 0 && <Typography color="text.secondary">{t("discover_empty")}</Typography>}
      {topics !== undefined && error === "" && topics.length > 0 && visibleTopics.length === 0 && (
        <Typography color="text.secondary">{t("discover_empty_subscribed")}</Typography>
      )}
      <Stack spacing={2}>
        {visibleTopics.map((topic) => (
          <Card key={topic.topic} variant="outlined">
            <CardContent sx={{ display: "flex", alignItems: "center", justifyContent: "space-between", gap: 2 }}>
              <Box sx={{ minWidth: 0 }}>
                <Typography variant="subtitle1" sx={{ wordBreak: "break-all" }}>
                  {topic.topic}
                </Typography>
                <Typography variant="body2" color="text.secondary">
                  {t("discover_owner", { owner: topic.owner })}
                </Typography>
              </Box>
              <Button variant="contained" onClick={() => handleSubscribe(topic.topic)}>
                {t("discover_subscribe")}
              </Button>
            </CardContent>
          </Card>
        ))}
      </Stack>
    </Container>
  );
};

export default Discover;
