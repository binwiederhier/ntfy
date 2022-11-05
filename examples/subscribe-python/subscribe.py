#!/usr/bin/env python3

import requests

resp = requests.get("https://ntfy.sh/mytopic/json", stream=True)
for line in resp.iter_lines():
    if line:
        print(line)
