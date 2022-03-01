import * as React from 'react';
import {useState} from 'react';
import {FormControl, Select, Stack, Table, TableBody, TableCell, TableHead, TableRow} from "@mui/material";
import Typography from "@mui/material/Typography";
import Paper from "@mui/material/Paper";
import repository from "../app/Repository";
import {Paragraph} from "./styles";
import EditIcon from '@mui/icons-material/Edit';
import CloseIcon from "@mui/icons-material/Close";
import IconButton from "@mui/material/IconButton";
import Container from "@mui/material/Container";
import TextField from "@mui/material/TextField";
import MenuItem from "@mui/material/MenuItem";

const Preferences = (props) => {
    return (
        <Container maxWidth="lg" sx={{marginTop: 3, marginBottom: 3}}>
            <Stack spacing={3}>
                <Notifications/>
                <DefaultServer/>
                <Users/>
            </Stack>
        </Container>
    );
};

const Notifications = (props) => {
    return (
        <Paper sx={{p: 3}}>
            <Typography variant="h5">
                Notifications
            </Typography>
            <PrefGroup>
                <MinPriority/>
                <DeleteAfter/>
            </PrefGroup>
        </Paper>
    );
};

const MinPriority = () => {
    const [minPriority, setMinPriority] = useState(repository.getMinPriority());
    const handleChange = (ev) => {
        setMinPriority(ev.target.value);
        repository.setMinPriority(ev.target.value);
    }
    return (
        <Pref title="Minimum priority">
            <FormControl fullWidth variant="standard" sx={{ m: 1 }}>
                <Select value={minPriority} onChange={handleChange}>
                    <MenuItem value={1}><em>Any priority</em></MenuItem>
                    <MenuItem value={2}>Low priority and higher</MenuItem>
                    <MenuItem value={3}>Default priority and higher</MenuItem>
                    <MenuItem value={4}>High priority and higher</MenuItem>
                    <MenuItem value={5}>Only max priority</MenuItem>
                </Select>
            </FormControl>
        </Pref>
    )
};

const DeleteAfter = () => {
    const [deleteAfter, setDeleteAfter] = useState(repository.getDeleteAfter());
    const handleChange = (ev) => {
        setDeleteAfter(ev.target.value);
        repository.setDeleteAfter(ev.target.value);
    }
    return (
        <Pref title="Minimum priority">
            <FormControl fullWidth variant="standard" sx={{ m: 1 }}>
                <Select value={deleteAfter} onChange={handleChange}>
                    <MenuItem value={0}>Never</MenuItem>
                    <MenuItem value={10800}>After three hour</MenuItem>
                    <MenuItem value={86400}>After one day</MenuItem>
                    <MenuItem value={604800}>After one week</MenuItem>
                    <MenuItem value={2592000}>After one month</MenuItem>
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

const DefaultServer = (props) => {
    return (
        <Paper sx={{p: 3}}>
            <Typography variant="h5">
                Default server
            </Typography>
            <Paragraph>
                This server is used as a default when adding new topics.
            </Paragraph>
            <TextField
                margin="dense"
                id="defaultBaseUrl"
                placeholder="https://ntfy.sh"
                type="text"
                fullWidth
                variant="standard"
            />
        </Paper>
    );
};

const Users = (props) => {
    return (
        <Paper sx={{p: 3}}>
            <Typography variant="h5">
                Manage users
            </Typography>
            <Paragraph>
                You may manage users for your protected topics here. Please note that since this is a client
                application only, username and password are stored in the browser's local storage.
            </Paragraph>
            <UserTable/>
        </Paper>
    );
};

const UserTable = () => {
    const users = repository.loadUsers();
    return (
            <Table size="small">
                <TableHead>
                    <TableRow>
                        <TableCell>User</TableCell>
                        <TableCell>Service URL</TableCell>
                        <TableCell/>
                    </TableRow>
                </TableHead>
                <TableBody>
                    {users.map((user, i) => (
                        <TableRow
                            key={i}
                            sx={{ '&:last-child td, &:last-child th': { border: 0 } }}
                        >
                            <TableCell component="th" scope="row">{user.username}</TableCell>
                            <TableCell>{user.baseUrl}</TableCell>
                            <TableCell align="right">
                                <IconButton>
                                    <EditIcon/>
                                </IconButton>
                                <IconButton>
                                    <CloseIcon />
                                </IconButton>
                            </TableCell>
                        </TableRow>
                    ))}
                </TableBody>
            </Table>

    );
}

export default Preferences;
