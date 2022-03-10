import * as React from "react";
import StackTrace from "stacktrace-js";
import {CircularProgress} from "@mui/material";
import Button from "@mui/material/Button";

class ErrorBoundary extends React.Component {
    constructor(props) {
        super(props);
        this.state = { error: null, info: null, stack: null };
    }

    componentDidCatch(error, info) {
        this.setState({ error, info });
        console.error("[ErrorBoundary] Error caught", error, info);
        StackTrace.fromError(error).then(stack => {
            console.error("[ErrorBoundary] Stacktrace fetched", stack);
            const stackStr = stack.map( el => {
                return `  at ${el.functionName} (${el.fileName}:${el.columnNumber}:${el.lineNumber})\n`;
            })
            this.setState({ stack: stackStr })
        });
    }

    copyStack() {
        let stack = "";
        if (this.state.stack) {
            stack += `Stack trace:\n${this.state.error}\n${this.state.stack}\n\n`;
        }
        stack += `Original stack trace:\n${this.state.error}\n${this.state.info.componentStack}\n\n`;
        navigator.clipboard.writeText(stack);
    }

    render() {
        if (this.state.info) {
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
                    {this.state.stack
                        ?
                            <pre>
                                {this.state.error && this.state.error.toString()}{"\n"}
                                {this.state.stack}
                            </pre>
                        :
                            <>
                                <CircularProgress size="20px" sx={{verticalAlign: "text-bottom"}}/> Gather more info ...
                            </>
                    }
                    <pre>
                        {this.state.error && this.state.error.toString()}
                        {this.state.info.componentStack}
                    </pre>
                </div>
            );
        }
        return this.props.children;
    }
}

export default ErrorBoundary;
