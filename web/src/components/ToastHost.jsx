import * as React from "react";
import { useEffect, useState } from "react";
import { Alert, Snackbar } from "@mui/material";
import toast from "../app/Toast";

/**
 * Renders transient app notices (see app/Toast.js) as a bottom-center Snackbar. Mounted once in the
 * app Layout so any part of the app can surface a message via toast.show(...) without prop drilling.
 */
const ToastHost = () => {
  const [message, setMessage] = useState("");

  useEffect(() => {
    const listener = (m) => setMessage(m);
    toast.registerListener(listener);
    return () => toast.resetListener(listener);
  }, []);

  return (
    <Snackbar
      open={!!message}
      autoHideDuration={6000}
      onClose={() => setMessage("")}
      anchorOrigin={{ vertical: "bottom", horizontal: "center" }}
    >
      <Alert severity="info" variant="filled" onClose={() => setMessage("")}>
        {message}
      </Alert>
    </Snackbar>
  );
};

export default ToastHost;
