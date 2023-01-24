import * as React from 'react';
import {useState} from 'react';
import Button from '@mui/material/Button';
import TextField from '@mui/material/TextField';
import Dialog from '@mui/material/Dialog';
import DialogContent from '@mui/material/DialogContent';
import DialogContentText from '@mui/material/DialogContentText';
import DialogTitle from '@mui/material/DialogTitle';
import {Checkbox, FormControl, FormControlLabel, Select, useMediaQuery} from "@mui/material";
import theme from "./theme";
import subscriptionManager from "../app/SubscriptionManager";
import DialogFooter from "./DialogFooter";
import {useTranslation} from "react-i18next";
import accountApi, {UnauthorizedError} from "../app/AccountApi";
import session from "../app/Session";
import routes from "./routes";
import MenuItem from "@mui/material/MenuItem";
import ListItemIcon from "@mui/material/ListItemIcon";
import LockIcon from "@mui/icons-material/Lock";
import ListItemText from "@mui/material/ListItemText";
import {Public, PublicOff} from "@mui/icons-material";
import {PermissionDenyAll, PermissionRead, PermissionReadWrite, PermissionWrite} from "./ReserveIcons";

const ReserveTopicSelect = (props) => {
    const { t } = useTranslation();
    const sx = props.sx || {};
    return (
        <FormControl fullWidth variant="standard" sx={sx}>
            <Select
                value={props.value}
                onChange={(ev) => props.onChange(ev.target.value)}
                aria-label={t("prefs_reservations_dialog_access_label")}
                sx={{
                    "& .MuiSelect-select": {
                        display: 'flex',
                        alignItems: 'center',
                        paddingTop: "4px",
                        paddingBottom: "4px",
                    }
                }}
            >
                <MenuItem value="deny-all">
                    <ListItemIcon><PermissionDenyAll/></ListItemIcon>
                    <ListItemText primary={t("prefs_reservations_table_everyone_deny_all")}/>
                </MenuItem>
                <MenuItem value="read-only">
                    <ListItemIcon><PermissionRead/></ListItemIcon>
                    <ListItemText primary={t("prefs_reservations_table_everyone_read_only")}/>
                </MenuItem>
                <MenuItem value="write-only">
                    <ListItemIcon><PermissionWrite/></ListItemIcon>
                    <ListItemText primary={t("prefs_reservations_table_everyone_write_only")}/>
                </MenuItem>
                <MenuItem value="read-write">
                    <ListItemIcon><PermissionReadWrite/></ListItemIcon>
                    <ListItemText primary={t("prefs_reservations_table_everyone_read_write")}/>
                </MenuItem>
            </Select>
        </FormControl>
    );
};

export default ReserveTopicSelect;
