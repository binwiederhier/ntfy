import * as React from "react";
import { createContext, useContext, useEffect, useMemo, useState } from "react";
import {
  Alert,
  Box,
  Button,
  Card,
  Chip,
  CircularProgress,
  Container,
  Dialog,
  DialogContent,
  DialogContentText,
  DialogTitle,
  FormControl,
  IconButton,
  InputAdornment,
  InputLabel,
  MenuItem,
  Select,
  Stack,
  Table,
  TableBody,
  TableCell,
  TableHead,
  TablePagination,
  TableRow,
  TextField,
  Tooltip,
  Typography,
  useMediaQuery,
  useTheme,
} from "@mui/material";
import { useTranslation } from "react-i18next";
import EditIcon from "@mui/icons-material/Edit";
import DeleteOutlineIcon from "@mui/icons-material/DeleteOutlineOutlined";
import AddIcon from "@mui/icons-material/Add";
import LockIcon from "@mui/icons-material/Lock";
import RefreshIcon from "@mui/icons-material/Refresh";
import SearchIcon from "@mui/icons-material/Search";
import PersonAddIcon from "@mui/icons-material/PersonAddAlt1";
import routes from "./routes";
import AccountContext from "./AccountContext";
import DialogFooter from "./DialogFooter";
import { Paragraph } from "./styles";
import { PermissionDenyAll, PermissionRead, PermissionReadWrite, PermissionWrite } from "./ReserveIcons";
import { UnauthorizedError } from "../app/errors";
import session from "../app/Session";
import adminApi, { TIER_RESET } from "../app/AdminApi";
import { Role } from "../app/AccountApi";
import { formatBytes } from "../app/utils";

// Username of the anonymous "Everyone" user, see user.Everyone in user/types.go
const EVERYONE = "*";

const ROWS_PER_PAGE_OPTIONS = [10, 25, 50, 100];
const SEARCH_DEBOUNCE_MILLIS = 300;

const PERMISSIONS = ["read-write", "read-only", "write-only", "deny-all"];

const BYTES_PER_MB = 1024 * 1024;
const SECONDS_PER_HOUR = 3600;

// AdminRefreshContext lets every card reload after a change made in any card (e.g. removing a
// reservation in the users card also changes the access control list and the reservations card)
const AdminRefreshContext = createContext({ version: 0, refresh: () => {} });

const Admin = () => {
  const { account } = useContext(AccountContext);
  const [version, setVersion] = useState(0);
  const refreshContext = useMemo(() => ({ version, refresh: () => setVersion((prev) => prev + 1) }), [version]);

  if (!session.exists() || (account && account.role !== Role.ADMIN)) {
    window.location.href = routes.app;
    return <></>;
  }
  if (!account) {
    return (
      <Box sx={{ display: "flex", justifyContent: "center", p: 6 }}>
        <CircularProgress />
      </Box>
    );
  }
  return (
    <AdminRefreshContext.Provider value={refreshContext}>
      <Container maxWidth="lg" sx={{ marginTop: 3, marginBottom: 3 }}>
        <Stack spacing={3}>
          <Users />
          <AccessControlList />
          <Reservations />
          <Tiers />
        </Stack>
      </Container>
    </AdminRefreshContext.Provider>
  );
};

const AdminCard = (props) => (
  <Card sx={{ p: 3 }} aria-label={props.title}>
    <Typography variant="h5" sx={{ marginBottom: 2 }}>
      {props.title}
    </Typography>
    <Paragraph>{props.description}</Paragraph>
    {props.children}
  </Card>
);

// handleError redirects to the login page if the session is no longer valid, and otherwise passes
// the error message to setError
const handleError = async (e, setError) => {
  console.log(`[Admin] Error`, e);
  if (e instanceof UnauthorizedError) {
    await session.resetAndRedirect(routes.login);
  } else {
    setError(e.message);
  }
};

// Stable page fetchers for usePaginatedList (defined here so they don't change between renders)
const fetchUsersPage = (pagination) => adminApi.getUsers(pagination);
const fetchAccessPage = (pagination) => adminApi.getAccess(pagination);
const fetchReservationsPage = (pagination) => adminApi.getReservations(pagination);

// usePaginatedList loads one page of a server-side paginated list, and reloads it whenever the
// (debounced) search, the page or the page size changes
const usePaginatedList = (fetchPage) => {
  const { version } = useContext(AdminRefreshContext);
  const [search, setSearch] = useState("");
  const [debouncedSearch, setDebouncedSearch] = useState("");
  const [page, setPage] = useState(0);
  const [rowsPerPage, setRowsPerPage] = useState(ROWS_PER_PAGE_OPTIONS[0]);
  const [rows, setRows] = useState([]);
  const [total, setTotal] = useState(0);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [reloadCounter, setReloadCounter] = useState(0);

  // Only reset to the first page if the search actually changed (not on mount)
  useEffect(() => {
    const trimmed = search.trim();
    if (trimmed === debouncedSearch) {
      return undefined;
    }
    const timeout = setTimeout(() => {
      setDebouncedSearch(trimmed);
      setPage(0);
    }, SEARCH_DEBOUNCE_MILLIS);
    return () => clearTimeout(timeout);
  }, [search, debouncedSearch]);

  useEffect(() => {
    let canceled = false;
    const load = async () => {
      setLoading(true);
      try {
        const result = await fetchPage({ search: debouncedSearch, limit: rowsPerPage, offset: page * rowsPerPage });
        if (!canceled) {
          setRows(result.rows);
          setTotal(result.total);
          setError("");
        }
      } catch (e) {
        if (!canceled) {
          await handleError(e, setError);
        }
      } finally {
        if (!canceled) {
          setLoading(false);
        }
      }
    };
    load();
    return () => {
      canceled = true;
    };
  }, [fetchPage, debouncedSearch, page, rowsPerPage, reloadCounter, version]);

  return {
    search,
    setSearch,
    page,
    setPage,
    rowsPerPage,
    setRowsPerPage: (value) => {
      setRowsPerPage(value);
      setPage(0);
    },
    rows,
    total,
    loading,
    error,
    reload: () => setReloadCounter((prev) => prev + 1),
  };
};

