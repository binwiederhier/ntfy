import * as React from "react";
import StackTrace from "stacktrace-js";
import {CircularProgress} from "@mui/material";
import Button from "@mui/material/Button";

class ErrorBoundary extends React.Component {
    constructor(props) {
        super(props);
        this.state = {
            error: false,
            originalStack: null,
            niceStack: null
        };
    }

    componentDidCatch(error, info) {
        console.error("[ErrorBoundary] Error caught", error, info);

        // Immediately render original stack trace
        const prettierOriginalStack = info.componentStack
            .trim()
            .split("\n")
            .map(line => `  at ${line}`)
            .join("\n");
        this.setState({
            error: true,
            originalStack: `${error.toString()}\n${prettierOriginalStack}`
        });

        // Fetch additional info and a better stack trace
        StackTrace.fromError(error).then(stack => {
            console.error("[ErrorBoundary] Stacktrace fetched", stack);
            const niceStack = `${error.toString()}\n` + stack.map( el => `  at ${el.functionName} (${el.fileName}:${el.columnNumber}:${el.lineNumber})`).join("\n");
            this.setState({ niceStack });
        });
    }

    copyStack() {
        let stack = "";
        if (this.state.niceStack) {
            stack += `${this.state.niceStack}\n\n`;
        }
        stack += `${this.state.originalStack}\n`;
        navigator.clipboard.writeText(stack);
    }

    render() {
        if (this.state.error) {
            return (
                <div style={{margin: '20px'}}>
                    <h2>Oh no, ntfy crashed 😮</h2>
                    <p>
                        This should obviously not happen. Very sorry about this.<br/>
                        If you have a minute, please <a href="https://github.com/binwiederhier/ntfy/issues">report this on GitHub</a>, or let us
                        know via <a href="https://discord.gg/cT7ECsZj9w">Discord</a> or <a href="https://matrix.to/#/#ntfy:matrix.org">Matrix</a>.
                    </p>
                    <p>
                        <Button variant="outlined" onClick={() => this.copyStack()}>Copy stack trace</Button>
                    </p>
                    <h3>Stack trace</h3>
                    {this.state.niceStack
                        ? <pre>{this.state.niceStack}</pre>
                        : <><CircularProgress size="20px" sx={{verticalAlign: "text-bottom"}}/> Gather more info ...</>}
                    <pre>{this.state.originalStack}</pre>
                </div>
            );
        }
        return this.props.children;
    }
}

export default ErrorBoundary;
