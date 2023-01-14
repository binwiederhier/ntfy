import * as React from 'react';
import Dialog from '@mui/material/Dialog';
import DialogContent from '@mui/material/DialogContent';
import DialogTitle from '@mui/material/DialogTitle';
import {CardActionArea, CardContent, useMediaQuery} from "@mui/material";
import theme from "./theme";
import DialogFooter from "./DialogFooter";
import Button from "@mui/material/Button";
import accountApi, {TopicReservedError, UnauthorizedError} from "../app/AccountApi";
import session from "../app/Session";
import routes from "./routes";
import {useContext, useState} from "react";
import Card from "@mui/material/Card";
import Typography from "@mui/material/Typography";
import {AccountContext} from "./App";

const UpgradeDialog = (props) => {
    const { account } = useContext(AccountContext);
    const fullScreen = useMediaQuery(theme.breakpoints.down('sm'));
    const [selected, setSelected] = useState(account?.tier?.code || null);
    const [errorText, setErrorText] = useState("");

    const handleCheckout = async () => {
        try {
            const response = await accountApi.createCheckoutSession(selected);
            if (response.redirect_url) {
                window.location.href = response.redirect_url;
            } else {
                await accountApi.sync();
            }

        } catch (e) {
            console.log(`[UpgradeDialog] Error creating checkout session`, e);
            if ((e instanceof UnauthorizedError)) {
                session.resetAndRedirect(routes.login);
            }
            // FIXME show error
        }
    }

    return (
        <Dialog open={props.open} onClose={props.onCancel} maxWidth="md" fullScreen={fullScreen}>
            <DialogTitle>Upgrade to Pro</DialogTitle>
            <DialogContent>
                <div style={{
                    display: "flex",
                    flexDirection: "row"
                }}>
                    <TierCard code={null} name={"Free"} selected={selected === null} onClick={() => setSelected(null)}/>
                    <TierCard code="starter" name={"Starter"} selected={selected === "starter"} onClick={() => setSelected("starter")}/>
                    <TierCard code="pro" name={"Pro"} selected={selected === "pro"} onClick={() => setSelected("pro")}/>
                    <TierCard code="business" name={"Business"} selected={selected === "business"} onClick={() => setSelected("business")}/>
                </div>
            </DialogContent>
            <DialogFooter status={errorText}>
                <Button onClick={handleCheckout}>Checkout</Button>
            </DialogFooter>
        </Dialog>
    );
};

const TierCard = (props) => {
    const cardStyle = (props.selected) ? {
        border: "1px solid red",

    } : {};
    return (
        <Card sx={{ m: 1, maxWidth: 345 }}>
            <CardActionArea>
                <CardContent sx={{...cardStyle}} onClick={props.onClick}>
                    <Typography gutterBottom variant="h5" component="div">
                        {props.name}
                    </Typography>
                    <Typography variant="body2" color="text.secondary">
                        Lizards are a widespread group of squamate reptiles, with over 6,000
                        species, ranging across all continents except Antarctica
                    </Typography>
                </CardContent>
            </CardActionArea>
        </Card>
    );
}

export default UpgradeDialog;