// useTiers loads the list of tiers once (for tier select boxes)
const useTiers = () => {
  const { version } = useContext(AdminRefreshContext);
  const [tiers, setTiers] = useState([]);
  useEffect(() => {
    (async () => {
      try {
        setTiers(await adminApi.getTiers());
      } catch (e) {
        console.log(`[Admin] Error loading tiers`, e);
      }
    })();
  }, [version]);
  return tiers;
};

const ListToolbar = (props) => {
  const { t } = useTranslation();
  return (
    <Stack direction={{ xs: "column", sm: "row" }} spacing={1} alignItems={{ sm: "center" }} sx={{ mb: 1 }}>
      <TextField
        size="small"
        placeholder={props.searchPlaceholder}
        value={props.list.search}
        onChange={(ev) => props.list.setSearch(ev.target.value)}
        sx={{ flexGrow: 1 }}
        slotProps={{
          input: {
            startAdornment: (
              <InputAdornment position="start">
                <SearchIcon fontSize="small" />
              </InputAdornment>
            ),
          },
        }}
      />
      <Stack direction="row" spacing={1}>
        <Tooltip title={t("common_refresh")}>
          <IconButton onClick={props.list.reload}>
            <RefreshIcon />
          </IconButton>
        </Tooltip>
        {props.actions}
      </Stack>
    </Stack>
  );
};

const ListBody = (props) => {
  const { t } = useTranslation();
  const { list } = props;
  return (
    <>
      {list.error && (
        <Alert severity="error" sx={{ mb: 1 }}>
          {list.error}
        </Alert>
      )}
      <Box sx={{ width: "100%", overflowX: "auto", position: "relative", opacity: list.loading ? 0.5 : 1 }}>{props.children}</Box>
      {!list.loading && list.rows.length === 0 && (
        <Typography variant="body2" color="text.secondary" sx={{ p: 2, textAlign: "center" }}>
          {t("admin_list_empty")}
        </Typography>
      )}
      <TablePagination
        component="div"
        count={list.total}
        page={list.total > 0 ? list.page : 0}
        onPageChange={(ev, page) => list.setPage(page)}
        rowsPerPage={list.rowsPerPage}
        rowsPerPageOptions={ROWS_PER_PAGE_OPTIONS}
        onRowsPerPageChange={(ev) => list.setRowsPerPage(parseInt(ev.target.value, 10))}
      />
    </>
  );
};

// ActionButton is a small icon button with a tooltip. When disabled, the tooltip explains why.
const ActionButton = (props) => (
  <Tooltip title={props.disabled && props.disabledReason ? props.disabledReason : props.label}>
    <span>
      <IconButton size="small" aria-label={props.label} disabled={props.disabled} onClick={props.onClick}>
        {props.children}
      </IconButton>
    </span>
  </Tooltip>
);

const ProvisionedIcon = () => {
  const { t } = useTranslation();
  return (
    <Tooltip title={t("admin_provisioned_tooltip")}>
      <LockIcon fontSize="small" color="disabled" sx={{ verticalAlign: "middle" }} />
    </Tooltip>
  );
};

const PermissionIcon = (props) => {
  const { t } = useTranslation();
  const title = t(`admin_permission_${props.permission.replace("-", "_")}`);
  if (props.permission === "read-write") {
    return <PermissionReadWrite size="small" title={title} />;
  } else if (props.permission === "read-only") {
    return <PermissionRead size="small" title={title} />;
  } else if (props.permission === "write-only") {
    return <PermissionWrite size="small" title={title} />;
  }
  return <PermissionDenyAll size="small" title={title} />;
};

const displayUsername = (username, t) => (username === EVERYONE ? t("admin_everyone") : username);

//
// Users card
//

const Users = () => {
  const { t } = useTranslation();
  const { refresh } = useContext(AdminRefreshContext);
  const list = usePaginatedList(fetchUsersPage);
  const tiers = useTiers();
  const [dialog, setDialog] = useState(null); // { type, user, grant, key }

  const openDialog = (type, user, grant) => setDialog({ type, user, grant, key: Date.now() });
  const closeDialog = (changed) => {
    setDialog(null);
    if (changed) {
      refresh();
    }
  };

  return (
    <AdminCard title={t("admin_users_title")} description={t("admin_users_description")}>
      <ListToolbar
        list={list}
        searchPlaceholder={t("admin_users_search_placeholder")}
        actions={
          <Button variant="outlined" startIcon={<PersonAddIcon />} onClick={() => openDialog("add")} sx={{ whiteSpace: "nowrap" }}>
            {t("admin_users_add_button")}
          </Button>
        }
      />
      <ListBody list={list}>
        <Table size="small" aria-label={t("admin_users_title")}>
          <TableHead>
            <TableRow>
              <TableCell>{t("admin_users_table_username_header")}</TableCell>
              <TableCell>{t("admin_users_table_tier_header")}</TableCell>
              <TableCell>{t("admin_users_table_access_header")}</TableCell>
              <TableCell>{t("admin_users_table_reservations_header")}</TableCell>
              <TableCell align="right" />
            </TableRow>
          </TableHead>
          <TableBody>
            {list.rows.map((user) => (
              <UserRow key={user.username} user={user} onAction={openDialog} />
            ))}
          </TableBody>
        </Table>
      </ListBody>
      {dialog?.type === "add" && <AddUserDialog key={dialog.key} tiers={tiers} onClose={closeDialog} />}
      {dialog?.type === "edit" && <EditUserDialog key={dialog.key} user={dialog.user} tiers={tiers} onClose={closeDialog} />}
      {dialog?.type === "delete" && <DeleteUserDialog key={dialog.key} user={dialog.user} onClose={closeDialog} />}
      {dialog?.type === "addAccess" && <AccessDialog key={dialog.key} username={dialog.user.username} onClose={closeDialog} />}
      {dialog?.type === "removeAccess" && (
        <RemoveAccessDialog
          key={dialog.key}
          username={dialog.user.username}
          topic={dialog.grant.topic}
          reservation={dialog.grant.reservation}
          onClose={closeDialog}
        />
      )}
    </AdminCard>
  );
};

