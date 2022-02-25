import {makeStyles} from "@mui/styles";

const useStyles = makeStyles(theme => ({
  bottomBar: {
    display: 'flex',
    flexDirection: 'row',
    justifyContent: 'space-between',
    paddingLeft: '24px',
    paddingTop: '8px 24px',
    paddingBottom: '8px 24px',
  },
  statusText: {
    margin: '0px',
    paddingTop: '8px',
  }
}));

export default useStyles;
