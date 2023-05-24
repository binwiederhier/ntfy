import * as React from "react";
import { Lock, Public } from "@mui/icons-material";
import { Box } from "@mui/material";

export const PermissionReadWrite = React.forwardRef((props, ref) => <PermissionInternal icon={Public} ref={ref} {...props} />);

export const PermissionDenyAll = React.forwardRef((props, ref) => <PermissionInternal icon={Lock} ref={ref} {...props} />);

export const PermissionRead = React.forwardRef((props, ref) => <PermissionInternal icon={Public} text="R" ref={ref} {...props} />);

export const PermissionWrite = React.forwardRef((props, ref) => <PermissionInternal icon={Public} text="W" ref={ref} {...props} />);

const PermissionInternal = React.forwardRef((props, ref) => {
  const size = props.size ?? "medium";
  const Icon = props.icon;
  return (
    <Box
      ref={ref}
      {...props}
      style={{
        position: "relative",
        display: "inline-flex",
        verticalAlign: "middle",
        height: "24px",
      }}
    >
      <Icon fontSize={size} sx={{ color: "gray" }} />
      {props.text && (
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
            marginTop: "3px",
          }}
        >
          {props.text}
        </Box>
      )}
    </Box>
  );
});
