import * as React from 'react';
import {useEffect, useState} from 'react';
import {
    CardActions,
    CardContent,
    FormControl,
    Select,
    Stack,
    Table,
    TableBody,
    TableCell,
    TableHead,
    TableRow,
    useMediaQuery
} from "@mui/material";
import Typography from "@mui/material/Typography";
import prefs from "../app/Prefs";
import {Paragraph} from "./styles";
import EditIcon from '@mui/icons-material/Edit';
import CloseIcon from "@mui/icons-material/Close";
import IconButton from "@mui/material/IconButton";
import PlayArrowIcon from '@mui/icons-material/PlayArrow';
import Container from "@mui/material/Container";
import TextField from "@mui/material/TextField";
import MenuItem from "@mui/material/MenuItem";
import Card from "@mui/material/Card";
import Button from "@mui/material/Button";
import {useLiveQuery} from "dexie-react-hooks";
import theme from "./theme";
import Dialog from "@mui/material/Dialog";
import DialogTitle from "@mui/material/DialogTitle";
import DialogContent from "@mui/material/DialogContent";
import DialogActions from "@mui/material/DialogActions";
import userManager from "../app/UserManager";
import {playSound, shuffle} from "../app/utils";
import {useTranslation} from "react-i18next";

const Preferences = () => {
    return (
        <Container maxWidth="md" sx={{marginTop: 3, marginBottom: 3}}>
            <Stack spacing={3}>
                <Notifications/>
                <Appearance/>
                <Users/>
            </Stack>
        </Container>
    );
};

const Notifications = () => {
    const { t } = useTranslation();
    return (
        <Card sx={{p: 3}}>
            <Typography variant="h5">
                {t("prefs_notifications_title")}
            </Typography>
            <PrefGroup>
                <Sound/>
                <MinPriority/>
                <DeleteAfter/>
            </PrefGroup>
        </Card>
    );
};

const Sound = () => {
    const { t } = useTranslation();
    const sound = useLiveQuery(async () => prefs.sound());
    const handleChange = async (ev) => {
        await prefs.setSound(ev.target.value);
    }
    if (!sound) {
        return null; // While loading
    }
    return (
        <Pref title={t("prefs_notifications_sound_title")}>
            <div style={{ display: 'flex', width: '100%' }}>
                <FormControl fullWidth variant="standard" sx={{ margin: 1 }}>
                    <Select value={sound} onChange={handleChange}>
                        <MenuItem value={"none"}>{t("prefs_notifications_sound_no_sound")}</MenuItem>
                        <MenuItem value={"ding"}>Ding</MenuItem>
                        <MenuItem value={"juntos"}>Juntos</MenuItem>
                        <MenuItem value={"pristine"}>Pristine</MenuItem>
                        <MenuItem value={"dadum"}>Dadum</MenuItem>
                        <MenuItem value={"pop"}>Pop</MenuItem>
                        <MenuItem value={"pop-swoosh"}>Pop swoosh</MenuItem>
                        <MenuItem value={"beep"}>Beep</MenuItem>
                    </Select>
                </FormControl>
                <IconButton onClick={() => playSound(sound)} disabled={sound === "none"}>
                    <PlayArrowIcon />
                </IconButton>
            </div>
        </Pref>
    )
};

const MinPriority = () => {
    const { t } = useTranslation();
    const minPriority = useLiveQuery(async () => prefs.minPriority());
    const handleChange = async (ev) => {
        await prefs.setMinPriority(ev.target.value);
    }
    if (!minPriority) {
        return null; // While loading
    }
    return (
        <Pref title={t("prefs_notifications_min_priority_title")}>
            <FormControl fullWidth variant="standard" sx={{ m: 1 }}>
                <Select value={minPriority} onChange={handleChange}>
                    <MenuItem value={1}>{t("prefs_notifications_min_priority_any")}</MenuItem>
                    <MenuItem value={2}>{t("prefs_notifications_min_priority_low_and_higher")}</MenuItem>
                    <MenuItem value={3}>{t("prefs_notifications_min_priority_default_and_higher")}</MenuItem>
                    <MenuItem value={4}>{t("prefs_notifications_min_priority_high_and_higher")}</MenuItem>
                    <MenuItem value={5}>{t("prefs_notifications_min_priority_max_only")}</MenuItem>
                </Select>
            </FormControl>
        </Pref>
    )
};

