import * as React from 'react';
import {NavLink} from "react-router-dom";
import routes from "./routes";
import CloudOutlinedIcon from '@mui/icons-material/CloudOutlined';
import GitHubIcon from '@mui/icons-material/GitHub';
import {Link} from "@mui/material";

const SiteLayout = (props) => {
    return (
        <div id="site">
            <nav id="header">
                <div id="headerBox">
                    <img id="logo" src="static/img/ntfy.png" alt="logo"/>
                    <div id="name">ntfy</div>
                    <ol id="menu">
                        <li><NavLink to={routes.home}>Features</NavLink></li>
                        <li><NavLink to={routes.pricing}>Pricing</NavLink></li>
                        <li><NavLink to="/docs" reloadDocument={true}>Docs</NavLink></li>
                        <li><Link href="https://github.com/binwiederhier/ntfy" reloadDocument={true}><GitHubIcon fontSize="small" sx={{verticalAlign: "text-top"}}/> Forever open</Link></li>
                        <li><NavLink to={routes.app}><CloudOutlinedIcon fontSize="small" sx={{verticalAlign: "text-top"}}/> Open app</NavLink></li>
                    </ol>
                </div>
            </nav>
            <div id="main">
                {props.children}
            </div>
        </div>
    );
};

export default SiteLayout;
