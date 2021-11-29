#!/bin/bash
# This is a PAM script hook that shows how to notify you when
# somebody logs into your server. Place at /usr/local/bin/ntfy-ssh-login.sh (with chmod +x!).

TOPIC_URL=ntfy.sh/alerts

if [ "${PAM_TYPE}" = "open_session" ]; then
  curl -H tags:warning -H prio:high -d "SSH login to $(hostname): ${PAM_USER} from ${PAM_RHOST}" "${TOPIC_URL}"
fi
