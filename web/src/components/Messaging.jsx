import * as React from "react";
import { useState } from "react";
import { Paper, IconButton, TextField, Portal, Snackbar } from "@mui/material";
import SendIcon from "@mui/icons-material/Send";
import KeyboardArrowUpIcon from "@mui/icons-material/KeyboardArrowUp";
import { useTranslation } from "react-i18next";
import PublishDialog from "./PublishDialog";
import api from "../app/Api";
import Navigation from "./Navigation";

const Messaging = (props) => {
  const [message, setMessage] = useState("");
  const [attachFile, setAttachFile] = useState(null);
  const [dialogKey, setDialogKey] = useState(0);

  const { dialogOpenMode } = props;
  const subscription = props.selected;

  const handleOpenDialogClick = () => {
    props.onDialogOpenModeChange(PublishDialog.OPEN_MODE_DEFAULT);
  };

  const handleDialogClose = () => {
    props.onDialogOpenModeChange("");
    setDialogKey((prev) => prev + 1);
    setAttachFile(null);
  };

  return (
    <>
      {subscription && (
        <MessageBar
          subscription={subscription}
          message={message}
          onMessageChange={setMessage}
          onFilePasted={setAttachFile}
          onOpenDialogClick={handleOpenDialogClick}
        />
      )}
      <PublishDialog
        key={`publishDialog${dialogKey}`} // Resets dialog when canceled/closed
        openMode={dialogOpenMode}
        baseUrl={subscription?.baseUrl ?? config.base_url}
        topic={subscription?.topic ?? ""}
        message={message}
        attachFile={attachFile}
        onClose={handleDialogClose}
        onDragEnter={() => props.onDialogOpenModeChange((prev) => prev || PublishDialog.OPEN_MODE_DRAG)} // Only update if not already open
        onResetOpenMode={() => props.onDialogOpenModeChange(PublishDialog.OPEN_MODE_DEFAULT)}
      />
    </>
  );
};

const MessageBar = (props) => {
  const { t } = useTranslation();
  const { subscription } = props;
  const [snackOpen, setSnackOpen] = useState(false);
  const handleSendClick = async () => {
    try {
      await api.publish(subscription.baseUrl, subscription.topic, props.message);
    } catch (e) {
      console.log(`[MessageBar] Error publishing message`, e);
      setSnackOpen(true);
    }
    props.onMessageChange("");
  };

  const handlePaste = (ev) => {
    const clipboardData = ev.clipboardData || window.clipboardData;
    const { items } = clipboardData;
    for (let i = 0; i < items.length; i += 1) {
      if (items[i].type.indexOf("image") !== -1) {
        const blob = items[i].getAsFile();
        props.onFilePasted(blob);
        props.onOpenDialogClick();
        console.log(`[MessageBar] Pasted image`, blob);
        break;
      }
    }
  };

  return (
    <Paper
      elevation={3}
      sx={{
        display: "flex",
        position: "fixed",
        bottom: 0,
        right: 0,
        padding: 2,
        width: { xs: "100%", sm: `calc(100% - ${Navigation.width}px)` },
        backgroundColor: (theme) => (theme.palette.mode === "light" ? theme.palette.grey[100] : theme.palette.grey[900]),
      }}
    >
      <IconButton color="inherit" size="large" edge="start" onClick={props.onOpenDialogClick} aria-label={t("message_bar_show_dialog")}>
        <KeyboardArrowUpIcon />
      </IconButton>
      <TextField
        autoFocus
        margin="dense"
        placeholder={t("message_bar_type_message")}
        aria-label={t("message_bar_type_message")}
        role="textbox"
        type="text"
        fullWidth
        variant="standard"
        value={props.message}
        onChange={(ev) => props.onMessageChange(ev.target.value)}
        onKeyPress={(ev) => {
          if (ev.key === "Enter") {
            ev.preventDefault();
            handleSendClick();
          }
        }}
        onPaste={handlePaste}
      />
      <IconButton color="inherit" size="large" edge="end" onClick={handleSendClick} aria-label={t("message_bar_publish")}>
        <SendIcon />
      </IconButton>
      <Portal>
        <Snackbar
          open={snackOpen}
          autoHideDuration={3000}
          onClose={() => setSnackOpen(false)}
          message={t("message_bar_error_publishing")}
        />
      </Portal>
    </Paper>
  );
};

export default Messaging;
