import * as React from "react";
import Box from "@mui/material/Box";
import fileDocument from "../img/file-document.svg";
import fileImage from "../img/file-image.svg";
import fileVideo from "../img/file-video.svg";
import fileAudio from "../img/file-audio.svg";
import fileApp from "../img/file-app.svg";
import {useTranslation} from "react-i18next";

const AttachmentIcon = (props) => {
    const { t } = useTranslation();
    const type = props.type;
    let imageFile, imageLabel;
    if (!type) {
        imageFile = fileDocument;
        imageLabel = t("notifications_attachment_file_image");
    } else if (type.startsWith('image/')) {
        imageFile = fileImage;
        imageLabel = t("notifications_attachment_file_video");
    } else if (type.startsWith('video/')) {
        imageFile = fileVideo;
        imageLabel = t("notifications_attachment_file_video");
    } else if (type.startsWith('audio/')) {
        imageFile = fileAudio;
        imageLabel = t("notifications_attachment_file_audio");
    } else if (type === "application/vnd.android.package-archive") {
        imageFile = fileApp;
        imageLabel = t("notifications_attachment_file_app");
    } else {
        imageFile = fileDocument;
        imageLabel = t("notifications_attachment_file_document");
    }
    return (
        <Box
            component="img"
            src={imageFile}
            alt={imageLabel}
            loading="lazy"
            sx={{
                width: '28px',
                height: '28px'
            }}
        />
    );
}

export default AttachmentIcon;
