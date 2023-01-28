import * as React from 'react';
import {FormControl, Select} from "@mui/material";
import {useTranslation} from "react-i18next";
import MenuItem from "@mui/material/MenuItem";
import ListItemIcon from "@mui/material/ListItemIcon";
import ListItemText from "@mui/material/ListItemText";
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
