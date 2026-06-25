import * as React from "react";
import { Suspense, lazy, useContext, useEffect, useRef, useState } from "react";
import {
  Checkbox,
  Chip,
  FormControl,
  FormControlLabel,
  InputLabel,
  Link,
  Select,
  Tooltip,
  useMediaQuery,
  TextField,
  Dialog,
  DialogTitle,
  DialogContent,
  Button,
  Typography,
  IconButton,
  MenuItem,
  Box,
  useTheme,
} from "@mui/material";
import InsertEmoticonIcon from "@mui/icons-material/InsertEmoticon";
import Close from "@mui/icons-material/Close";
import { Trans, useTranslation } from "react-i18next";
import priority1 from "../img/priority-1.svg";
import priority2 from "../img/priority-2.svg";
import priority3 from "../img/priority-3.svg";
import priority4 from "../img/priority-4.svg";
import priority5 from "../img/priority-5.svg";
import { formatBytes, maybeWithAuth, topicShortUrl, topicUrl, validTopic, validUrl } from "../app/utils";
import { imageRegex } from "../app/notificationUtils";
import AttachmentIcon from "./AttachmentIcon";
import DialogFooter from "./DialogFooter";
import api from "../app/Api";
import userManager from "../app/UserManager";
import session from "../app/Session";
import routes from "./routes";
import accountApi from "../app/AccountApi";
import { UnauthorizedError } from "../app/errors";
import AccountContext from "./AccountContext";

// Loaded lazily so the full emoji dataset (~300 KB) is only fetched when the publish
// dialog is opened, not in the initial app bundle (see EmojiPicker.jsx).
const EmojiPicker = lazy(() => import("./EmojiPicker"));

