#!/bin/bash
# This is a PAM script hook that shows how to notify you when
# somebody logs into your server. Place at /usr/local/bin/ntfy-ssh-login.sh (with chmod +x!).

if [ "${PAM_TYPE}" = "open_session" ]; then
  echo -en "\u26A0\uFE0F SSH login to $(hostname): ${PAM_USER} from ${PAM_RHOST}" | curl -T- ntfy.sh/alerts
fi
