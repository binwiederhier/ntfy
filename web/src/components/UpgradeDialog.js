import * as React from 'react';
import {useState} from 'react';
import Button from '@mui/material/Button';
import TextField from '@mui/material/TextField';
import Dialog from '@mui/material/Dialog';
import DialogContent from '@mui/material/DialogContent';
import DialogContentText from '@mui/material/DialogContentText';
import DialogTitle from '@mui/material/DialogTitle';
import {Autocomplete, Checkbox, FormControlLabel, FormGroup, useMediaQuery} from "@mui/material";
import theme from "./theme";
import api from "../app/Api";
import {randomAlphanumericString, topicUrl, validTopic, validUrl} from "../app/utils";
import userManager from "../app/UserManager";
import subscriptionManager from "../app/SubscriptionManager";
import poller from "../app/Poller";
import DialogFooter from "./DialogFooter";
import {useTranslation} from "react-i18next";
import session from "../app/Session";
import routes from "./routes";
import accountApi, {TopicReservedError, UnauthorizedError} from "../app/AccountApi";
import ReserveTopicSelect from "./ReserveTopicSelect";
import {useOutletContext} from "react-router-dom";

const UpgradeDialog = (props) => {
    const fullScreen = useMediaQuery(theme.breakpoints.down('sm'));

    const handleSuccess = async () => {
        // TODO
    }

    return (
        <Dialog open={props.open} onClose={props.onCancel} fullScreen={fullScreen}>
            <DialogTitle>Upgrade to Pro</DialogTitle>
            <DialogContent>
                Content
            </DialogContent>
            <DialogFooter>
                Footer
            </DialogFooter>
        </Dialog>
    );
};

export default UpgradeDialog;