const DeleteAfter = () => {
    const { t } = useTranslation();
    const deleteAfter = useLiveQuery(async () => prefs.deleteAfter());
    const handleChange = async (ev) => {
        await prefs.setDeleteAfter(ev.target.value);
    }
    if (!deleteAfter) {
        return null; // While loading
    }
    return (
        <Pref title={t("prefs_notifications_delete_after_title")}>
            <FormControl fullWidth variant="standard" sx={{ m: 1 }}>
                <Select value={deleteAfter} onChange={handleChange}>
                    <MenuItem value={0}>{t("prefs_notifications_delete_after_never")}</MenuItem>
                    <MenuItem value={10800}>{t("prefs_notifications_delete_after_three_hours")}</MenuItem>
                    <MenuItem value={86400}>{t("prefs_notifications_delete_after_one_day")}</MenuItem>
                    <MenuItem value={604800}>{t("prefs_notifications_delete_after_one_week")}</MenuItem>
                    <MenuItem value={2592000}>{t("prefs_notifications_delete_after_one_month")}</MenuItem>
                </Select>
            </FormControl>
        </Pref>
    )
};

const PrefGroup = (props) => {
    return (
        <div style={{
            display: 'flex',
            flexWrap: 'wrap'
        }}>
            {props.children}
        </div>
    )
};

const Pref = (props) => {
    return (
        <>
            <div style={{
                flex: '1 0 30%',
                display: 'inline-flex',
                flexDirection: 'column',
                minHeight: '60px',
                justifyContent: 'center'
            }}>
                <b>{props.title}</b>
            </div>
            <div style={{
                flex: '1 0 calc(70% - 50px)',
                display: 'inline-flex',
                flexDirection: 'column',
                minHeight: '60px',
                justifyContent: 'center'
            }}>
                {props.children}
            </div>
        </>
    );
};

const Users = () => {
    const { t } = useTranslation();
    const [dialogKey, setDialogKey] = useState(0);
    const [dialogOpen, setDialogOpen] = useState(false);
    const users = useLiveQuery(() => userManager.all());
    const handleAddClick = () => {
        setDialogKey(prev => prev+1);
        setDialogOpen(true);
    };
    const handleDialogCancel = () => {
        setDialogOpen(false);
    };
    const handleDialogSubmit = async (user) => {
        setDialogOpen(false);
        try {
            await userManager.save(user);
            console.debug(`[Preferences] User ${user.username} for ${user.baseUrl} added`);
        } catch (e) {
            console.log(`[Preferences] Error adding user.`, e);
        }
    };
    return (
        <Card sx={{ padding: 1 }}>
            <CardContent>
                <Typography variant="h5">
                    {t("prefs_users_title")}
                </Typography>
                <Paragraph>
                    {t("prefs_users_description")}
                </Paragraph>
                {users?.length > 0 && <UserTable users={users}/>}
            </CardContent>
            <CardActions>
                <Button onClick={handleAddClick}>{t("prefs_users_add_button")}</Button>
                <UserDialog
                    key={`userAddDialog${dialogKey}`}
                    open={dialogOpen}
                    user={null}
                    users={users}
                    onCancel={handleDialogCancel}
                    onSubmit={handleDialogSubmit}
                />
            </CardActions>
        </Card>
    );
};

const UserTable = (props) => {
    const { t } = useTranslation();
    const [dialogKey, setDialogKey] = useState(0);
    const [dialogOpen, setDialogOpen] = useState(false);
    const [dialogUser, setDialogUser] = useState(null);
    const handleEditClick = (user) => {
        setDialogKey(prev => prev+1);
        setDialogUser(user);
        setDialogOpen(true);
    };
    const handleDialogCancel = () => {
        setDialogOpen(false);
    };
    const handleDialogSubmit = async (user) => {
        setDialogOpen(false);
        try {
            await userManager.save(user);
            console.debug(`[Preferences] User ${user.username} for ${user.baseUrl} updated`);
        } catch (e) {
            console.log(`[Preferences] Error updating user.`, e);
        }
    };
    const handleDeleteClick = async (user) => {
        try {
            await userManager.delete(user.baseUrl);
            console.debug(`[Preferences] User ${user.username} for ${user.baseUrl} deleted`);
        } catch (e) {
            console.error(`[Preferences] Error deleting user for ${user.baseUrl}`, e);
        }
    };
    return (
        <Table size="small">
            <TableHead>
                <TableRow>
                    <TableCell>{t("prefs_users_table_user_header")}</TableCell>
                    <TableCell>{t("prefs_users_table_base_url_header")}</TableCell>
                    <TableCell/>
                </TableRow>
            </TableHead>
            <TableBody>
                {props.users?.map(user => (
                    <TableRow
                        key={user.baseUrl}
                        sx={{ '&:last-child td, &:last-child th': { border: 0 } }}
                    >
                        <TableCell component="th" scope="row">{user.username}</TableCell>
                        <TableCell>{user.baseUrl}</TableCell>
                        <TableCell align="right">
                            <IconButton onClick={() => handleEditClick(user)}>
                                <EditIcon/>
                            </IconButton>
                            <IconButton onClick={() => handleDeleteClick(user)}>
                                <CloseIcon />
                            </IconButton>
                        </TableCell>
                    </TableRow>
                ))}
            </TableBody>
            <UserDialog
                key={`userEditDialog${dialogKey}`}
                open={dialogOpen}
                user={dialogUser}
                users={props.users}
                onCancel={handleDialogCancel}
                onSubmit={handleDialogSubmit}
            />
        </Table>
    );
};