const PublishDialog = (props) => {
  const theme = useTheme();
  const { t } = useTranslation();
  const { account } = useContext(AccountContext);
  const [baseUrl, setBaseUrl] = useState("");
  const [topic, setTopic] = useState("");
  const [message, setMessage] = useState("");
  const [messageFocused, setMessageFocused] = useState(true);
  const [title, setTitle] = useState("");
  const [tags, setTags] = useState("");
  const [priority, setPriority] = useState(3);
  const [clickUrl, setClickUrl] = useState("");
  const [attachUrl, setAttachUrl] = useState("");
  const [attachFiles, setAttachFiles] = useState([]);
  const [filename, setFilename] = useState("");
  const [filenameEdited, setFilenameEdited] = useState(false);
  const [email, setEmail] = useState("");
  const [call, setCall] = useState("");
  const [delay, setDelay] = useState("");
  const [publishAnother, setPublishAnother] = useState(false);
  const [markdownEnabled, setMarkdownEnabled] = useState(false);

  const [showTopicUrl, setShowTopicUrl] = useState("");
  const [showClickUrl, setShowClickUrl] = useState(false);
  const [showAttachUrl, setShowAttachUrl] = useState(false);
  const [showEmail, setShowEmail] = useState(false);
  const [showCall, setShowCall] = useState(false);
  const [showDelay, setShowDelay] = useState(false);

  const showAttachFile = attachFiles.length > 0 && !showAttachUrl;
  const attachFileInput = useRef();
  const [attachFileError, setAttachFileError] = useState("");

  const [activeRequest, setActiveRequest] = useState(null);
  const [status, setStatus] = useState("");
  const disabled = !!activeRequest;

  const [emojiPickerAnchorEl, setEmojiPickerAnchorEl] = useState(null);

  const [dropZone, setDropZone] = useState(false);
  const [sendButtonEnabled, setSendButtonEnabled] = useState(true);

  const open = !!props.openMode;
  const fullScreen = useMediaQuery(theme.breakpoints.down("sm"));

  useEffect(() => {
    window.addEventListener("dragenter", () => {
      props.onDragEnter();
      setDropZone(true);
    });
  }, []);

  useEffect(() => {
    setBaseUrl(props.baseUrl);
    setTopic(props.topic);
    setShowTopicUrl(!props.baseUrl || !props.topic);
    setMessageFocused(!!props.topic); // Focus message only if topic is set
  }, [props.baseUrl, props.topic]);

  useEffect(() => {
    const valid = validUrl(baseUrl) && validTopic(topic) && !attachFileError;
    setSendButtonEnabled(valid);
  }, [baseUrl, topic, attachFileError]);

  useEffect(() => {
    setMessage(props.message);
  }, [props.message]);

  const updateBaseUrl = (newVal) => {
    if (validUrl(newVal)) {
      setBaseUrl(newVal.replace(/\/$/, "")); // strip traililng slash after https?://
    } else {
      setBaseUrl(newVal);
    }
  };

  const handleSubmit = async () => {
    const url = new URL(topicUrl(baseUrl, topic));
    if (title.trim()) {
      url.searchParams.append("title", title.trim());
    }
    if (tags.trim()) {
      url.searchParams.append("tags", tags.trim());
    }
    if (priority && priority !== 3) {
      url.searchParams.append("priority", priority.toString());
    }
    if (clickUrl.trim()) {
      url.searchParams.append("click", clickUrl.trim());
    }
    if (attachUrl.trim()) {
      url.searchParams.append("attach", attachUrl.trim());
    }
    if (filename.trim()) {
      url.searchParams.append("filename", filename.trim());
    }
    if (email.trim()) {
      url.searchParams.append("email", email.trim());
    }
    if (call.trim()) {
      url.searchParams.append("call", call.trim());
    }
    if (delay.trim()) {
      url.searchParams.append("delay", delay.trim());
    }
    if (markdownEnabled) {
      url.searchParams.append("markdown", "true");
    }

    const body = attachFiles.length > 0 ? multipartBody(message, attachFiles) : message;
    try {
      const user = await userManager.get(baseUrl);
      const headers = maybeWithAuth({}, user);
      const progressFn = (ev) => {
        if (ev.loaded > 0 && ev.total > 0) {
          setStatus(
            t("publish_dialog_progress_uploading_detail", {
              loaded: formatBytes(ev.loaded),
              total: formatBytes(ev.total),
              percent: Math.round((ev.loaded * 100.0) / ev.total),
            })
          );
        } else {
          setStatus(t("publish_dialog_progress_uploading"));
        }
      };
      const request = api.publishXHR(url, body, headers, progressFn);
      setActiveRequest(request);
      await request;
      if (!publishAnother) {
        props.onClose();
      } else {
        setStatus(t("publish_dialog_message_published"));
        setActiveRequest(null);
      }
    } catch (e) {
      setStatus(<Typography sx={{ color: "error.main", maxWidth: "400px" }}>{e}</Typography>);
      setActiveRequest(null);
    }
  };

  const checkAttachmentLimits = async (files) => {
    try {
      const apiAccount = await accountApi.get();
      const fileSizeLimit = apiAccount.limits.attachment_file_size ?? 0;
      const remainingBytes = apiAccount.stats.attachment_total_size_remaining;
      const fileSizeLimitReached = fileSizeLimit > 0 && files.some((file) => file.size > fileSizeLimit);
      const totalSize = files.reduce((sum, file) => sum + file.size, 0);
      const quotaReached = remainingBytes > 0 && totalSize > remainingBytes;
      if (fileSizeLimitReached && quotaReached) {
        setAttachFileError(
          t("publish_dialog_attachment_limits_file_and_quota_reached", {
            fileSizeLimit: formatBytes(fileSizeLimit),
            remainingBytes: formatBytes(remainingBytes),
          })
        );
      } else if (fileSizeLimitReached) {
        setAttachFileError(
          t("publish_dialog_attachment_limits_file_reached", {
            fileSizeLimit: formatBytes(fileSizeLimit),
          })
        );
      } else if (quotaReached) {
        setAttachFileError(
          t("publish_dialog_attachment_limits_quota_reached", {
            remainingBytes: formatBytes(remainingBytes),
          })
        );
      } else {
        setAttachFileError("");
      }
    } catch (e) {
      console.log(`[PublishDialog] Retrieving attachment limits failed`, e);
      if (e instanceof UnauthorizedError) {
        await session.resetAndRedirect(routes.login);
      } else {
        setAttachFileError(""); // Reset error (rely on server-side checking)
      }
    }
  };

  const handleAttachFileClick = () => {
    attachFileInput.current.click();
  };

  const updateAttachFiles = async (files, append = false) => {
    const attachments = files.map((file) => ({
      file,
      filename: file.name || "attachment",
    }));
    const next = append ? [...attachFiles, ...attachments] : attachments;
    setAttachFiles(next);
    props.onResetOpenMode();
    await checkAttachmentLimits(next.map(({ file }) => file));
  };

  useEffect(() => {
    if (props.attachFile) {
      updateAttachFiles([props.attachFile]);
    }
  }, [props.attachFile]);

  const handlePaste = (ev) => {
    const blob = props.getPastedImage(ev);
    if (blob) {
      updateAttachFiles([blob], true);
    }
  };

  const handleAttachFileChanged = async (ev) => {
    await updateAttachFiles(Array.from(ev.target.files));
    ev.target.value = "";
  };

  const handleAttachFileDrop = async (ev) => {
    ev.preventDefault();
    setDropZone(false);
    await updateAttachFiles(Array.from(ev.dataTransfer.files));
  };

  const handleAttachFileRemove = async (index) => {
    const next = attachFiles.filter((_, i) => i !== index);
    setAttachFiles(next);
    if (next.length === 0) {
      setAttachFileError("");
    } else {
      await checkAttachmentLimits(next.map(({ file }) => file));
    }
  };

  const handleAttachFilenameChange = (index, newFilename) => {
    setAttachFiles((prev) => prev.map((attachment, i) => (i === index ? { ...attachment, filename: newFilename } : attachment)));
  };

  const handleAttachFileDragLeave = () => {
    setDropZone(false);
    if (props.openMode === PublishDialog.OPEN_MODE_DRAG) {
      props.onClose(); // Only close dialog if it was not open before dragging file in
    }
  };

  const handleEmojiClick = (ev) => {
    setEmojiPickerAnchorEl(ev.currentTarget);
  };

  const handleEmojiPick = (emoji) => {
    setTags((prevTags) => (prevTags.trim() ? `${prevTags.trim()}, ${emoji}` : emoji));
  };

  const handleEmojiClose = () => {
    setEmojiPickerAnchorEl(null);
  };

  const priorities = {
    1: { label: t("publish_dialog_priority_min"), file: priority1 },
    2: { label: t("publish_dialog_priority_low"), file: priority2 },
    3: { label: t("publish_dialog_priority_default"), file: priority3 },
    4: { label: t("publish_dialog_priority_high"), file: priority4 },
    5: { label: t("publish_dialog_priority_max"), file: priority5 },
  };

  return (
    <>
      {dropZone && <DropArea onDrop={handleAttachFileDrop} onDragLeave={handleAttachFileDragLeave} />}
      <Dialog maxWidth="md" open={open} onClose={props.onCancel} fullScreen={fullScreen}>
        <DialogTitle>
          {baseUrl && topic
            ? t("publish_dialog_title_topic", {
                topic: topicShortUrl(baseUrl, topic),
              })
            : t("publish_dialog_title_no_topic")}
        </DialogTitle>
        <DialogContent>
          {dropZone && <DropBox />}
          {showTopicUrl && (
            <ClosableRow
              closable={!!props.baseUrl && !!props.topic}
              disabled={disabled}
              closeLabel={t("publish_dialog_topic_reset")}
              onClose={() => {
                setBaseUrl(props.baseUrl);
                setTopic(props.topic);
                setShowTopicUrl(false);
              }}
            >
              <TextField
                margin="dense"
                label={t("publish_dialog_base_url_label")}
                placeholder={t("publish_dialog_base_url_placeholder")}
                value={baseUrl}
                onChange={(ev) => updateBaseUrl(ev.target.value)}
                disabled={disabled}
                type="url"
                variant="standard"
                sx={{ flexGrow: 1, marginRight: 1 }}
                slotProps={{
                  htmlInput: {
                    "aria-label": t("publish_dialog_base_url_label"),
                  },
                }}
              />
              <TextField
                margin="dense"
                label={t("publish_dialog_topic_label")}
                placeholder={t("publish_dialog_topic_placeholder")}
                value={topic}
                onChange={(ev) => setTopic(ev.target.value)}
                disabled={disabled}
                type="text"
                variant="standard"
                autoFocus={!messageFocused}
                sx={{ flexGrow: 1 }}
                slotProps={{
                  htmlInput: {
                    "aria-label": t("publish_dialog_topic_label"),
                  },
                }}
              />
            </ClosableRow>
          )}
          <TextField
            margin="dense"
            label={t("publish_dialog_title_label")}
            placeholder={t("publish_dialog_title_placeholder")}
            value={title}
            onChange={(ev) => setTitle(ev.target.value)}
            disabled={disabled}
            type="text"
            fullWidth
            variant="standard"
            slotProps={{
              htmlInput: {
                "aria-label": t("publish_dialog_title_label"),
              },
            }}
          />
          <TextField
            margin="dense"
            label={t("publish_dialog_message_label")}
            placeholder={t("publish_dialog_message_placeholder")}
            value={message}
            onChange={(ev) => setMessage(ev.target.value)}
            disabled={disabled}
            type="text"
            variant="standard"
            rows={5}
            autoFocus={messageFocused}
            fullWidth
            multiline
            slotProps={{
              htmlInput: {
                "aria-label": t("publish_dialog_message_label"),
              },
            }}
            onPaste={handlePaste}
          />
          <FormControlLabel
            label={t("publish_dialog_checkbox_markdown")}
            sx={{ marginRight: 2 }}
            control={
              <Checkbox
                size="small"
                checked={markdownEnabled}
                onChange={(ev) => setMarkdownEnabled(ev.target.checked)}
                slotProps={{
                  input: {
                    "aria-label": t("publish_dialog_checkbox_markdown"),
                  },
                }}
              />
            }
          />
          <div style={{ display: "flex" }}>
            <Suspense fallback={null}>
              <EmojiPicker anchorEl={emojiPickerAnchorEl} onEmojiPick={handleEmojiPick} onClose={handleEmojiClose} />
            </Suspense>
            <DialogIconButton disabled={disabled} onClick={handleEmojiClick} aria-label={t("publish_dialog_emoji_picker_show")}>
              <InsertEmoticonIcon />
            </DialogIconButton>
            <TextField
              margin="dense"
              label={t("publish_dialog_tags_label")}
              placeholder={t("publish_dialog_tags_placeholder")}
              value={tags}
              onChange={(ev) => setTags(ev.target.value)}
              disabled={disabled}
              type="text"
              variant="standard"
              sx={{ flexGrow: 1, marginRight: 1 }}
              slotProps={{
                htmlInput: {
                  "aria-label": t("publish_dialog_tags_label"),
                },
              }}
            />
            <FormControl variant="standard" margin="dense" sx={{ minWidth: 170, maxWidth: 300, flexGrow: 1 }}>
              <InputLabel />
              <Select
                label={t("publish_dialog_priority_label")}
                margin="dense"
                value={priority}
                onChange={(ev) => setPriority(ev.target.value)}
                disabled={disabled}
                inputProps={{
                  "aria-label": t("publish_dialog_priority_label"),
                }}
              >
                {[5, 4, 3, 2, 1].map((p) => (
                  <MenuItem
                    key={`priorityMenuItem${p}`}
                    value={p}
                    aria-label={t("notifications_priority_x", {
                      priority: p,
                    })}
                  >
                    <div style={{ display: "flex", alignItems: "center" }}>
                      <img
                        src={priorities[p].file}
                        style={{ marginRight: "8px" }}
                        alt={t("notifications_priority_x", {
                          priority: p,
                        })}
                      />
                      <div>{priorities[p].label}</div>
                    </div>
                  </MenuItem>
                ))}
              </Select>
            </FormControl>
          </div>
          {showClickUrl && (
            <ClosableRow
              disabled={disabled}
              closeLabel={t("publish_dialog_click_reset")}
              onClose={() => {
                setClickUrl("");
                setShowClickUrl(false);
              }}
            >
              <TextField
                margin="dense"
                label={t("publish_dialog_click_label")}
                placeholder={t("publish_dialog_click_placeholder")}
                value={clickUrl}
                onChange={(ev) => setClickUrl(ev.target.value)}
                disabled={disabled}
                type="url"
                fullWidth
                variant="standard"
                slotProps={{
                  htmlInput: {
                    "aria-label": t("publish_dialog_click_label"),
                  },
                }}
              />
            </ClosableRow>
          )}
          {showEmail && (
            <ClosableRow
              disabled={disabled}
              closeLabel={t("publish_dialog_email_reset")}
              onClose={() => {
                setEmail("");
                setShowEmail(false);
              }}
            >
              <TextField
                margin="dense"
                label={t("publish_dialog_email_label")}
                placeholder={t("publish_dialog_email_placeholder")}
                value={email}
                onChange={(ev) => setEmail(ev.target.value)}
                disabled={disabled}
                type="email"
                variant="standard"
                fullWidth
                slotProps={{
                  htmlInput: {
                    "aria-label": t("publish_dialog_email_label"),
                  },
                }}
              />
            </ClosableRow>
          )}
          {showCall && (
            <ClosableRow
              disabled={disabled}
              closeLabel={t("publish_dialog_call_reset")}
              onClose={() => {
                setCall("");
                setShowCall(false);
              }}
            >
              <FormControl fullWidth variant="standard" margin="dense">
                <InputLabel />
                <Select
                  label={t("publish_dialog_call_label")}
                  margin="dense"
                  value={call}
                  onChange={(ev) => setCall(ev.target.value)}
                  disabled={disabled}
                  inputProps={{
                    "aria-label": t("publish_dialog_call_label"),
                  }}
                >
                  {account?.phone_numbers?.map((phoneNumber) => (
                    <MenuItem key={phoneNumber} value={phoneNumber} aria-label={phoneNumber}>
                      {t("publish_dialog_call_item", { number: phoneNumber })}
                    </MenuItem>
                  ))}
                </Select>
              </FormControl>
            </ClosableRow>
          )}
          {showAttachUrl && (
            <ClosableRow
              disabled={disabled}
              closeLabel={t("publish_dialog_attach_reset")}
              onClose={() => {
                setAttachUrl("");
                setFilename("");
                setFilenameEdited(false);
                setShowAttachUrl(false);
              }}
            >
              <TextField
                margin="dense"
                label={t("publish_dialog_attach_label")}
                placeholder={t("publish_dialog_attach_placeholder")}
                value={attachUrl}
                onChange={(ev) => {
                  const url = ev.target.value;
                  setAttachUrl(url);
                  if (!filenameEdited) {
                    try {
                      const u = new URL(url);
                      const parts = u.pathname.split("/");
                      if (parts.length > 0) {
                        setFilename(parts[parts.length - 1]);
                      }
                    } catch (e) {
                      // Do nothing
                    }
                  }
                }}
                disabled={disabled}
                type="url"
                variant="standard"
                sx={{ flexGrow: 5, marginRight: 1 }}
                slotProps={{
                  htmlInput: {
                    "aria-label": t("publish_dialog_attach_label"),
                  },
                }}
              />
              <TextField
                margin="dense"
                label={t("publish_dialog_filename_label")}
                placeholder={t("publish_dialog_filename_placeholder")}
                value={filename}
                onChange={(ev) => {
                  setFilename(ev.target.value);
                  setFilenameEdited(true);
                }}
                disabled={disabled}
                type="text"
                variant="standard"
                sx={{ flexGrow: 1 }}
                slotProps={{
                  htmlInput: {
                    "aria-label": t("publish_dialog_filename_label"),
                  },
                }}
              />
            </ClosableRow>
          )}
          <input type="file" ref={attachFileInput} onChange={handleAttachFileChanged} style={{ display: "none" }} aria-hidden multiple />
          {showAttachFile && (
            <AttachmentBox
              attachments={attachFiles}
              disabled={disabled}
              error={attachFileError}
              onChangeFilename={handleAttachFilenameChange}
              onRemove={handleAttachFileRemove}
            />
          )}
          {showDelay && (
            <ClosableRow
              disabled={disabled}
              closeLabel={t("publish_dialog_delay_reset")}
              onClose={() => {
                setDelay("");
                setShowDelay(false);
              }}
            >
              <TextField
                margin="dense"
                label={t("publish_dialog_delay_label")}
                placeholder={t("publish_dialog_delay_placeholder", {
                  unixTimestamp: "1649029748",
                  relativeTime: "30m",
                  naturalLanguage: "tomorrow, 9am",
                })}
                value={delay}
                onChange={(ev) => setDelay(ev.target.value)}
                disabled={disabled}
                type="text"
                variant="standard"
                fullWidth
                slotProps={{
                  htmlInput: {
                    "aria-label": t("publish_dialog_delay_label"),
                  },
                }}
              />
            </ClosableRow>
          )}
          <Typography variant="body1" sx={{ marginTop: 2, marginBottom: 1 }}>
            {t("publish_dialog_other_features")}
          </Typography>
          <div>
            {!showClickUrl && (
              <Chip
                clickable
                disabled={disabled}
                label={t("publish_dialog_chip_click_label")}
                aria-label={t("publish_dialog_chip_click_label")}
                onClick={() => setShowClickUrl(true)}
                sx={{ marginRight: 1, marginBottom: 1 }}
              />
            )}
            {!showEmail && (
              <Chip
                clickable
                disabled={disabled}
                label={t("publish_dialog_chip_email_label")}
                aria-label={t("publish_dialog_chip_email_label")}
                onClick={() => setShowEmail(true)}
                sx={{ marginRight: 1, marginBottom: 1 }}
              />
            )}
            {account?.phone_numbers?.length > 0 && !showCall && (
              <Chip
                clickable
                disabled={disabled}
                label={t("publish_dialog_chip_call_label")}
                aria-label={t("publish_dialog_chip_call_label")}
                onClick={() => {
                  setShowCall(true);
                  setCall(account.phone_numbers[0]);
                }}
                sx={{ marginRight: 1, marginBottom: 1 }}
              />
            )}
            {!showAttachUrl && !showAttachFile && (
              <Chip
                clickable
                disabled={disabled}
                label={t("publish_dialog_chip_attach_url_label")}
                aria-label={t("publish_dialog_chip_attach_url_label")}
                onClick={() => setShowAttachUrl(true)}
                sx={{ marginRight: 1, marginBottom: 1 }}
              />
            )}
            {!showAttachFile && !showAttachUrl && (
              <Chip
                clickable
                disabled={disabled}
                label={t("publish_dialog_chip_attach_file_label")}
                aria-label={t("publish_dialog_chip_attach_file_label")}
                onClick={() => handleAttachFileClick()}
                sx={{ marginRight: 1, marginBottom: 1 }}
              />
            )}
            {!showDelay && (
              <Chip
                clickable
                disabled={disabled}
                label={t("publish_dialog_chip_delay_label")}
                aria-label={t("publish_dialog_chip_delay_label")}
                onClick={() => setShowDelay(true)}
                sx={{ marginRight: 1, marginBottom: 1 }}
              />
            )}
            {!showTopicUrl && (
              <Chip
                clickable
                disabled={disabled}
                label={t("publish_dialog_chip_topic_label")}
                aria-label={t("publish_dialog_chip_topic_label")}
                onClick={() => setShowTopicUrl(true)}
                sx={{ marginRight: 1, marginBottom: 1 }}
              />
            )}
            {account && !account?.phone_numbers && (
              <Tooltip title={t("publish_dialog_chip_call_no_verified_numbers_tooltip")}>
                <span>
                  <Chip
                    clickable
                    disabled
                    label={t("publish_dialog_chip_call_label")}
                    aria-label={t("publish_dialog_chip_call_label")}
                    sx={{ marginRight: 1, marginBottom: 1 }}
                  />
                </span>
              </Tooltip>
            )}
          </div>
          <Typography variant="body1" sx={{ marginTop: 1, marginBottom: 1 }}>
            <Trans
              i18nKey="publish_dialog_details_examples_description"
              components={{
                docsLink: <Link href="https://ntfy.sh/docs" target="_blank" rel="noopener" />,
              }}
            />
          </Typography>
        </DialogContent>
        <DialogFooter status={status}>
          {activeRequest && <Button onClick={() => activeRequest.abort()}>{t("publish_dialog_button_cancel_sending")}</Button>}
          {!activeRequest && (
            <>
              <FormControlLabel
                label={t("publish_dialog_checkbox_publish_another")}
                sx={{ marginRight: 2 }}
                control={
                  <Checkbox
                    size="small"
                    checked={publishAnother}
                    onChange={(ev) => setPublishAnother(ev.target.checked)}
                    slotProps={{
                      input: {
                        "aria-label": t("publish_dialog_checkbox_publish_another"),
                      },
                    }}
                  />
                }
              />
              <Button onClick={props.onClose}>{t("publish_dialog_button_cancel")}</Button>
              <Button onClick={handleSubmit} disabled={!sendButtonEnabled}>
                {t("publish_dialog_button_send")}
              </Button>
            </>
          )}
        </DialogFooter>
      </Dialog>
    </>
  );
};

