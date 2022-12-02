import * as React from 'react';
import {useEffect, useState} from 'react';
import {
    CardActions,
    CardContent,
    FormControl, Link,
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
import {playSound, shuffle, sounds, validUrl} from "../app/utils";
import {useTranslation} from "react-i18next";

const Home = () => {
    return (
        <Container maxWidth="md" sx={{marginTop: 3, marginBottom: 3}}>
            <Stack spacing={3}>
               This is the landing page
                <Link href="/login">Login</Link>
            </Stack>
        </Container>
    );
};

export default Home;
