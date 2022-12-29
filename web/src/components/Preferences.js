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
import {playSound, shuffle, sounds, validTopic, validUrl} from "../app/utils";
import {useTranslation} from "react-i18next";
import session from "../app/Session";
import routes from "./routes";
import accountApi, {UnauthorizedError} from "../app/AccountApi";

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
        <Card sx={{p: 3}} aria-label={t("prefs_notifications_title")}>
            <Typography variant="h5" sx={{marginBottom: 2}}>
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
    const labelId = "prefSound";
    const sound = useLiveQuery(async () => prefs.sound());
    const handleChange = async (ev) => {
        await prefs.setSound(ev.target.value);
        await maybeUpdateAccountSettings({
            notification: {
                sound: ev.target.value
            }
        });
    }
    if (!sound) {
        return null; // While loading
    }
    let description;
    if (sound === "none") {
        description = t("prefs_notifications_sound_description_none");
    } else {
        description = t("prefs_notifications_sound_description_some", { sound: sounds[sound].label });
    }
    return (
        <Pref labelId={labelId} title={t("prefs_notifications_sound_title")} description={description}>
            <div style={{ display: 'flex', width: '100%' }}>
                <FormControl fullWidth variant="standard" sx={{ margin: 1 }}>
                    <Select value={sound} onChange={handleChange} aria-labelledby={labelId}>
                        <MenuItem value={"none"}>{t("prefs_notifications_sound_no_sound")}</MenuItem>
                        {Object.entries(sounds).map(s => <MenuItem key={s[0]} value={s[0]}>{s[1].label}</MenuItem>)}
                    </Select>
                </FormControl>
                <IconButton onClick={() => playSound(sound)} disabled={sound === "none"} aria-label={t("prefs_notifications_sound_play")}>
                    <PlayArrowIcon />
                </IconButton>
            </div>
        </Pref>
    )
};

