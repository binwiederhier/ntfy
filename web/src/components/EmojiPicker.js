import * as React from 'react';
import Popover from '@mui/material/Popover';
import Typography from '@mui/material/Typography';
import {rawEmojis} from '../app/emojis';
import Box from "@mui/material/Box";

const emojisByCategory = {};
rawEmojis.forEach(emoji => {
    if (!emojisByCategory[emoji.category]) {
        emojisByCategory[emoji.category] = [];
    }
    emojisByCategory[emoji.category].push(emoji);
});

const EmojiPicker = (props) => {
    const open = Boolean(props.anchorEl);

    return (
        <>
            <Popover
                open={open}
                anchorEl={props.anchorEl}
                onClose={props.onClose}
                anchorOrigin={{
                    vertical: 'bottom',
                    horizontal: 'left',
                }}
            >
                <Box sx={{ padding: 2, paddingRight: 0, width: "370px", maxHeight: "300px" }}>
                    {Object.keys(emojisByCategory).map(category =>
                        <Category title={category} emojis={emojisByCategory[category]} onPick={props.onEmojiPick}/>
                    )}
                </Box>
            </Popover>
        </>
    );
};

const Category = (props) => {
    return (
        <>
            <Typography variant="body2">{props.title}</Typography>
            <Box sx={{ display: "flex", flexWrap: "wrap", paddingRight: 0, marginBottom: 1 }}>
                {props.emojis.map(emoji => <Emoji emoji={emoji} onClick={() => props.onPick(emoji.aliases[0])}/>)}
            </Box>
        </>
    );
};

const Emoji = (props) => {
    const emoji = props.emoji;
    return (
        <div
            onClick={props.onClick}
            title={`${emoji.description} (${emoji.aliases[0]})`}
            style={{
                fontSize: "30px",
                width: "30px",
                height: "30px",
                marginTop: "8px",
                marginBottom: "8px",
                marginRight: "8px",
                lineHeight: "30px",
                cursor: "pointer"
            }}
        >
            {props.emoji.emoji}
        </div>
    );
};

export default EmojiPicker;
