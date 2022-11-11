#!/usr/bin/env python3

import requests

resp = requests.get("https://ntfy.sh/mytopic/trigger",
    data="Backup successful 😀".encode(encoding='utf-8'),
    headers={
        "Priority": "high",
        "Tags": "warning,skull",
        "Title": "Hello there"
    })
resp.raise_for_status()
