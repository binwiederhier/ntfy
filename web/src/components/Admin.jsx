import * as React from "react";
import { useContext, useEffect, useState } from "react";
import {
  Alert,
  CardActions,
  CardContent,
  Chip,
  FormControl,
  Select,
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableRow,
  Tooltip,
  Typography,
  Container,
  Card,
  Button,
  Dialog,
  DialogTitle,
  DialogContent,
  TextField,
  IconButton,
  MenuItem,
  DialogContentText,
  useMediaQuery,
  useTheme,
  Stack,
  CircularProgress,
  Box,
} from "@mui/material";
import EditIcon from "@mui/icons-material/Edit";
import { useTranslation } from "react-i18next";
import DeleteOutlineIcon from "@mui/icons-material/DeleteOutline";
import AddIcon from "@mui/icons-material/Add";
import CloseIcon from "@mui/icons-material/Close";
import LockIcon from "@mui/icons-material/Lock";
import routes from "./routes";
import { AccountContext } from "./App";
import DialogFooter from "./DialogFooter";
import { Paragraph } from "./styles";
import { UnauthorizedError } from "../app/errors";
import session from "../app/Session";
import adminApi from "../app/AdminApi";
import { Role } from "../app/AccountApi";

const Admin = () => {
  const { account } = useContext(AccountContext);

  // Redirect non-admins away
  if (!session.exists() || (account && account.role !== Role.ADMIN)) {
    window.location.href = routes.app;
    return null;
  }

  // Wait for account to load
  if (!account) {
    return (
      <Box sx={{ display: "flex", justifyContent: "center", alignItems: "center", height: "100vh" }}>
        <CircularProgress />
      </Box>
    );
  }

  return (
    <Container maxWidth="lg" sx={{ marginTop: 3, marginBottom: 3 }}>
      <Stack spacing={3}>
        <Users />
      </Stack>
    </Container>
  );
};

