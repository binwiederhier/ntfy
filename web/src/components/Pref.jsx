import { styled } from "@mui/material";
import * as React from "react";

export const PrefGroup = styled("div", { attrs: { role: "table" } })`
  display: flex;
  flex-direction: column;
  gap: 20px;
`;

const PrefRow = styled("div")`
  display: flex;
  flex-direction: row;

  > div:first-of-type {
    flex: 1 0 40%;
    display: flex;
    flex-direction: column;
    justify-content: ${(props) => (props.alignTop ? "normal" : "center")};
  }

  > div:last-of-type {
    flex: 1 0 calc(60% - 50px);
    display: flex;
    flex-direction: column;
    justify-content: ${(props) => (props.alignTop ? "normal" : "center")};
  }

  @media (max-width: 1000px) {
    flex-direction: column;
    gap: 10px;

    > :div:first-of-type,
    > :div:last-of-type {
      flex: unset;
    }

    > div:last-of-type {
      .MuiFormControl-root {
        margin: 0;
      }
    }
  }
`;

export const Pref = (props) => (
  <PrefRow role="row" alignTop={props.alignTop}>
    <div role="cell" id={props.labelId ?? ""} aria-label={props.title}>
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
    <div role="cell">{props.children}</div>
  </PrefRow>
);
