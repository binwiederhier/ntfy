import {Menu} from "@mui/material";
import * as React from "react";

const PopupMenu = (props) => {
    const horizontal = props.horizontal ?? "left";
    const arrow = (horizontal === "right") ? { right: 19 } : { left: 19 };
    return (
        <Menu
            anchorEl={props.anchorEl}
            open={props.open}
            onClose={props.onClose}
            onClick={props.onClose}
            PaperProps={{
                elevation: 0,
                sx: {
                    overflow: 'visible',
                    filter: 'drop-shadow(0px 2px 8px rgba(0,0,0,0.32))',
                    mt: 1.5,
                    '& .MuiAvatar-root': {
                        width: 32,
                        height: 32,
                        ml: -0.5,
                        mr: 1,
                    },
                    '&:before': {
                        content: '""',
                        display: 'block',
                        position: 'absolute',
                        top: 0,
                        width: 10,
                        height: 10,
                        bgcolor: 'background.paper',
                        transform: 'translateY(-50%) rotate(45deg)',
                        zIndex: 0,
                        ...arrow
                    },
                },
            }}
            transformOrigin={{ horizontal: horizontal, vertical: 'top' }}
            anchorOrigin={{ horizontal: horizontal, vertical: 'bottom' }}
        >
            {props.children}
        </Menu>
    );
};

export default PopupMenu;