const UserRow = (props) => {
  const { t } = useTranslation();
  const { user } = props;
  const isAdmin = user.role === Role.ADMIN;
  const isEveryone = user.username === EVERYONE;
  const reservedTopics = (user.reservations || []).map((r) => r.topic);
  const grants = (user.grants || []).filter((g) => !reservedTopics.includes(g.topic));
  const canEdit = !isAdmin && !isEveryone && !user.provisioned;
  return (
    <TableRow sx={{ verticalAlign: "top" }}>
      <TableCell>
        <Stack direction="row" spacing={0.5} alignItems="center" flexWrap="wrap" useFlexGap>
          <span style={isEveryone ? { fontStyle: "italic" } : undefined}>{displayUsername(user.username, t)}</span>
          {user.provisioned && <ProvisionedIcon />}
          {isAdmin && <Chip label={t("admin_users_role_admin")} size="small" color="primary" />}
          {user.billing_status && (
            <Chip
              label={user.billing_status}
              size="small"
              variant="outlined"
              color={user.billing_status === "active" ? "success" : "default"}
            />
          )}
        </Stack>
      </TableCell>
      <TableCell>{user.tier || "-"}</TableCell>
      <TableCell>
        {isAdmin && (
          <Typography variant="body2" color="text.secondary">
            {t("admin_users_admin_access")}
          </Typography>
        )}
        {!isAdmin && (
          <Stack direction="row" spacing={0.5} flexWrap="wrap" useFlexGap>
            {grants.map((grant) => (
              <Chip
                key={grant.topic}
                size="small"
                variant="outlined"
                icon={<PermissionIcon permission={grant.permission} />}
                label={
                  <>
                    {grant.topic} {grant.provisioned && <LockIcon sx={{ fontSize: 12, verticalAlign: "middle" }} />}
                  </>
                }
                onDelete={grant.provisioned ? undefined : () => props.onAction("removeAccess", user, grant)}
              />
            ))}
            {grants.length === 0 && "-"}
          </Stack>
        )}
      </TableCell>
      <TableCell>
        <Stack direction="row" spacing={0.5} flexWrap="wrap" useFlexGap>
          {(user.reservations || []).map((reservation) => (
            <Tooltip
              key={reservation.topic}
              title={t("admin_users_reservation_tooltip", { permission: t(`admin_permission_${reservation.everyone.replace("-", "_")}`) })}
            >
              <Chip
                size="small"
                icon={<PermissionIcon permission={reservation.everyone} />}
                label={reservation.topic}
                onDelete={() => props.onAction("removeAccess", user, { topic: reservation.topic, reservation: true })}
              />
            </Tooltip>
          ))}
          {(user.reservations || []).length === 0 && "-"}
        </Stack>
      </TableCell>
      <TableCell align="right" sx={{ whiteSpace: "nowrap" }}>
        {!isAdmin && (
          <ActionButton label={t("admin_users_add_access_tooltip")} onClick={() => props.onAction("addAccess", user)}>
            <AddIcon />
          </ActionButton>
        )}
        {!isEveryone && (
          <ActionButton
            label={t("admin_users_edit_tooltip")}
            disabledReason={t("admin_users_cannot_edit_tooltip")}
            disabled={!canEdit}
            onClick={() => props.onAction("edit", user)}
          >
            <EditIcon />
          </ActionButton>
        )}
        {!isEveryone && (
          <ActionButton
            label={t("admin_users_delete_tooltip")}
            disabledReason={t("admin_users_cannot_edit_tooltip")}
            disabled={!canEdit}
            onClick={() => props.onAction("delete", user)}
          >
            <DeleteOutlineIcon />
          </ActionButton>
        )}
      </TableCell>
    </TableRow>
  );
};

const TierSelect = (props) => {
  const { t } = useTranslation();
  return (
    <FormControl fullWidth variant="standard" margin="dense">
      <InputLabel id="admin-tier-select">{t("admin_users_dialog_tier_label")}</InputLabel>
      <Select labelId="admin-tier-select" value={props.value} onChange={(ev) => props.onChange(ev.target.value)}>
        <MenuItem value={TIER_RESET}>
          <em>{t("admin_users_dialog_tier_none")}</em>
        </MenuItem>
        {props.tiers.map((tier) => (
          <MenuItem key={tier.code} value={tier.code}>
            {tier.name} ({tier.code})
          </MenuItem>
        ))}
      </Select>
    </FormControl>
  );
};

const AddUserDialog = (props) => {
  const theme = useTheme();
  const { t } = useTranslation();
  const fullScreen = useMediaQuery(theme.breakpoints.down("sm"));
  const [username, setUsername] = useState("");
  const [password, setPassword] = useState("");
  const [tier, setTier] = useState(TIER_RESET);
  const [error, setError] = useState("");

  const handleSubmit = async () => {
    try {
      await adminApi.addUser(username.trim(), password, tier === TIER_RESET ? undefined : tier);
      props.onClose(true);
    } catch (e) {
      await handleError(e, setError);
    }
  };

  return (
    <Dialog open onClose={() => props.onClose(false)} maxWidth="sm" fullWidth fullScreen={fullScreen}>
      <DialogTitle>{t("admin_users_add_dialog_title")}</DialogTitle>
      <DialogContent>
        <TextField
          margin="dense"
          label={t("admin_users_dialog_username_label")}
          value={username}
          onChange={(ev) => setUsername(ev.target.value)}
          fullWidth
          variant="standard"
          autoFocus
        />
        <TextField
          margin="dense"
          label={t("admin_users_dialog_password_label")}
          type="password"
          autoComplete="new-password"
          value={password}
          onChange={(ev) => setPassword(ev.target.value)}
          fullWidth
          variant="standard"
        />
        <TierSelect value={tier} tiers={props.tiers} onChange={setTier} />
      </DialogContent>
      <DialogFooter status={error}>
        <Button onClick={() => props.onClose(false)}>{t("common_cancel")}</Button>
        <Button onClick={handleSubmit} disabled={!username.trim() || !password}>
          {t("common_add")}
        </Button>
      </DialogFooter>
    </Dialog>
  );
};

