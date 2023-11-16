import * as React from "react";
import { useState } from "react";
import {
  Button,
  TextField,
  Dialog,
  DialogContent,
  DialogContentText,
  DialogTitle,
  Alert,
  FormControl,
  Select,
  useMediaQuery,
  MenuItem,
  ListItemIcon,
  ListItemText,
  useTheme,
} from "@mui/material";
import { useTranslation } from "react-i18next";
import { Check, DeleteForever } from "@mui/icons-material";
import { validTopic } from "../app/utils";
import DialogFooter from "./DialogFooter";
import session from "../app/Session";
import routes from "./routes";
import accountApi, { Permission } from "../app/AccountApi";
import ReserveTopicSelect from "./ReserveTopicSelect";
import { TopicReservedError, UnauthorizedError } from "../app/errors";

export const ReserveAddDialog = (props) => {
  const theme = useTheme();
  const { t } = useTranslation();
  const [error, setError] = useState("");
  const [topic, setTopic] = useState(props.topic || "");
  const [everyone, setEveryone] = useState(Permission.DENY_ALL);
  const fullScreen = useMediaQuery(theme.breakpoints.down("sm"));
  const allowTopicEdit = !props.topic;
  const alreadyReserved = props.reservations.filter((r) => r.topic === topic).length > 0;
  const submitButtonEnabled = validTopic(topic) && !alreadyReserved;

  const handleSubmit = async () => {
    try {
      await accountApi.upsertReservation(topic, everyone);
      console.debug(`[ReserveAddDialog] Added reservation for topic ${topic}: ${everyone}`);
    } catch (e) {
      console.log(`[ReserveAddDialog] Error adding topic reservation.`, e);
      if (e instanceof UnauthorizedError) {
        await session.resetAndRedirect(routes.login);
      } else if (e instanceof TopicReservedError) {
        setError(t("subscribe_dialog_error_topic_already_reserved"));
        return;
      } else {
        setError(e.message);
        return;
      }
    }
    props.onClose();
  };

  return (
    <Dialog open={props.open} onClose={props.onClose} maxWidth="sm" fullWidth fullScreen={fullScreen}>
      <DialogTitle>{t("prefs_reservations_dialog_title_add")}</DialogTitle>
      <DialogContent>
        <DialogContentText>{t("prefs_reservations_dialog_description")}</DialogContentText>
        {allowTopicEdit && (
          <TextField
            autoFocus
            margin="dense"
            id="topic"
            label={t("prefs_reservations_dialog_topic_label")}
            aria-label={t("prefs_reservations_dialog_topic_label")}
            value={topic}
            onChange={(ev) => setTopic(ev.target.value)}
            type="url"
            fullWidth
            variant="standard"
          />
        )}
        <ReserveTopicSelect value={everyone} onChange={setEveryone} sx={{ mt: 1 }} />
      </DialogContent>
      <DialogFooter status={error}>
        <Button onClick={props.onClose}>{t("common_cancel")}</Button>
        <Button onClick={handleSubmit} disabled={!submitButtonEnabled}>
          {t("common_add")}
        </Button>
      </DialogFooter>
    </Dialog>
  );
};

export const ReserveEditDialog = (props) => {
  const theme = useTheme();
  const { t } = useTranslation();
  const [error, setError] = useState("");
  const [everyone, setEveryone] = useState(props.reservation?.everyone || Permission.DENY_ALL);
  const fullScreen = useMediaQuery(theme.breakpoints.down("sm"));

  const handleSubmit = async () => {
    try {
      await accountApi.upsertReservation(props.reservation.topic, everyone);
      console.debug(`[ReserveEditDialog] Updated reservation for topic ${t}: ${everyone}`);
    } catch (e) {
      console.log(`[ReserveEditDialog] Error updating topic reservation.`, e);
      if (e instanceof UnauthorizedError) {
        await session.resetAndRedirect(routes.login);
      } else {
        setError(e.message);
        return;
      }
    }
    props.onClose();
  };

  return (
    <Dialog open={props.open} onClose={props.onClose} maxWidth="sm" fullWidth fullScreen={fullScreen}>
      <DialogTitle>{t("prefs_reservations_dialog_title_edit")}</DialogTitle>
      <DialogContent>
        <DialogContentText>{t("prefs_reservations_dialog_description")}</DialogContentText>
        <ReserveTopicSelect value={everyone} onChange={setEveryone} sx={{ mt: 1 }} />
      </DialogContent>
      <DialogFooter status={error}>
        <Button onClick={props.onClose}>{t("common_cancel")}</Button>
        <Button onClick={handleSubmit}>{t("common_save")}</Button>
      </DialogFooter>
    </Dialog>
  );
};

export const ReserveDeleteDialog = (props) => {
  const theme = useTheme();
  const { t } = useTranslation();
  const [error, setError] = useState("");
  const [deleteMessages, setDeleteMessages] = useState(false);
  const fullScreen = useMediaQuery(theme.breakpoints.down("sm"));

  const handleSubmit = async () => {
    try {
      await accountApi.deleteReservation(props.topic, deleteMessages);
      console.debug(`[ReserveDeleteDialog] Deleted reservation for topic ${props.topic}`);
    } catch (e) {
      console.log(`[ReserveDeleteDialog] Error deleting topic reservation.`, e);
      if (e instanceof UnauthorizedError) {
        await session.resetAndRedirect(routes.login);
      } else {
        setError(e.message);
        return;
      }
    }
    props.onClose();
  };

  return (
    <Dialog open={props.open} onClose={props.onClose} maxWidth="sm" fullWidth fullScreen={fullScreen}>
      <DialogTitle>{t("prefs_reservations_dialog_title_delete")}</DialogTitle>
      <DialogContent>
        <DialogContentText>{t("reservation_delete_dialog_description")}</DialogContentText>
        <FormControl fullWidth variant="standard">
          <Select
            value={deleteMessages}
            onChange={(ev) => setDeleteMessages(ev.target.value)}
            sx={{
              "& .MuiSelect-select": {
                display: "flex",
                alignItems: "center",
                paddingTop: "4px",
                paddingBottom: "4px",
              },
            }}
          >
            <MenuItem value={false}>
              <ListItemIcon>
                <Check />
              </ListItemIcon>
              <ListItemText primary={t("reservation_delete_dialog_action_keep_title")} />
            </MenuItem>
            <MenuItem value>
              <ListItemIcon>
                <DeleteForever />
              </ListItemIcon>
              <ListItemText primary={t("reservation_delete_dialog_action_delete_title")} />
            </MenuItem>
          </Select>
        </FormControl>
        {!deleteMessages && (
          <Alert severity="info" sx={{ mt: 1 }}>
            {t("reservation_delete_dialog_action_keep_description")}
          </Alert>
        )}
        {deleteMessages && (
          <Alert severity="warning" sx={{ mt: 1 }}>
            {t("reservation_delete_dialog_action_delete_description")}
          </Alert>
        )}
      </DialogContent>
      <DialogFooter status={error}>
        <Button onClick={props.onClose}>{t("common_cancel")}</Button>
        <Button onClick={handleSubmit} color="error">
          {t("reservation_delete_dialog_submit_button")}
        </Button>
      </DialogFooter>
    </Dialog>
  );
};