const Users = () => {
  const { t } = useTranslation();
  const [users, setUsers] = useState(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [addDialogKey, setAddDialogKey] = useState(0);
  const [addDialogOpen, setAddDialogOpen] = useState(false);

  const loadUsers = async () => {
    try {
      setLoading(true);
      const data = await adminApi.getUsers();
      setUsers(data);
      setError("");
    } catch (e) {
      console.log(`[Admin] Error loading users`, e);
      if (e instanceof UnauthorizedError) {
        await session.resetAndRedirect(routes.login);
      } else {
        setError(e.message);
      }
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    loadUsers();
  }, []);

  const handleAddClick = () => {
    setAddDialogKey((prev) => prev + 1);
    setAddDialogOpen(true);
  };

  const handleDialogClose = () => {
    setAddDialogOpen(false);
    loadUsers();
  };

  return (
    <Card sx={{ padding: 1 }} aria-label={t("admin_users_title")}>
      <CardContent sx={{ paddingBottom: 1 }}>
        <Typography variant="h5" sx={{ marginBottom: 2 }}>
          {t("admin_users_title")}
        </Typography>
        <Paragraph>{t("admin_users_description")}</Paragraph>
        {error && (
          <Alert severity="error" sx={{ mb: 2 }}>
            {error}
          </Alert>
        )}
        {loading && (
          <Box sx={{ display: "flex", justifyContent: "center", p: 3 }}>
            <CircularProgress />
          </Box>
        )}
        {!loading && users && (
          <div style={{ width: "100%", overflowX: "auto" }}>
            <UsersTable users={users} onUserChanged={loadUsers} />
          </div>
        )}
      </CardContent>
      <CardActions>
        <Button onClick={handleAddClick} startIcon={<AddIcon />}>
          {t("admin_users_add_button")}
        </Button>
      </CardActions>
      <AddUserDialog key={`addUserDialog${addDialogKey}`} open={addDialogOpen} onClose={handleDialogClose} />
    </Card>
  );
};

const UsersTable = (props) => {
  const { t } = useTranslation();
  const [editDialogKey, setEditDialogKey] = useState(0);
  const [editDialogOpen, setEditDialogOpen] = useState(false);
  const [deleteDialogOpen, setDeleteDialogOpen] = useState(false);
  const [accessDialogKey, setAccessDialogKey] = useState(0);
  const [accessDialogOpen, setAccessDialogOpen] = useState(false);
  const [deleteAccessDialogOpen, setDeleteAccessDialogOpen] = useState(false);
  const [selectedUser, setSelectedUser] = useState(null);
  const [selectedGrant, setSelectedGrant] = useState(null);

  const { users } = props;

  const handleEditClick = (user) => {
    setEditDialogKey((prev) => prev + 1);
    setSelectedUser(user);
    setEditDialogOpen(true);
  };

  const handleDeleteClick = (user) => {
    setSelectedUser(user);
    setDeleteDialogOpen(true);
  };

  const handleAddAccessClick = (user) => {
    setAccessDialogKey((prev) => prev + 1);
    setSelectedUser(user);
    setAccessDialogOpen(true);
  };

  const handleDeleteAccessClick = (user, grant) => {
    setSelectedUser(user);
    setSelectedGrant(grant);
    setDeleteAccessDialogOpen(true);
  };

  const handleDialogClose = () => {
    setEditDialogOpen(false);
    setDeleteDialogOpen(false);
    setAccessDialogOpen(false);
    setDeleteAccessDialogOpen(false);
    setSelectedUser(null);
    setSelectedGrant(null);
    props.onUserChanged();
  };

  return (
    <>
      <Table size="small" aria-label={t("admin_users_title")}>
        <TableHead>
          <TableRow>
            <TableCell sx={{ paddingLeft: 0 }}>{t("admin_users_table_username_header")}</TableCell>
            <TableCell>{t("admin_users_table_role_header")}</TableCell>
            <TableCell>{t("admin_users_table_tier_header")}</TableCell>
            <TableCell>{t("admin_users_table_grants_header")}</TableCell>
            <TableCell align="right">{t("admin_users_table_actions_header")}</TableCell>
          </TableRow>
        </TableHead>
        <TableBody>
          {users.map((user) => (
            <TableRow key={user.username} sx={{ "&:last-child td, &:last-child th": { border: 0 } }}>
              <TableCell component="th" scope="row" sx={{ paddingLeft: 0 }}>
                <Stack direction="row" spacing={0.5} alignItems="center">
                  <span>{user.username}</span>
                  {user.provisioned && (
                    <Tooltip title={t("admin_users_provisioned_tooltip")}>
                      <LockIcon fontSize="small" color="disabled" />
                    </Tooltip>
                  )}
                </Stack>
              </TableCell>
              <TableCell>
                <RoleChip role={user.role} />
              </TableCell>
              <TableCell>{user.tier || "-"}</TableCell>
              <TableCell>
                {user.grants && user.grants.length > 0 ? (
                  <Stack direction="row" spacing={0.5} flexWrap="wrap" useFlexGap>
                    {user.grants.map((grant, idx) => {
                      const canDelete = user.role !== "admin" && !grant.provisioned;
                      const tooltipText = grant.provisioned
                        ? t("admin_users_table_grant_provisioned_tooltip", { permission: grant.permission })
                        : t("admin_users_table_grant_tooltip", { permission: grant.permission });
                      return (
                        <Tooltip key={idx} title={tooltipText}>
                          <Chip
                            label={grant.topic}
                            size="small"
                            variant={grant.provisioned ? "filled" : "outlined"}
                            color={grant.provisioned ? "default" : "default"}
                            icon={grant.provisioned ? <LockIcon fontSize="small" /> : undefined}
                            onDelete={canDelete ? () => handleDeleteAccessClick(user, grant) : undefined}
                          />
                        </Tooltip>
                      );
                    })}
                  </Stack>
                ) : (
                  "-"
                )}
              </TableCell>
              <TableCell align="right" sx={{ whiteSpace: "nowrap" }}>
                {user.role !== "admin" && !user.provisioned ? (
                  <>
                    <Tooltip title={t("admin_users_table_add_access_tooltip")}>
                      <IconButton onClick={() => handleAddAccessClick(user)} size="small">
                        <AddIcon />
                      </IconButton>
                    </Tooltip>
                    <Tooltip title={t("admin_users_table_edit_tooltip")}>
                      <IconButton onClick={() => handleEditClick(user)} size="small">
                        <EditIcon />
                      </IconButton>
                    </Tooltip>
                    <Tooltip title={t("admin_users_table_delete_tooltip")}>
                      <IconButton onClick={() => handleDeleteClick(user)} size="small">
                        <DeleteOutlineIcon />
                      </IconButton>
                    </Tooltip>
                  </>
                ) : user.role !== "admin" && user.provisioned ? (
                  <>
                    <Tooltip title={t("admin_users_table_add_access_tooltip")}>
                      <IconButton onClick={() => handleAddAccessClick(user)} size="small">
                        <AddIcon />
                      </IconButton>
                    </Tooltip>
                    <Tooltip title={t("admin_users_provisioned_cannot_edit")}>
                      <span>
                        <IconButton disabled size="small">
                          <EditIcon />
                        </IconButton>
                        <IconButton disabled size="small">
                          <DeleteOutlineIcon />
                        </IconButton>
                      </span>
                    </Tooltip>
                  </>
                ) : (
                  <Tooltip title={t("admin_users_table_admin_no_actions")}>
                    <span>
                      <IconButton disabled size="small">
                        <EditIcon />
                      </IconButton>
                    </span>
                  </Tooltip>
                )}
              </TableCell>
            </TableRow>
          ))}
        </TableBody>
      </Table>
      <EditUserDialog key={`editUserDialog${editDialogKey}`} open={editDialogOpen} user={selectedUser} onClose={handleDialogClose} />
      <DeleteUserDialog open={deleteDialogOpen} user={selectedUser} onClose={handleDialogClose} />
      <AddAccessDialog key={`addAccessDialog${accessDialogKey}`} open={accessDialogOpen} user={selectedUser} onClose={handleDialogClose} />
      <DeleteAccessDialog open={deleteAccessDialogOpen} user={selectedUser} grant={selectedGrant} onClose={handleDialogClose} />
    </>
  );
};

const RoleChip = ({ role }) => {
  const { t } = useTranslation();
  if (role === "admin") {
    return <Chip label={t("admin_users_role_admin")} size="small" color="primary" />;
  }
  return <Chip label={t("admin_users_role_user")} size="small" variant="outlined" />;
};

const AddUserDialog = (props) => {
  const theme = useTheme();
  const { t } = useTranslation();
  const [error, setError] = useState("");
  const [username, setUsername] = useState("");
  const [password, setPassword] = useState("");
  const [tier, setTier] = useState("");
  const fullScreen = useMediaQuery(theme.breakpoints.down("sm"));

  const handleSubmit = async () => {
    try {
      await adminApi.addUser(username, password, tier || undefined);
      props.onClose();
    } catch (e) {
      console.log(`[Admin] Error adding user`, e);
      if (e instanceof UnauthorizedError) {
        await session.resetAndRedirect(routes.login);
      } else {
        setError(e.message);
      }
    }
  };

  return (
    <Dialog open={props.open} onClose={props.onClose} maxWidth="sm" fullWidth fullScreen={fullScreen}>
      <DialogTitle>{t("admin_users_add_dialog_title")}</DialogTitle>
      <DialogContent>
        <TextField
          margin="dense"
          id="username"
          label={t("admin_users_add_dialog_username_label")}
          type="text"
          value={username}
          onChange={(ev) => setUsername(ev.target.value)}
          fullWidth
          variant="standard"
          autoFocus
        />
        <TextField
          margin="dense"
          id="password"
          label={t("admin_users_add_dialog_password_label")}
          type="password"
          value={password}
          onChange={(ev) => setPassword(ev.target.value)}
          fullWidth
          variant="standard"
        />
        <TextField
          margin="dense"
          id="tier"
          label={t("admin_users_add_dialog_tier_label")}
          type="text"
          value={tier}
          onChange={(ev) => setTier(ev.target.value)}
          fullWidth
          variant="standard"
          helperText={t("admin_users_add_dialog_tier_helper")}
        />
      </DialogContent>
      <DialogFooter status={error}>
        <Button onClick={props.onClose}>{t("common_cancel")}</Button>
        <Button onClick={handleSubmit} disabled={!username || !password}>
          {t("common_add")}
        </Button>
      </DialogFooter>
    </Dialog>
  );
};

const EditUserDialog = (props) => {
  const theme = useTheme();
  const { t } = useTranslation();
  const [error, setError] = useState("");
  const [password, setPassword] = useState("");
  const [tier, setTier] = useState(props.user?.tier || "");
  const fullScreen = useMediaQuery(theme.breakpoints.down("sm"));

  const handleSubmit = async () => {
    try {
      await adminApi.updateUser(props.user.username, password || undefined, tier || undefined);
      props.onClose();
    } catch (e) {
      console.log(`[Admin] Error updating user`, e);
      if (e instanceof UnauthorizedError) {
        await session.resetAndRedirect(routes.login);
      } else {
        setError(e.message);
      }
    }
  };

  if (!props.user) {
    return null;
  }

  return (
    <Dialog open={props.open} onClose={props.onClose} maxWidth="sm" fullWidth fullScreen={fullScreen}>
      <DialogTitle>{t("admin_users_edit_dialog_title", { username: props.user.username })}</DialogTitle>
      <DialogContent>
        <TextField
          margin="dense"
          id="password"
          label={t("admin_users_edit_dialog_password_label")}
          type="password"
          value={password}
          onChange={(ev) => setPassword(ev.target.value)}
          fullWidth
          variant="standard"
          helperText={t("admin_users_edit_dialog_password_helper")}
        />
        <TextField
          margin="dense"
          id="tier"
          label={t("admin_users_edit_dialog_tier_label")}
          type="text"
          value={tier}
          onChange={(ev) => setTier(ev.target.value)}
          fullWidth
          variant="standard"
          helperText={t("admin_users_edit_dialog_tier_helper")}
        />
      </DialogContent>
      <DialogFooter status={error}>
        <Button onClick={props.onClose}>{t("common_cancel")}</Button>
        <Button onClick={handleSubmit} disabled={!password && !tier}>
          {t("common_save")}
        </Button>
      </DialogFooter>
    </Dialog>
  );
};

const DeleteUserDialog = (props) => {
  const { t } = useTranslation();
  const [error, setError] = useState("");

  const handleSubmit = async () => {
    try {
      await adminApi.deleteUser(props.user.username);
      props.onClose();
    } catch (e) {
      console.log(`[Admin] Error deleting user`, e);
      if (e instanceof UnauthorizedError) {
        await session.resetAndRedirect(routes.login);
      } else {
        setError(e.message);
      }
    }
  };

  if (!props.user) {
    return null;
  }

  return (
    <Dialog open={props.open} onClose={props.onClose}>
      <DialogTitle>{t("admin_users_delete_dialog_title")}</DialogTitle>
      <DialogContent>
        <DialogContentText>{t("admin_users_delete_dialog_description", { username: props.user.username })}</DialogContentText>
      </DialogContent>
      <DialogFooter status={error}>
        <Button onClick={props.onClose}>{t("common_cancel")}</Button>
        <Button onClick={handleSubmit} color="error">
          {t("admin_users_delete_dialog_button")}
        </Button>
      </DialogFooter>
    </Dialog>
  );
};

const AddAccessDialog = (props) => {
  const theme = useTheme();
  const { t } = useTranslation();
  const [error, setError] = useState("");
  const [topic, setTopic] = useState("");
  const [permission, setPermission] = useState("read-write");
  const fullScreen = useMediaQuery(theme.breakpoints.down("sm"));

  const handleSubmit = async () => {
    try {
      await adminApi.allowAccess(props.user.username, topic, permission);
      props.onClose();
    } catch (e) {
      console.log(`[Admin] Error adding access`, e);
      if (e instanceof UnauthorizedError) {
        await session.resetAndRedirect(routes.login);
      } else {
        setError(e.message);
      }
    }
  };

  if (!props.user) {
    return null;
  }

  return (
    <Dialog open={props.open} onClose={props.onClose} maxWidth="sm" fullWidth fullScreen={fullScreen}>
      <DialogTitle>{t("admin_access_add_dialog_title", { username: props.user.username })}</DialogTitle>
      <DialogContent>
        <TextField
          margin="dense"
          id="topic"
          label={t("admin_access_add_dialog_topic_label")}
          type="text"
          value={topic}
          onChange={(ev) => setTopic(ev.target.value)}
          fullWidth
          variant="standard"
          autoFocus
          helperText={t("admin_access_add_dialog_topic_helper")}
        />
        <FormControl fullWidth variant="standard" sx={{ mt: 2 }}>
          <Select
            value={permission}
            onChange={(ev) => setPermission(ev.target.value)}
            label={t("admin_access_add_dialog_permission_label")}
          >
            <MenuItem value="read-write">{t("admin_access_permission_read_write")}</MenuItem>
            <MenuItem value="read-only">{t("admin_access_permission_read_only")}</MenuItem>
            <MenuItem value="write-only">{t("admin_access_permission_write_only")}</MenuItem>
            <MenuItem value="deny-all">{t("admin_access_permission_deny_all")}</MenuItem>
          </Select>
        </FormControl>
      </DialogContent>
      <DialogFooter status={error}>
        <Button onClick={props.onClose}>{t("common_cancel")}</Button>
        <Button onClick={handleSubmit} disabled={!topic}>
          {t("common_add")}
        </Button>
      </DialogFooter>
    </Dialog>
  );
};

const DeleteAccessDialog = (props) => {
  const { t } = useTranslation();
  const [error, setError] = useState("");

  const handleSubmit = async () => {
    try {
      await adminApi.resetAccess(props.user.username, props.grant.topic);
      props.onClose();
    } catch (e) {
      console.log(`[Admin] Error removing access`, e);
      if (e instanceof UnauthorizedError) {
        await session.resetAndRedirect(routes.login);
      } else {
        setError(e.message);
      }
    }
  };

  if (!props.user || !props.grant) {
    return null;
  }

  return (
    <Dialog open={props.open} onClose={props.onClose}>
      <DialogTitle>{t("admin_access_delete_dialog_title")}</DialogTitle>
      <DialogContent>
        <DialogContentText>
          {t("admin_access_delete_dialog_description", { username: props.user.username, topic: props.grant.topic })}
        </DialogContentText>
      </DialogContent>
      <DialogFooter status={error}>
        <Button onClick={props.onClose}>{t("common_cancel")}</Button>
        <Button onClick={handleSubmit} color="error">
          {t("admin_access_delete_dialog_button")}
        </Button>
      </DialogFooter>
    </Dialog>
  );
};

export default Admin;

