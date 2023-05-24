import * as React from "react";

export const PrefGroup = (props) => <div role="table">{props.children}</div>;

export const Pref = (props) => {
  const justifyContent = props.alignTop ? "normal" : "center";
  return (
    <div
      role="row"
      style={{
        display: "flex",
        flexDirection: "row",
        marginTop: "10px",
        marginBottom: "20px",
      }}
    >
      <div
        role="cell"
        id={props.labelId ?? ""}
        aria-label={props.title}
        style={{
          flex: "1 0 40%",
          display: "flex",
          flexDirection: "column",
          justifyContent,
          paddingRight: "30px",
        }}
      >
        <div>
          <b>{props.title}</b>
          {props.subtitle && <em> ({props.subtitle})</em>}
        </div>
        {props.description && (
          <div>
            <em>{props.description}</em>
          </div>
        )}
      </div>
      <div
        role="cell"
        style={{
          flex: "1 0 calc(60% - 50px)",
          display: "flex",
          flexDirection: "column",
          justifyContent,
        }}
      >
        {props.children}
      </div>
    </div>
  );
};