const Row = (props) => (
  <div style={{ display: "flex" }} role="row">
    {props.children}
  </div>
);

const ClosableRow = (props) => {
  const closable = props.closable !== undefined ? props.closable : true;
  return (
    <Row>
      {props.children}
      {closable && (
        <DialogIconButton disabled={props.disabled} onClick={props.onClose} sx={{ marginLeft: "6px" }} aria-label={props.closeLabel}>
          <Close />
        </DialogIconButton>
      )}
    </Row>
  );
};

const DialogIconButton = (props) => {
  const sx = props.sx || {};
  return (
    <IconButton
      color="inherit"
      size="large"
      edge="start"
      sx={{ height: "45px", marginTop: "17px", ...sx }}
      onClick={props.onClick}
      disabled={props.disabled}
      aria-label={props["aria-label"]}
    >
      {props.children}
    </IconButton>
  );
};

const multipartBody = (message, attachFiles) => {
  const body = new FormData();
  if (message.trim()) {
    body.append("message", message.trim());
  }
  attachFiles.forEach(({ file, filename }) => {
    body.append("attachment", file, filename || file.name || "attachment");
  });
  return body;
};

const AttachmentBox = (props) => {
  const { t } = useTranslation();
  return (
    <>
      <Typography variant="body1" sx={{ marginTop: 2 }}>
        {t("publish_dialog_attached_file_title")}
      </Typography>
      {props.attachments.map(({ file, filename }, index) => (
        <Box
          key={`${file.name || "attachment"}${file.size}${index}`}
          sx={{
            display: "flex",
            alignItems: "center",
            padding: 0.5,
            borderRadius: "4px",
          }}
        >
          <AttachmentIcon type={file.type} href={imageRegex.test(file.name || "") ? URL.createObjectURL(file) : undefined} />
          <Box sx={{ marginLeft: 1, textAlign: "left", minWidth: 0 }}>
            <ExpandingTextField
              minWidth={140}
              variant="body2"
              placeholder={t("publish_dialog_attached_file_filename_placeholder")}
              value={filename}
              onChange={(ev) => props.onChangeFilename(index, ev.target.value)}
              disabled={props.disabled}
            />
            <br />
            <Typography variant="body2" sx={{ color: "text.primary" }}>
              {formatBytes(file.size)}
              {props.error && index === 0 && (
                <Typography component="span" sx={{ color: "error.main" }} aria-live="polite">
                  {" "}
                  ({props.error})
                </Typography>
              )}
            </Typography>
          </Box>
          <DialogIconButton
            disabled={props.disabled}
            onClick={() => props.onRemove(index)}
            sx={{ marginLeft: "6px" }}
            aria-label={t("publish_dialog_attached_file_remove")}
          >
            <Close />
          </DialogIconButton>
        </Box>
      ))}
    </>
  );
};

