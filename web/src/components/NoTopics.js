import {Link} from "@mui/material";
import Typography from "@mui/material/Typography";
import * as React from "react";
import {Paragraph, VerticallyCenteredContainer} from "./styles";

const NoTopics = (props) => {
    return (
        <VerticallyCenteredContainer maxWidth="xs">
            <Typography variant="h5" align="center" sx={{ paddingBottom: 1 }}>
                <img src="static/img/ntfy-outline.svg" height="64" width="64" alt="No topics"/><br />
                It looks like you don't have any subscriptions yet.
            </Typography>
            <Paragraph>
                Click the "Add subscription" link to create or subscribe to a topic. After that, you can send messages
                via PUT or POST and you'll receive notifications here.
            </Paragraph>
            <Paragraph>
                For more information, check out the <Link href="https://ntfy.sh" target="_blank" rel="noopener">website</Link> or
                {" "}<Link href="https://ntfy.sh/docs" target="_blank" rel="noopener">documentation</Link>.
            </Paragraph>
        </VerticallyCenteredContainer>
    );
};

export default NoTopics;
