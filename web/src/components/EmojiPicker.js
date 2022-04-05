import * as React from 'react';
import {useRef, useState} from 'react';
import Popover from '@mui/material/Popover';
import Typography from '@mui/material/Typography';
import {rawEmojis} from '../app/emojis';
import Box from "@mui/material/Box";
import TextField from "@mui/material/TextField";
import {InputAdornment} from "@mui/material";
import IconButton from "@mui/material/IconButton";
import {Close} from "@mui/icons-material";

// Create emoji list by category; filter emojis that are not supported by Desktop Chrome
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
            emojisByCategory[emoji.category].push(emoji);
        }
    } catch (e) {
        // Nothing. Ignore.
    }
});

const EmojiPicker = (props) => {
    const open = Boolean(props.anchorEl);
    const [search, setSearch] = useState("");
    const searchRef = useRef(null);

    /*
        FIXME Search is inefficient, somehow make it faster

        useEffect(() => {
            const matching = rawEmojis.filter(e => {
                const searchLower = search.toLowerCase();
                return e.description.toLowerCase().indexOf(searchLower) !== -1
                    || matchInArray(e.aliases, searchLower)
                    || matchInArray(e.tags, searchLower);
            });
            console.log("matching", matching.length);
        }, [search]);
    */
    const handleSearchClear = () => {
        setSearch("");
        searchRef.current?.focus();
    };

    return (
        <>
            <Popover
                open={open}
                elevation={3}
                onClose={props.onClose}
                anchorEl={props.anchorEl}
                anchorOrigin={{
                    vertical: 'bottom',
                    horizontal: 'left',
                }}
            >
                <Box sx={{ padding: 2, paddingRight: 0, width: "370px", maxHeight: "300px" }}>
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
                        sx={{ marginTop: 0, paddingRight: 2 }}
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
                                search={search.toLowerCase()}
                                onPick={props.onEmojiPick}
                            />
                        )}
                    </Box>
                </Box>
            </Popover>
        </>
    );
};

const Category = (props) => {
    const showTitle = !props.search;
    return (
        <>
            {showTitle &&
                <Typography variant="body1" sx={{ width: "100%", marginTop: 1, marginBottom: 1 }}>
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
    const search = props.search;
    const matches = search === ""
        || emoji.description.toLowerCase().indexOf(search) !== -1
        || matchInArray(emoji.aliases, search)
        || matchInArray(emoji.tags, search);
    if (!matches) {
        return null;
    }
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

const matchInArray = (arr, search) => {
    if (!arr || !search) {
        return false;
    }
    return arr.filter(s => s.indexOf(search) !== -1).length > 0;
}

export default EmojiPicker;