const ExpandingTextField = (props) => {
  const theme = useTheme();
  const invisibleFieldRef = useRef();
  const [textWidth, setTextWidth] = useState(props.minWidth);
  const determineTextWidth = () => {
    const boundingRect = invisibleFieldRef?.current?.getBoundingClientRect();
    if (!boundingRect) {
      return props.minWidth;
    }
    return boundingRect.width >= props.minWidth ? Math.round(boundingRect.width) : props.minWidth;
  };
  useEffect(() => {
    setTextWidth(determineTextWidth() + 5);
  }, [props.value]);
  return (
    <>
      <Typography ref={invisibleFieldRef} component="span" variant={props.variant} aria-hidden sx={{ position: "absolute", left: "-200%" }}>
        {props.value}
      </Typography>
      <TextField
        margin="dense"
        placeholder={props.placeholder}
        value={props.value}
        onChange={props.onChange}
        type="text"
        variant="standard"
        sx={{ width: `${textWidth}px`, borderBottom: "none" }}
        slotProps={{
          input: {
            style: { fontSize: theme.typography[props.variant].fontSize },
          },
          htmlInput: {
            style: { paddingBottom: 0, paddingTop: 0 },
            "aria-label": props.placeholder,
          },
        }}
        disabled={props.disabled}
      />
    </>
  );
};

