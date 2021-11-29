#!/bin/bash
# This is an example shell script showing how to consume a ntfy.sh topic using
# a simple script. The notify-send command sends any arriving message as a desktop notification.

TOPIC_URL=ntfy.sh/mytopic

while read msg; do
  [ -n "$msg" ] && notify-send "$msg"
done < <(stdbuf -i0 -o0 curl -s $TOPIC_URL/raw)
