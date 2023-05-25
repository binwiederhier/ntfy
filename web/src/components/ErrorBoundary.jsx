import * as React from "react";
import StackTrace from "stacktrace-js";
import { CircularProgress, Link, Button } from "@mui/material";
import { Trans, withTranslation } from "react-i18next";

class ErrorBoundaryImpl extends React.Component {
  constructor(props) {
    super(props);
    this.state = {
      error: false,
      originalStack: null,
      niceStack: null,
      unsupportedIndexedDB: false,
    };
  }

  componentDidCatch(error, info) {
    console.error("[ErrorBoundary] Error caught", error, info);

    // Special case for unsupported IndexedDB in Private Browsing mode (Firefox, Safari), see
    // - https://github.com/dexie/Dexie.js/issues/312
    // - https://bugzilla.mozilla.org/show_bug.cgi?id=781982
    const isUnsupportedIndexedDB =
      error?.name === "InvalidStateError" || (error?.name === "DatabaseClosedError" && error?.message?.indexOf("InvalidStateError") !== -1);

    if (isUnsupportedIndexedDB) {
      this.handleUnsupportedIndexedDB();
    } else {
      this.handleError(error, info);
    }
  }

  handleError(error, info) {
    // Immediately render original stack trace
    const prettierOriginalStack = info.componentStack
      .trim()
      .split("\n")
      .map((line) => `  at ${line}`)
      .join("\n");
    this.setState({
      error: true,
      originalStack: `${error.toString()}\n${prettierOriginalStack}`,
    });

    // Fetch additional info and a better stack trace
    StackTrace.fromError(error).then((stack) => {
      console.error("[ErrorBoundary] Stacktrace fetched", stack);
      const stackString = stack
          .map((el) => `  at ${el.functionName} (${el.fileName}:${el.columnNumber}:${el.lineNumber})`)
          .join("\n");
      const niceStack = `${error.toString()}\n${stackString}`;
      this.setState({ niceStack });
    });
  }

  handleUnsupportedIndexedDB() {
    this.setState({
      error: true,
      unsupportedIndexedDB: true,
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

  renderUnsupportedIndexedDB() {
    const { t } = this.props;
    return (
      <div style={{ margin: "20px" }}>
        <h2>{t("error_boundary_unsupported_indexeddb_title")} 😮</h2>
        <p style={{ maxWidth: "600px" }}>
          <Trans
            i18nKey="error_boundary_unsupported_indexeddb_description"
            components={{
              githubLink: <Link href="https://github.com/binwiederhier/ntfy/issues/208" />,
              discordLink: <Link href="https://discord.gg/cT7ECsZj9w" />,
              matrixLink: <Link href="https://matrix.to/#/#ntfy:matrix.org" />,
            }}
          />
        </p>
      </div>
    );
  }

  renderError() {
    const { t } = this.props;
    return (
      <div style={{ margin: "20px" }}>
        <h2>{t("error_boundary_title")} 😮</h2>
        <p>
          <Trans
            i18nKey="error_boundary_description"
            components={{
              githubLink: <Link href="https://github.com/binwiederhier/ntfy/issues" />,
              discordLink: <Link href="https://discord.gg/cT7ECsZj9w" />,
              matrixLink: <Link href="https://matrix.to/#/#ntfy:matrix.org" />,
            }}
          />
        </p>
        <p>
          <Button variant="outlined" onClick={() => this.copyStack()}>
            {t("error_boundary_button_copy_stack_trace")}
          </Button>
        </p>
        <h3>{t("error_boundary_stack_trace")}</h3>
        {this.state.niceStack ? (
          <pre>{this.state.niceStack}</pre>
        ) : (
          <>
            <CircularProgress size="20px" sx={{ verticalAlign: "text-bottom" }} /> {t("error_boundary_gathering_info")}
          </>
        )}
        <pre>{this.state.originalStack}</pre>
      </div>
    );
  }

  render() {
    if (this.state.error) {
      if (this.state.unsupportedIndexedDB) {
        return this.renderUnsupportedIndexedDB();
      }
      return this.renderError();
    }
    return this.props.children;
  }
}

const ErrorBoundary = withTranslation()(ErrorBoundaryImpl); // Adds props.t
export default ErrorBoundary;