const DropArea = (props) => {
  const allowDrag = (ev) => {
    // This is where we could disallow certain files to be dragged in.
    // For now we allow all files.

    // eslint-disable-next-line no-param-reassign
    ev.dataTransfer.dropEffect = "copy";
    ev.preventDefault();
  };

  return (
    <Box
      sx={{
        position: "absolute",
        left: 0,
        top: 0,
        right: 0,
        bottom: 0,
        zIndex: 10002,
      }}
      onDrop={props.onDrop}
      onDragEnter={allowDrag}
      onDragOver={allowDrag}
      onDragLeave={props.onDragLeave}
    />
  );
};

const DropBox = () => {
  const { t } = useTranslation();
  return (
    <Box
      sx={{
        position: "absolute",
        left: 0,
        top: 0,
        right: 0,
        bottom: 0,
        zIndex: 10000,
        backgroundColor: "#ffffffbb",
      }}
    >
      <Box
        sx={{
          position: "absolute",
          border: "3px dashed #ccc",
          borderRadius: "5px",
          left: "40px",
          top: "40px",
          right: "40px",
          bottom: "40px",
          zIndex: 10001,
          display: "flex",
          justifyContent: "center",
          alignItems: "center",
        }}
      >
        <Typography variant="h5">{t("publish_dialog_drop_file_here")}</Typography>
      </Box>
    </Box>
  );
};

PublishDialog.OPEN_MODE_DEFAULT = "default";
PublishDialog.OPEN_MODE_DRAG = "drag";

export default PublishDialog;