const EditUserDialog = (props) => {
  const theme = useTheme();
  const { t } = useTranslation();
  const fullScreen = useMediaQuery(theme.breakpoints.down("sm"));
  const initialTier = props.user.tier || TIER_RESET;
  const [password, setPassword] = useState("");
  const [tier, setTier] = useState(initialTier);
  const [error, setError] = useState("");
  const tierChanged = tier !== initialTier;

  const handleSubmit = async () => {
    try {
      await adminApi.updateUser(props.user.username, password || undefined, tierChanged ? tier : undefined);
      props.onClose(true);
    } catch (e) {
      await handleError(e, setError);
    }
  };

  return (
    <Dialog open onClose={() => props.onClose(false)} maxWidth="sm" fullWidth fullScreen={fullScreen}>
      <DialogTitle>{t("admin_users_edit_dialog_title", { username: props.user.username })}</DialogTitle>
      <DialogContent>
        <TextField
          margin="dense"
          label={t("admin_users_edit_dialog_password_label")}
          type="password"
          autoComplete="new-password"
          value={password}
          onChange={(ev) => setPassword(ev.target.value)}
          fullWidth
          variant="standard"
          helperText={t("admin_users_edit_dialog_password_helper")}
        />
        <TierSelect value={tier} tiers={props.tiers} onChange={setTier} />
        {tierChanged && props.user.billing_status && (
          <Alert severity="warning" sx={{ mt: 2 }}>
            {t("admin_users_edit_dialog_tier_billing_warning")}
          </Alert>
        )}
      </DialogContent>
      <DialogFooter status={error}>
        <Button onClick={() => props.onClose(false)}>{t("common_cancel")}</Button>
        <Button onClick={handleSubmit} disabled={!password && !tierChanged}>
          {t("common_save")}
        </Button>
      </DialogFooter>
    </Dialog>
  );
};

const DeleteUserDialog = (props) => {
  const { t } = useTranslation();
  const [error, setError] = useState("");
  const reservations = props.user.reservations?.length || 0;

  const handleSubmit = async () => {
    try {
      await adminApi.deleteUser(props.user.username);
      props.onClose(true);
    } catch (e) {
      await handleError(e, setError);
    }
  };

  return (
    <Dialog open onClose={() => props.onClose(false)}>
      <DialogTitle>{t("admin_users_delete_dialog_title")}</DialogTitle>
      <DialogContent>
        <DialogContentText>{t("admin_users_delete_dialog_description", { username: props.user.username })}</DialogContentText>
        {reservations > 0 && (
          <Alert severity="warning" sx={{ mt: 2 }}>
            {t("admin_users_delete_dialog_reservations_warning", { count: reservations })}
          </Alert>
        )}
        {props.user.billing_status && (
          <Alert severity="warning" sx={{ mt: 2 }}>
            {t("admin_users_delete_dialog_billing_warning", { status: props.user.billing_status })}
          </Alert>
        )}
      </DialogContent>
      <DialogFooter status={error}>
        <Button onClick={() => props.onClose(false)}>{t("common_cancel")}</Button>
        <Button onClick={handleSubmit} color="error">
          {t("admin_users_delete_dialog_button")}
        </Button>
      </DialogFooter>
    </Dialog>
  );
};

//
// Access control (shared between the users and the ACL card)
//

const PermissionSelect = (props) => {
  const { t } = useTranslation();
  return (
    <FormControl fullWidth variant="standard" margin="dense">
      <InputLabel id="admin-permission-select">{props.label ?? t("admin_access_dialog_permission_label")}</InputLabel>
      <Select labelId="admin-permission-select" value={props.value} onChange={(ev) => props.onChange(ev.target.value)}>
        {PERMISSIONS.map((permission) => (
          <MenuItem key={permission} value={permission}>
            <Stack direction="row" spacing={1} alignItems="center">
              <PermissionIcon permission={permission} />
              <span>{t(`admin_permission_${permission.replace("-", "_")}`)}</span>
            </Stack>
          </MenuItem>
        ))}
      </Select>
    </FormControl>
  );
};

// AccessDialog adds an access control entry, or changes the permission of an existing one. If no
// username is passed, the username can be entered (use "*" for everyone).
const AccessDialog = (props) => {
  const theme = useTheme();
  const { t } = useTranslation();
  const fullScreen = useMediaQuery(theme.breakpoints.down("sm"));
  const editing = !!props.topic;
  const [username, setUsername] = useState(props.username ?? "");
  const [topic, setTopic] = useState(props.topic ?? "");
  const [permission, setPermission] = useState(props.permission ?? "read-write");
  const [error, setError] = useState("");

  const handleSubmit = async () => {
    try {
      await adminApi.allowAccess(username.trim(), topic.trim(), permission);
      props.onClose(true);
    } catch (e) {
      await handleError(e, setError);
    }
  };

  const title = editing
    ? t("admin_access_edit_dialog_title", { topic: props.topic, username: displayUsername(username, t) })
    : t("admin_access_add_dialog_title");
  return (
    <Dialog open onClose={() => props.onClose(false)} maxWidth="sm" fullWidth fullScreen={fullScreen}>
      <DialogTitle>{title}</DialogTitle>
      <DialogContent>
        {props.username === undefined && (
          <TextField
            margin="dense"
            label={t("admin_access_dialog_username_label")}
            value={username}
            onChange={(ev) => setUsername(ev.target.value)}
            fullWidth
            variant="standard"
            autoFocus
            helperText={t("admin_access_dialog_username_helper")}
          />
        )}
        {props.username !== undefined && !editing && (
          <DialogContentText>{t("admin_access_add_dialog_description", { username: displayUsername(username, t) })}</DialogContentText>
        )}
        {!editing && (
          <TextField
            margin="dense"
            label={t("admin_access_dialog_topic_label")}
            value={topic}
            onChange={(ev) => setTopic(ev.target.value)}
            fullWidth
            variant="standard"
            autoFocus={props.username !== undefined}
            helperText={t("admin_access_dialog_topic_helper")}
          />
        )}
        <PermissionSelect value={permission} onChange={setPermission} />
      </DialogContent>
      <DialogFooter status={error}>
        <Button onClick={() => props.onClose(false)}>{t("common_cancel")}</Button>
        <Button onClick={handleSubmit} disabled={!username.trim() || !topic.trim()}>
          {editing ? t("common_save") : t("common_add")}
        </Button>
      </DialogFooter>
    </Dialog>
  );
};

