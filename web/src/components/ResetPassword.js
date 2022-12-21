import * as React from 'react';
import TextField from "@mui/material/TextField";
import Button from "@mui/material/Button";
import Box from "@mui/material/Box";
import routes from "./routes";
import Typography from "@mui/material/Typography";
import {NavLink} from "react-router-dom";
import AvatarBox from "./AvatarBox";

const ResetPassword = () => {
    const handleSubmit = async (event) => {
        //
    };

    return (
        <AvatarBox>
            <Typography sx={{ typography: 'h6' }}>
                Reset password
            </Typography>
            <Box component="form" onSubmit={handleSubmit} noValidate sx={{mt: 1, maxWidth: 400}}>
                <TextField
                    margin="dense"
                    required
                    fullWidth
                    id="email"
                    label="Email"
                    name="email"
                    autoFocus
                />
                <Button
                    type="submit"
                    fullWidth
                    variant="contained"
                    sx={{mt: 2, mb: 2}}
                >
                    Reset password
                </Button>
            </Box>
            <Typography sx={{mb: 4}}>
                <NavLink to={routes.login} variant="body1">
                    &lt; Return to sign in
                </NavLink>
            </Typography>
        </AvatarBox>
    );
}

export default ResetPassword;