const UserDialog = (props) => {
    const { t } = useTranslation();
    const [baseUrl, setBaseUrl] = useState("");
    const [username, setUsername] = useState("");
    const [password, setPassword] = useState("");
    const fullScreen = useMediaQuery(theme.breakpoints.down('sm'));
    const editMode = props.user !== null;
    const addButtonEnabled = (() => {
        if (editMode) {
            return username.length > 0 && password.length > 0;
        }
        const baseUrlExists = props.users?.map(user => user.baseUrl).includes(baseUrl);
        return !baseUrlExists && username.length > 0 && password.length > 0;
    })();
    const handleSubmit = async () => {
        props.onSubmit({
            baseUrl: baseUrl,
            username: username,
            password: password
        })
    };
    useEffect(() => {
        if (editMode) {
            setBaseUrl(props.user.baseUrl);
            setUsername(props.user.username);
            setPassword(props.user.password);
        }
    }, [editMode, props.user]);
    return (
        <Dialog open={props.open} onClose={props.onCancel} fullScreen={fullScreen}>
            <DialogTitle>{editMode ? t("prefs_users_dialog_title_edit") : t("prefs_users_dialog_title_add")}</DialogTitle>
            <DialogContent>
                {!editMode && <TextField
                    autoFocus
                    margin="dense"
                    id="baseUrl"
                    label={t("prefs_users_dialog_base_url_label")}
                    value={baseUrl}
                    onChange={ev => setBaseUrl(ev.target.value)}
                    type="url"
                    fullWidth
                    variant="standard"
                />}
                <TextField
                    autoFocus={editMode}
                    margin="dense"
                    id="username"
                    label={t("prefs_users_dialog_username_label")}
                    value={username}
                    onChange={ev => setUsername(ev.target.value)}
                    type="text"
                    fullWidth
                    variant="standard"
                />
                <TextField
                    margin="dense"
                    id="password"
                    label={t("prefs_users_dialog_password_label")}
                    type="password"
                    value={password}
                    onChange={ev => setPassword(ev.target.value)}
                    fullWidth
                    variant="standard"
                />
            </DialogContent>
            <DialogActions>
                <Button onClick={props.onCancel}>{t("prefs_users_dialog_button_cancel")}</Button>
                <Button onClick={handleSubmit} disabled={!addButtonEnabled}>{editMode ? t("prefs_users_dialog_button_save") : t("prefs_users_dialog_button_add")}</Button>
            </DialogActions>
        </Dialog>
    );
};

const Appearance = () => {
    const { t } = useTranslation();
    return (
        <Card sx={{p: 3}}>
            <Typography variant="h5">
                {t("prefs_appearance_title")}
            </Typography>
            <PrefGroup>
                <Language/>
            </PrefGroup>
        </Card>
    );
};

const Language = () => {
    const { t, i18n } = useTranslation();
    const randomFlags = shuffle(["🇬🇧", "🇺🇸", "🇧🇬", "🇩🇪", "🇮🇩", "🇯🇵", "🇹🇷"]).slice(0, 3);
    const title = t("prefs_appearance_language_title") + " " + randomFlags.join(" ");

    // Remember: Flags are not languages. Don't put flags next to the language in the list.
    // Languages names from: https://www.omniglot.com/language/names.htm

    return (
        <Pref title={title}>
            <FormControl fullWidth variant="standard" sx={{ m: 1 }}>
                <Select value={i18n.language} onChange={(ev) => i18n.changeLanguage(ev.target.value)}>
                    <MenuItem value="en">English</MenuItem>
                    <MenuItem value="bg">Български</MenuItem>
                    <MenuItem value="de">Deutsch</MenuItem>
                    <MenuItem value="id">Bahasa Indonesia</MenuItem>
                    <MenuItem value="ja">日本語</MenuItem>
                    <MenuItem value="tr">Türkçe</MenuItem>
                </Select>
            </FormControl>
        </Pref>
    )
};

export default Preferences;