const MinPriority = () => {
    const { t } = useTranslation();
    const labelId = "prefMinPriority";
    const minPriority = useLiveQuery(async () => prefs.minPriority());
    const handleChange = async (ev) => {
        await prefs.setMinPriority(ev.target.value);
        await maybeUpdateAccountSettings({
            notification: {
                min_priority: ev.target.value
            }
        });
    }
    if (!minPriority) {
        return null; // While loading
    }
    const priorities = {
        1: t("priority_min"),
        2: t("priority_low"),
        3: t("priority_default"),
        4: t("priority_high"),
        5: t("priority_max")
    }
    let description;
    if (minPriority === 1) {
        description = t("prefs_notifications_min_priority_description_any");
    } else if (minPriority === 5) {
        description = t("prefs_notifications_min_priority_description_max");
    } else {
        description = t("prefs_notifications_min_priority_description_x_or_higher", {
            number: minPriority,
            name: priorities[minPriority]
        });
    }
    return (
        <Pref labelId={labelId} title={t("prefs_notifications_min_priority_title")} description={description}>
            <FormControl fullWidth variant="standard" sx={{ m: 1 }}>
                <Select value={minPriority} onChange={handleChange} aria-labelledby={labelId}>
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
    const labelId = "prefDeleteAfter";
    const deleteAfter = useLiveQuery(async () => prefs.deleteAfter());
    const handleChange = async (ev) => {
        await prefs.setDeleteAfter(ev.target.value);
        await maybeUpdateAccountSettings({
            notification: {
                delete_after: ev.target.value
            }
        });
    }
    if (deleteAfter === null || deleteAfter === undefined) { // !deleteAfter will not work with "0"
        return null; // While loading
    }
    const description = (() => {
        switch (deleteAfter) {
            case 0: return t("prefs_notifications_delete_after_never_description");
            case 10800: return t("prefs_notifications_delete_after_three_hours_description");
            case 86400: return t("prefs_notifications_delete_after_one_day_description");
            case 604800: return t("prefs_notifications_delete_after_one_week_description");
            case 2592000: return t("prefs_notifications_delete_after_one_month_description");
        }
    })();
    return (
        <Pref labelId={labelId} title={t("prefs_notifications_delete_after_title")} description={description}>
            <FormControl fullWidth variant="standard" sx={{ m: 1 }}>
                <Select value={deleteAfter} onChange={handleChange} aria-labelledby={labelId}>
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
        <div role="table">
            {props.children}
        </div>
    )
};

const Pref = (props) => {
    return (
        <div
            role="row"
            style={{
                display: "flex",
                flexDirection: "row",
                marginTop: "10px",
                marginBottom: "20px",
            }}
        >
            <div
                role="cell"
                id={props.labelId}
                aria-label={props.title}
                style={{
                    flex: '1 0 40%',
                    display: 'flex',
                    flexDirection: 'column',
                    justifyContent: 'center',
                    paddingRight: '30px'
                }}
            >
                <div><b>{props.title}</b></div>
                {props.description && <div><em>{props.description}</em></div>}
            </div>
            <div
                role="cell"
                style={{
                    flex: '1 0 calc(60% - 50px)',
                    display: 'flex',
                    flexDirection: 'column',
                    justifyContent: 'center'
                }}
            >
                {props.children}
            </div>
        </div>
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
        <Card sx={{ padding: 1 }} aria-label={t("prefs_users_title")}>
            <CardContent sx={{ paddingBottom: 1 }}>
                <Typography variant="h5" sx={{marginBottom: 2}}>
                    {t("prefs_users_title")}
                </Typography>
                <Paragraph>
                    {t("prefs_users_description")}
                    {session.exists() && <>{" " + t("prefs_users_description_no_sync")}</>}
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
        <Table size="small" aria-label={t("prefs_users_table")}>
            <TableHead>
                <TableRow>
                    <TableCell sx={{paddingLeft: 0}}>{t("prefs_users_table_user_header")}</TableCell>
                    <TableCell>{t("prefs_users_table_base_url_header")}</TableCell>
                    <TableCell/>
                </TableRow>
            </TableHead>
            <TableBody>
                {props.users?.map(user => (
                    <TableRow
                        key={user.baseUrl}
                        sx={{'&:last-child td, &:last-child th': {border: 0}}}
                    >
                        <TableCell component="th" scope="row" sx={{paddingLeft: 0}}
                                   aria-label={t("prefs_users_table_user_header")}>{user.username}</TableCell>
                        <TableCell aria-label={t("prefs_users_table_base_url_header")}>{user.baseUrl}</TableCell>
                        <TableCell align="right">
                            {user.baseUrl !== config.baseUrl &&
                                <>
                                    <IconButton onClick={() => handleEditClick(user)}
                                                aria-label={t("prefs_users_edit_button")}>
                                        <EditIcon/>
                                    </IconButton>
                                    <IconButton onClick={() => handleDeleteClick(user)}
                                                aria-label={t("prefs_users_delete_button")}>
                                        <CloseIcon/>
                                    </IconButton>
                                </>
                            }
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
        const baseUrlValid = validUrl(baseUrl);
        const baseUrlExists = props.users?.map(user => user.baseUrl).includes(baseUrl);
        return baseUrlValid
            && !baseUrlExists
            && username.length > 0
            && password.length > 0;
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
                    aria-label={t("prefs_users_dialog_base_url_label")}
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
                    aria-label={t("prefs_users_dialog_username_label")}
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
                    aria-label={t("prefs_users_dialog_password_label")}
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
        <Card sx={{p: 3}} aria-label={t("prefs_appearance_title")}>
            <Typography variant="h5" sx={{marginBottom: 2}}>
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
    const labelId = "prefLanguage";
    const randomFlags = shuffle(["🇬🇧", "🇺🇸", "🇪🇸", "🇫🇷", "🇧🇬", "🇨🇿", "🇩🇪", "🇵🇱", "🇺🇦", "🇨🇳", "🇮🇹", "🇭🇺", "🇧🇷", "🇳🇱", "🇮🇩", "🇯🇵", "🇷🇺", "🇹🇷"]).slice(0, 3);
    const title = t("prefs_appearance_language_title") + " " + randomFlags.join(" ");
    const lang = i18n.language ?? "en";

    const handleChange = async (ev) => {
        await i18n.changeLanguage(ev.target.value);
        await maybeUpdateAccountSettings({
            language: ev.target.value
        });
    };

    // Remember: Flags are not languages. Don't put flags next to the language in the list.
    // Languages names from: https://www.omniglot.com/language/names.htm
    // Better: Sidebar in Wikipedia: https://en.wikipedia.org/wiki/Bokm%C3%A5l

    return (
        <Pref labelId={labelId} title={title}>
            <FormControl fullWidth variant="standard" sx={{ m: 1 }}>
                <Select value={lang} onChange={handleChange} aria-labelledby={labelId}>
                    <MenuItem value="en">English</MenuItem>
                    <MenuItem value="id">Bahasa Indonesia</MenuItem>
                    <MenuItem value="bg">Български</MenuItem>
                    <MenuItem value="cs">Čeština</MenuItem>
                    <MenuItem value="zh_Hans">中文</MenuItem>
                    <MenuItem value="de">Deutsch</MenuItem>
                    <MenuItem value="es">Español</MenuItem>
                    <MenuItem value="fr">Français</MenuItem>
                    <MenuItem value="it">Italiano</MenuItem>
                    <MenuItem value="hu">Magyar</MenuItem>
                    <MenuItem value="ko">한국어</MenuItem>
                    <MenuItem value="ja">日本語</MenuItem>
                    <MenuItem value="nl">Nederlands</MenuItem>
                    <MenuItem value="nb_NO">Norsk bokmål</MenuItem>
                    <MenuItem value="uk">Українська</MenuItem>
                    <MenuItem value="pt_BR">Português (Brasil)</MenuItem>
                    <MenuItem value="pl">Polski</MenuItem>
                    <MenuItem value="ru">Русский</MenuItem>
                    <MenuItem value="tr">Türkçe</MenuItem>
                </Select>
            </FormControl>
        </Pref>
    )
};

const maybeUpdateAccountSettings = async (payload) => {
    if (!session.exists()) {
        return;
    }
    try {
        await accountApi.updateSettings(payload);
    } catch (e) {
        console.log(`[Preferences] Error updating account settings`, e);
        if ((e instanceof UnauthorizedError)) {
            session.resetAndRedirect(routes.login);
        }
    }
};

export default Preferences;
