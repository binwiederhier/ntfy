import * as React from "react";

class ErrorBoundary extends React.Component {
    constructor(props) {
        super(props);
        this.state = { error: null, info: null };
    }

    componentDidCatch(error, info) {
        this.setState({ error, info });
        console.error("[ErrorBoundary] A horrible error occurred", info);
    }

    static getDerivedStateFromError(error) {
        return { error: true, errorMessage: error.toString() }
    }

    render() {
        if (this.state.info) {
            return (
                <div>
                    <h2>Something went wrong.</h2>
                    <pre>{this.state.error && this.state.error.toString()}</pre>
                    <pre>{this.state.info.componentStack}</pre>
                </div>
            );
        }
        return this.props.children;
    }
}

export default ErrorBoundary;
