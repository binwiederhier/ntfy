import * as React from 'react';
import {Lock, Public} from "@mui/icons-material";
import Box from "@mui/material/Box";


export const PermissionReadWrite = React.forwardRef((props, ref) => {
    const size = props.size ?? "medium";
    return <Public fontSize={size} ref={ref} {...props}/>;
});

export const PermissionDenyAll = React.forwardRef((props, ref) => {
    const size = props.size ?? "medium";
    return <Lock fontSize={size} ref={ref} {...props}/>;
});

export const PermissionRead = React.forwardRef((props, ref) => {
    return <PermissionReadOrWrite text="R" ref={ref} {...props}/>;
});

export const PermissionWrite = React.forwardRef((props, ref) => {
    return <PermissionReadOrWrite text="W" ref={ref} {...props}/>;
});

const PermissionReadOrWrite = React.forwardRef((props, ref) => {
    const size = props.size ?? "medium";
    return (
        <div ref={ref} {...props} style={{position: "relative", display: "inline-flex", verticalAlign: "middle", height: "24px"}}>
            <Public fontSize={size}/>
            <Box
                sx={{
                    position: "absolute",
                    right: "-6px",
                    bottom: "5px",
                    fontSize: 10,
                    fontWeight: 600,
                    color: "gray",
                    width: "8px",
                    height: "8px",
                    marginTop: "3px"
                }}
            >
                {props.text}
            </Box>
        </div>
    );
});