// RemoveAccessDialog removes an access control entry. For reservations, the entry of the owner is
// removed, which removes the entire reservation (including the Everyone entry).
const RemoveAccessDialog = (props) => {
  const { t } = useTranslation();
  const [error, setError] = useState("");

  const handleSubmit = async () => {
    try {
      if (props.reservation) {
        await adminApi.deleteReservation(props.username, props.topic);
      } else {
        await adminApi.resetAccess(props.username, props.topic);
      }
      props.onClose(true);
    } catch (e) {
      await handleError(e, setError);
    }
  };

  const description = props.reservation
    ? t("admin_access_remove_reservation_dialog_description", { username: props.username, topic: props.topic })
    : t("admin_access_remove_dialog_description", { username: displayUsername(props.username, t), topic: props.topic });
  return (
    <Dialog open onClose={() => props.onClose(false)}>
      <DialogTitle>
        {props.reservation ? t("admin_access_remove_reservation_dialog_title") : t("admin_access_remove_dialog_title")}
      </DialogTitle>
      <DialogContent>
        <DialogContentText>{description}</DialogContentText>
      </DialogContent>
      <DialogFooter status={error}>
        <Button onClick={() => props.onClose(false)}>{t("common_cancel")}</Button>
        <Button onClick={handleSubmit} color="error">
          {t("admin_access_remove_dialog_button")}
        </Button>
      </DialogFooter>
    </Dialog>
  );
};

//
// Access control list card
//

const AccessControlList = () => {
  const { t } = useTranslation();
  const { refresh } = useContext(AdminRefreshContext);
  const list = usePaginatedList(fetchAccessPage);
  const [dialog, setDialog] = useState(null); // { type, entry, key }

  const openDialog = (type, entry) => setDialog({ type, entry, key: Date.now() });
  const closeDialog = (changed) => {
    setDialog(null);
    if (changed) {
      refresh();
    }
  };

  return (
    <AdminCard title={t("admin_access_title")} description={t("admin_access_description")}>
      <ListToolbar
        list={list}
        searchPlaceholder={t("admin_access_search_placeholder")}
        actions={
          <Button variant="outlined" startIcon={<AddIcon />} onClick={() => openDialog("add")} sx={{ whiteSpace: "nowrap" }}>
            {t("admin_access_add_button")}
          </Button>
        }
      />
      <ListBody list={list}>
        <Table size="small" aria-label={t("admin_access_title")}>
          <TableHead>
            <TableRow>
              <TableCell>{t("admin_access_table_topic_header")}</TableCell>
              <TableCell>{t("admin_access_table_user_header")}</TableCell>
              <TableCell>{t("admin_access_table_permission_header")}</TableCell>
              <TableCell>{t("admin_access_table_notes_header")}</TableCell>
              <TableCell align="right" />
            </TableRow>
          </TableHead>
          <TableBody>
            {list.rows.map((entry) => (
              <AccessEntryRow key={`${entry.username}/${entry.topic}`} entry={entry} onAction={openDialog} />
            ))}
          </TableBody>
        </Table>
      </ListBody>
      {dialog?.type === "add" && <AccessDialog key={dialog.key} onClose={closeDialog} />}
      {dialog?.type === "edit" && (
        <AccessDialog
          key={dialog.key}
          username={dialog.entry.username}
          topic={dialog.entry.topic}
          permission={dialog.entry.permission}
          onClose={closeDialog}
        />
      )}
      {dialog?.type === "remove" && (
        <RemoveAccessDialog
          key={dialog.key}
          username={dialog.entry.owner || dialog.entry.username}
          topic={dialog.entry.topic}
          reservation={!!dialog.entry.owner}
          onClose={closeDialog}
        />
      )}
    </AdminCard>
  );
};

const AccessEntryRow = (props) => {
  const { t } = useTranslation();
  const { entry } = props;
  const canEdit = !entry.provisioned && !entry.owner;
  const canRemove = !entry.provisioned;
  return (
    <TableRow>
      <TableCell>{entry.topic}</TableCell>
      <TableCell sx={entry.username === EVERYONE ? { fontStyle: "italic" } : undefined}>{displayUsername(entry.username, t)}</TableCell>
      <TableCell>
        <Stack direction="row" spacing={1} alignItems="center">
          <PermissionIcon permission={entry.permission} />
          <span>{t(`admin_permission_${entry.permission.replace("-", "_")}`)}</span>
        </Stack>
      </TableCell>
      <TableCell>
        <Stack direction="row" spacing={0.5} alignItems="center" flexWrap="wrap" useFlexGap>
          {entry.owner && <Chip size="small" variant="outlined" label={t("admin_access_reserved_by", { username: entry.owner })} />}
          {entry.provisioned && (
            <>
              <ProvisionedIcon />
              <Typography variant="body2" color="text.secondary">
                {t("admin_access_provisioned")}
              </Typography>
            </>
          )}
        </Stack>
      </TableCell>
      <TableCell align="right" sx={{ whiteSpace: "nowrap" }}>
        <ActionButton
          label={t("admin_access_edit_tooltip")}
          disabledReason={t("admin_access_cannot_edit_tooltip")}
          disabled={!canEdit}
          onClick={() => props.onAction("edit", entry)}
        >
          <EditIcon />
        </ActionButton>
        <ActionButton
          label={t("admin_access_remove_tooltip")}
          disabledReason={t("admin_provisioned_tooltip")}
          disabled={!canRemove}
          onClick={() => props.onAction("remove", entry)}
        >
          <DeleteOutlineIcon />
        </ActionButton>
      </TableCell>
    </TableRow>
  );
};

