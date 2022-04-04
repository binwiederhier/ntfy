import * as React from "react";
import Box from "@mui/material/Box";
import fileDocument from "../img/file-document.svg";
import fileImage from "../img/file-image.svg";
import fileVideo from "../img/file-video.svg";
import fileAudio from "../img/file-audio.svg";
import fileApp from "../img/file-app.svg";

const AttachmentIcon = (props) => {
    const type = props.type;
    let imageFile;
    if (!type) {
        imageFile = fileDocument;
    } else if (type.startsWith('image/')) {
        imageFile = fileImage;
    } else if (type.startsWith('video/')) {
        imageFile = fileVideo;
    } else if (type.startsWith('audio/')) {
        imageFile = fileAudio;
    } else if (type === "application/vnd.android.package-archive") {
        imageFile = fileApp;
    } else {
        imageFile = fileDocument;
    }
    return (
        <Box
            component="img"
            src={imageFile}
            loading="lazy"
            sx={{
                width: '28px',
                height: '28px'
            }}
        />
    );
}

export default AttachmentIcon;
