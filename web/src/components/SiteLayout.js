import * as React from 'react';
import {NavLink} from "react-router-dom";
import routes from "./routes";
import session from "../app/Session";

const SiteLayout = (props) => {
    return (
        <div id="site">
            <nav id="header">
                <div id="headerBox">
                    <img id="logo" src="static/img/ntfy.png" alt="logo"/>
                    <div id="name">ntfy</div>
                    <ol>
                        <li><NavLink to={routes.home} activeStyle>Features</NavLink></li>
                        <li><NavLink to={routes.pricing} activeStyle>Pricing</NavLink></li>
                        <li><NavLink to="/docs" reloadDocument={true} activeStyle>Docs</NavLink></li>
                        {config.enableSignup && !session.exists() && <li><NavLink to={routes.signup} activeStyle>Sign up</NavLink></li>}
                        {config.enableLogin && !session.exists() && <li><NavLink to={routes.login} activeStyle>Login</NavLink></li>}
                        <li><NavLink to={routes.app} activeStyle>Open app</NavLink></li>
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
