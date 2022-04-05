import * as React from 'react';
import {useRef, useState} from 'react';
import Typography from '@mui/material/Typography';
import {rawEmojis} from '../app/emojis';
import Box from "@mui/material/Box";
import TextField from "@mui/material/TextField";
import {ClickAwayListener, Fade, InputAdornment, styled} from "@mui/material";
import IconButton from "@mui/material/IconButton";
import {Close} from "@mui/icons-material";
import Popper from "@mui/material/Popper";
import {splitNoEmpty} from "../app/utils";

// Create emoji list by category and create a search base (string with all search words)
//
// This also filters emojis that are not supported by Desktop Chrome.
// This is a hack, but on Ubuntu 18.04, with Chrome 99, only Emoji <= 11 are supported.

const emojisByCategory = {};
const isDesktopChrome = /Chrome/.test(navigator.userAgent) && !/Mobile/.test(navigator.userAgent);
const maxSupportedVersionForDesktopChrome = 11;
rawEmojis.forEach(emoji => {
    if (!emojisByCategory[emoji.category]) {
        emojisByCategory[emoji.category] = [];
    }
    try {
        const unicodeVersion = parseFloat(emoji.unicode_version);
        const supportedEmoji = unicodeVersion <= maxSupportedVersionForDesktopChrome || !isDesktopChrome;
        if (supportedEmoji) {
            const searchBase = `${emoji.description.toLowerCase()} ${emoji.aliases.join(" ")} ${emoji.tags.join(" ")}`;
            const emojiWithSearchBase = { ...emoji, searchBase: searchBase };
            emojisByCategory[emoji.category].push(emojiWithSearchBase);
        }
    } catch (e) {
        // Nothing. Ignore.
    }
});

const EmojiPicker = (props) => {
    const open = Boolean(props.anchorEl);
    const [search, setSearch] = useState("");
    const searchRef = useRef(null);
    const searchFields = splitNoEmpty(search.toLowerCase(), " ");

    const handleSearchClear = () => {
        setSearch("");
        searchRef.current?.focus();
    };

    return (
        <Popper
            open={open}
            anchorEl={props.anchorEl}
            placement="bottom-start"
            sx={{ zIndex: 10005 }}
            transition
        >
            {({ TransitionProps }) => (
                <ClickAwayListener onClickAway={props.onClose}>
                    <Fade {...TransitionProps} timeout={350}>
                        <Box sx={{
                            boxShadow: 3,
                            padding: 2,
                            paddingRight: 0,
                            paddingBottom: 1,
                            width: "380px",
                            maxHeight: "300px",
                            backgroundColor: 'background.paper',
                            overflowY: "scroll"
                        }}>
                            <TextField
                                inputRef={searchRef}
                                margin="dense"
                                size="small"
                                placeholder="Search emoji"
                                value={search}
                                onChange={ev => setSearch(ev.target.value)}
                                type="text"
                                variant="standard"
                                fullWidth
                                sx={{ marginTop: 0, marginBottom: "12px", paddingRight: 2 }}
                                InputProps={{
                                    endAdornment:
                                        <InputAdornment position="end" sx={{ display: (search) ? '' : 'none' }}>
                                            <IconButton size="small" onClick={handleSearchClear} edge="end"><Close/></IconButton>
                                        </InputAdornment>
                                }}
                            />
                            <Box sx={{ display: "flex", flexWrap: "wrap", paddingRight: 0, marginTop: 1 }}>
                                {Object.keys(emojisByCategory).map(category =>
                                    <Category
                                        key={category}
                                        title={category}
                                        emojis={emojisByCategory[category]}
                                        search={searchFields}
                                        onPick={props.onEmojiPick}
                                    />
                                )}
                            </Box>
                        </Box>
                    </Fade>
                </ClickAwayListener>
            )}
        </Popper>
    );
};

const Category = (props) => {
    const showTitle = props.search.length === 0;
    return (
        <>
            {showTitle &&
                <Typography variant="body1" sx={{ width: "100%", marginBottom: 1 }}>
                    {props.title}
                </Typography>
            }
            {props.emojis.map(emoji =>
                <Emoji
                    key={emoji.aliases[0]}
                    emoji={emoji}
                    search={props.search}
                    onClick={() => props.onPick(emoji.aliases[0])}
                />
            )}
        </>
    );
};

const Emoji = (props) => {
    const emoji = props.emoji;
    const matches = emojiMatches(emoji, props.search);
    return (
        <EmojiDiv
            onClick={props.onClick}
            title={`${emoji.description} (${emoji.aliases[0]})`}
            style={{ display: (matches) ? '' : 'none' }}
        >
            {props.emoji.emoji}
        </EmojiDiv>
    );
};

const EmojiDiv = styled("div")({
    fontSize: "30px",
    width: "30px",
    height: "30px",
    marginTop: "8px",
    marginBottom: "8px",
    marginRight: "8px",
    lineHeight: "30px",
    cursor: "pointer",
    opacity: 0.85,
    "&:hover": {
        opacity: 1
    }
});

const emojiMatches = (emoji, words) => {
    if (words.length === 0) {
        return true;
    }
    for (const word of words) {
        if (emoji.searchBase.indexOf(word) === -1) {
            return false;
        }
    }
    return true;
}

export default EmojiPicker;