//
// Reservations card
//

const Reservations = () => {
  const { t } = useTranslation();
  const { refresh } = useContext(AdminRefreshContext);
  const list = usePaginatedList(fetchReservationsPage);
  const [dialog, setDialog] = useState(null); // { type, reservation, key }

  const openDialog = (type, reservation) => setDialog({ type, reservation, key: Date.now() });
  const closeDialog = (changed) => {
    setDialog(null);
    if (changed) {
      refresh();
    }
  };

  return (
    <AdminCard title={t("admin_reservations_title")} description={t("admin_reservations_description")}>
      <ListToolbar
        list={list}
        searchPlaceholder={t("admin_reservations_search_placeholder")}
        actions={
          <Button variant="outlined" startIcon={<AddIcon />} onClick={() => openDialog("add")} sx={{ whiteSpace: "nowrap" }}>
            {t("admin_reservations_add_button")}
          </Button>
        }
      />
      <ListBody list={list}>
        <Table size="small" aria-label={t("admin_reservations_title")}>
          <TableHead>
            <TableRow>
              <TableCell>{t("admin_reservations_table_topic_header")}</TableCell>
              <TableCell>{t("admin_reservations_table_owner_header")}</TableCell>
              <TableCell>{t("admin_reservations_table_everyone_header")}</TableCell>
              <TableCell align="right" />
            </TableRow>
          </TableHead>
          <TableBody>
            {list.rows.map((reservation) => (
              <TableRow key={reservation.topic}>
                <TableCell>{reservation.topic}</TableCell>
                <TableCell>{reservation.username}</TableCell>
                <TableCell>
                  <Stack direction="row" spacing={1} alignItems="center">
                    <PermissionIcon permission={reservation.everyone} />
                    <span>{t(`admin_permission_${reservation.everyone.replace("-", "_")}`)}</span>
                  </Stack>
                </TableCell>
                <TableCell align="right" sx={{ whiteSpace: "nowrap" }}>
                  <ActionButton label={t("admin_reservations_edit_tooltip")} onClick={() => openDialog("edit", reservation)}>
                    <EditIcon />
                  </ActionButton>
                  <ActionButton label={t("admin_reservations_remove_tooltip")} onClick={() => openDialog("remove", reservation)}>
                    <DeleteOutlineIcon />
                  </ActionButton>
                </TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      </ListBody>
      {(dialog?.type === "add" || dialog?.type === "edit") && (
        <ReservationDialog key={dialog.key} reservation={dialog.reservation} onClose={closeDialog} />
      )}
      {dialog?.type === "remove" && (
        <RemoveAccessDialog
          key={dialog.key}
          username={dialog.reservation.username}
          topic={dialog.reservation.topic}
          reservation
          onClose={closeDialog}
        />
      )}
    </AdminCard>
  );
};

// ReservationDialog reserves a topic for a user, or changes the access for everyone else of an
// existing reservation
const ReservationDialog = (props) => {
  const theme = useTheme();
  const { t } = useTranslation();
  const fullScreen = useMediaQuery(theme.breakpoints.down("sm"));
  const editing = !!props.reservation;
  const [username, setUsername] = useState(props.reservation?.username ?? "");
  const [topic, setTopic] = useState(props.reservation?.topic ?? "");
  const [everyone, setEveryone] = useState(props.reservation?.everyone ?? "deny-all");
  const [error, setError] = useState("");

  const handleSubmit = async () => {
    try {
      await adminApi.updateReservation(username.trim(), topic.trim(), everyone);
      props.onClose(true);
    } catch (e) {
      await handleError(e, setError);
    }
  };

  return (
    <Dialog open onClose={() => props.onClose(false)} maxWidth="sm" fullWidth fullScreen={fullScreen}>
      <DialogTitle>
        {editing ? t("admin_reservations_edit_dialog_title", { topic: props.reservation.topic }) : t("admin_reservations_add_dialog_title")}
      </DialogTitle>
      <DialogContent>
        {!editing && (
          <>
            <DialogContentText>{t("admin_reservations_add_dialog_description")}</DialogContentText>
            <TextField
              margin="dense"
              label={t("admin_reservations_dialog_username_label")}
              value={username}
              onChange={(ev) => setUsername(ev.target.value)}
              fullWidth
              variant="standard"
              autoFocus
            />
            <TextField
              margin="dense"
              label={t("admin_reservations_dialog_topic_label")}
              value={topic}
              onChange={(ev) => setTopic(ev.target.value)}
              fullWidth
              variant="standard"
            />
          </>
        )}
        <PermissionSelect label={t("admin_reservations_dialog_everyone_label")} value={everyone} onChange={setEveryone} />
      </DialogContent>
      <DialogFooter status={error}>
        <Button onClick={() => props.onClose(false)}>{t("common_cancel")}</Button>
        <Button onClick={handleSubmit} disabled={!username.trim() || !topic.trim()}>
          {editing ? t("common_save") : t("common_add")}
        </Button>
      </DialogFooter>
    </Dialog>
  );
};

//
// Tiers card
//

const formatLimit = (value) => (value ? value.toLocaleString() : "-");
const formatBytesLimit = (value) => (value ? formatBytes(value, 0) : "-");
const formatHours = (seconds, t) => (seconds ? t("admin_tiers_hours", { count: Math.round(seconds / SECONDS_PER_HOUR) }) : "-");

const Tiers = () => {
  const { t } = useTranslation();
  const [tiers, setTiers] = useState(null);
  const [error, setError] = useState("");
  const [dialog, setDialog] = useState(null); // { type, tier, key }
  const [reloadCounter, setReloadCounter] = useState(0);
  const { version, refresh } = useContext(AdminRefreshContext);

  useEffect(() => {
    (async () => {
      try {
        setTiers(await adminApi.getTiers());
        setError("");
      } catch (e) {
        await handleError(e, setError);
      }
    })();
  }, [reloadCounter, version]);

  const openDialog = (type, tier) => setDialog({ type, tier, key: Date.now() });
  const closeDialog = (changed) => {
    setDialog(null);
    if (changed) {
      refresh();
    }
  };

  return (
    <AdminCard title={t("admin_tiers_title")} description={t("admin_tiers_description")}>
      <Stack direction="row" spacing={1} justifyContent="flex-end" sx={{ mb: 1 }}>
        <Tooltip title={t("common_refresh")}>
          <IconButton onClick={() => setReloadCounter((prev) => prev + 1)}>
            <RefreshIcon />
          </IconButton>
        </Tooltip>
        <Button variant="outlined" startIcon={<AddIcon />} onClick={() => openDialog("add")}>
          {t("admin_tiers_add_button")}
        </Button>
      </Stack>
      {error && (
        <Alert severity="error" sx={{ mb: 1 }}>
          {error}
        </Alert>
      )}
      {!tiers && !error && (
        <Box sx={{ display: "flex", justifyContent: "center", p: 3 }}>
          <CircularProgress />
        </Box>
      )}
      {tiers && (
        <Box sx={{ width: "100%", overflowX: "auto" }}>
          <Table size="small" aria-label={t("admin_tiers_title")}>
            <TableHead>
              <TableRow>
                <TableCell>{t("admin_tiers_table_tier_header")}</TableCell>
                <TableCell align="right">{t("admin_tiers_table_users_header")}</TableCell>
                <TableCell align="right">{t("admin_tiers_table_messages_header")}</TableCell>
                <TableCell align="right">{t("admin_tiers_table_emails_header")}</TableCell>
                <TableCell align="right">{t("admin_tiers_table_calls_header")}</TableCell>
                <TableCell align="right">{t("admin_tiers_table_reservations_header")}</TableCell>
                <TableCell align="right">{t("admin_tiers_table_attachments_header")}</TableCell>
                <TableCell>{t("admin_tiers_table_billing_header")}</TableCell>
                <TableCell align="right" />
              </TableRow>
            </TableHead>
            <TableBody>
              {tiers.map((tier) => (
                <TableRow key={tier.code} sx={{ verticalAlign: "top" }}>
                  <TableCell>
                    <div>{tier.name}</div>
                    <Typography variant="caption" color="text.secondary">
                      {tier.code}
                    </Typography>
                  </TableCell>
                  <TableCell align="right">{tier.users.toLocaleString()}</TableCell>
                  <TableCell align="right">
                    <div>{formatLimit(tier.messages_limit)}</div>
                    <Typography variant="caption" color="text.secondary">
                      {formatHours(tier.messages_expiry_duration, t)}
                    </Typography>
                  </TableCell>
                  <TableCell align="right">{formatLimit(tier.emails_limit)}</TableCell>
                  <TableCell align="right">{formatLimit(tier.calls_limit)}</TableCell>
                  <TableCell align="right">{formatLimit(tier.reservations_limit)}</TableCell>
                  <TableCell align="right">
                    {!tier.attachment_file_size_limit && "-"}
                    {!!tier.attachment_file_size_limit && (
                      <>
                        <div>{t("admin_tiers_attachment_file_size", { size: formatBytesLimit(tier.attachment_file_size_limit) })}</div>
                        <Typography variant="caption" color="text.secondary" component="div">
                          {t("admin_tiers_attachment_total_size", { size: formatBytesLimit(tier.attachment_total_size_limit) })}
                        </Typography>
                        <Typography variant="caption" color="text.secondary" component="div">
                          {t("admin_tiers_attachment_bandwidth", { size: formatBytesLimit(tier.attachment_bandwidth_limit) })}
                        </Typography>
                      </>
                    )}
                  </TableCell>
                  <TableCell>
                    {tier.stripe_monthly_price_id || tier.stripe_yearly_price_id ? (
                      <Chip size="small" variant="outlined" label={t("admin_tiers_billing_stripe")} />
                    ) : (
                      "-"
                    )}
                  </TableCell>
                  <TableCell align="right" sx={{ whiteSpace: "nowrap" }}>
                    <ActionButton label={t("admin_tiers_edit_tooltip")} onClick={() => openDialog("edit", tier)}>
                      <EditIcon />
                    </ActionButton>
                    <ActionButton
                      label={t("admin_tiers_delete_tooltip")}
                      disabledReason={t("admin_tiers_cannot_delete_tooltip")}
                      disabled={tier.users > 0}
                      onClick={() => openDialog("delete", tier)}
                    >
                      <DeleteOutlineIcon />
                    </ActionButton>
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
          {tiers.length === 0 && (
            <Typography variant="body2" color="text.secondary" sx={{ p: 2, textAlign: "center" }}>
              {t("admin_tiers_empty")}
            </Typography>
          )}
        </Box>
      )}
      {(dialog?.type === "add" || dialog?.type === "edit") && <TierDialog key={dialog.key} tier={dialog.tier} onClose={closeDialog} />}
      {dialog?.type === "delete" && <DeleteTierDialog key={dialog.key} tier={dialog.tier} onClose={closeDialog} />}
    </AdminCard>
  );
};

// Tier form fields, with the unit conversion between the form (MB, hours) and the API (bytes, seconds)
const TIER_FIELDS = [
  { key: "messages_limit", label: "admin_tiers_dialog_messages_limit", factor: 1 },
  { key: "messages_expiry_duration", label: "admin_tiers_dialog_messages_expiry_duration", factor: SECONDS_PER_HOUR },
  { key: "emails_limit", label: "admin_tiers_dialog_emails_limit", factor: 1 },
  { key: "calls_limit", label: "admin_tiers_dialog_calls_limit", factor: 1 },
  { key: "reservations_limit", label: "admin_tiers_dialog_reservations_limit", factor: 1 },
  { key: "attachment_file_size_limit", label: "admin_tiers_dialog_attachment_file_size_limit", factor: BYTES_PER_MB },
  { key: "attachment_total_size_limit", label: "admin_tiers_dialog_attachment_total_size_limit", factor: BYTES_PER_MB },
  { key: "attachment_expiry_duration", label: "admin_tiers_dialog_attachment_expiry_duration", factor: SECONDS_PER_HOUR },
  { key: "attachment_bandwidth_limit", label: "admin_tiers_dialog_attachment_bandwidth_limit", factor: BYTES_PER_MB },
];

const TierDialog = (props) => {
  const theme = useTheme();
  const { t } = useTranslation();
  const fullScreen = useMediaQuery(theme.breakpoints.down("sm"));
  const editing = !!props.tier;
  const [code, setCode] = useState(props.tier?.code ?? "");
  const [name, setName] = useState(props.tier?.name ?? "");
  const [stripeMonthlyPriceId, setStripeMonthlyPriceId] = useState(props.tier?.stripe_monthly_price_id ?? "");
  const [stripeYearlyPriceId, setStripeYearlyPriceId] = useState(props.tier?.stripe_yearly_price_id ?? "");
  const [values, setValues] = useState(() =>
    Object.fromEntries(TIER_FIELDS.map((f) => [f.key, props.tier ? String(Math.round((props.tier[f.key] || 0) / f.factor)) : "0"])),
  );
  const [error, setError] = useState("");
  const valid = code.trim() && name.trim() && TIER_FIELDS.every((f) => /^\d+$/.test(values[f.key]));

  const handleSubmit = async () => {
    const tier = {
      code: code.trim(),
      name: name.trim(),
      stripe_monthly_price_id: stripeMonthlyPriceId.trim(),
      stripe_yearly_price_id: stripeYearlyPriceId.trim(),
      ...Object.fromEntries(TIER_FIELDS.map((f) => [f.key, parseInt(values[f.key], 10) * f.factor])),
    };
    try {
      if (editing) {
        await adminApi.updateTier(tier);
      } else {
        await adminApi.addTier(tier);
      }
      props.onClose(true);
    } catch (e) {
      await handleError(e, setError);
    }
  };

  return (
    <Dialog open onClose={() => props.onClose(false)} maxWidth="sm" fullWidth fullScreen={fullScreen}>
      <DialogTitle>
        {editing ? t("admin_tiers_edit_dialog_title", { name: props.tier.name }) : t("admin_tiers_add_dialog_title")}
      </DialogTitle>
      <DialogContent>
        <Stack direction={{ xs: "column", sm: "row" }} spacing={{ sm: 2 }}>
          <TextField
            margin="dense"
            label={t("admin_tiers_dialog_code_label")}
            value={code}
            onChange={(ev) => setCode(ev.target.value)}
            disabled={editing}
            fullWidth
            variant="standard"
            autoFocus={!editing}
          />
          <TextField
            margin="dense"
            label={t("admin_tiers_dialog_name_label")}
            value={name}
            onChange={(ev) => setName(ev.target.value)}
            fullWidth
            variant="standard"
          />
        </Stack>
        <Box sx={{ display: "grid", gridTemplateColumns: { xs: "1fr", sm: "1fr 1fr" }, columnGap: 2 }}>
          {TIER_FIELDS.map((f) => (
            <TextField
              key={f.key}
              margin="dense"
              label={t(f.label)}
              value={values[f.key]}
              onChange={(ev) => setValues((prev) => ({ ...prev, [f.key]: ev.target.value.trim() }))}
              error={!/^\d+$/.test(values[f.key])}
              fullWidth
              variant="standard"
              slotProps={{ htmlInput: { inputMode: "numeric" } }}
            />
          ))}
        </Box>
        <Stack direction={{ xs: "column", sm: "row" }} spacing={{ sm: 2 }}>
          <TextField
            margin="dense"
            label={t("admin_tiers_dialog_stripe_monthly_price_id_label")}
            value={stripeMonthlyPriceId}
            onChange={(ev) => setStripeMonthlyPriceId(ev.target.value)}
            fullWidth
            variant="standard"
          />
          <TextField
            margin="dense"
            label={t("admin_tiers_dialog_stripe_yearly_price_id_label")}
            value={stripeYearlyPriceId}
            onChange={(ev) => setStripeYearlyPriceId(ev.target.value)}
            fullWidth
            variant="standard"
          />
        </Stack>
        {editing && props.tier.users > 0 && (
          <Alert severity="info" sx={{ mt: 2 }}>
            {t("admin_tiers_edit_dialog_users_info", { count: props.tier.users })}
          </Alert>
        )}
      </DialogContent>
      <DialogFooter status={error}>
        <Button onClick={() => props.onClose(false)}>{t("common_cancel")}</Button>
        <Button onClick={handleSubmit} disabled={!valid}>
          {editing ? t("common_save") : t("common_add")}
        </Button>
      </DialogFooter>
    </Dialog>
  );
};

const DeleteTierDialog = (props) => {
  const { t } = useTranslation();
  const [error, setError] = useState("");

  const handleSubmit = async () => {
    try {
      await adminApi.deleteTier(props.tier.code);
      props.onClose(true);
    } catch (e) {
      await handleError(e, setError);
    }
  };

  return (
    <Dialog open onClose={() => props.onClose(false)}>
      <DialogTitle>{t("admin_tiers_delete_dialog_title")}</DialogTitle>
      <DialogContent>
        <DialogContentText>
          {t("admin_tiers_delete_dialog_description", { name: props.tier.name, code: props.tier.code })}
        </DialogContentText>
      </DialogContent>
      <DialogFooter status={error}>
        <Button onClick={() => props.onClose(false)}>{t("common_cancel")}</Button>
        <Button onClick={handleSubmit} color="error">
          {t("admin_tiers_delete_dialog_button")}
        </Button>
      </DialogFooter>
    </Dialog>
  );
};

export default Admin;
