# Examples

There are a million ways to use ntfy, but here are some inspirations. I try to collect
<a href="https://github.com/binwiederhier/ntfy/tree/main/examples">examples on GitHub</a>, so be sure to check
those out, too.

## A long process is done: backups, copying data, pipelines, ...
I started adding notifications pretty much all of my scripts. Typically, I just chain the <tt>curl</tt> call
directly to the command I'm running. The following example will either send <i>Laptop backup succeeded</i>
or ⚠️ <i>Laptop backup failed</i> directly to my phone:

```
rsync -a root@laptop /backups/laptop \
  && zfs snapshot ... \
  && curl -H prio:low -d "Laptop backup succeeded" ntfy.sh/backups \
  || curl -H tags:warning -H prio:high -d "Laptop backup failed" ntfy.sh/backups
```

## Low disk space alerts
Here's a simple cronjob that I use to alert me when the disk space on the root disk is running low. It's simple, but 
effective. 

``` bash 
#!/bin/bash

mingigs=10
avail=$(df | awk '$6 == "/" && $4 < '$mingigs' * 1024*1024 { print $4/1024/1024 }')
topicurl=https://ntfy.sh/mytopic

if [ -n "$avail" ]; then
  curl \
    -d "Only $avail GB available on the root disk. Better clean that up." \
    -H "Title: Low disk space alert on $(hostname)" \
    -H "Priority: high" \
    -H "Tags: warning,cd" \
    $topicurl
fi
```

## Server-sent messages in your web app
Just as you can [subscribe to topics in the Web UI](subscribe/web.md), you can use ntfy in your own
web application. Check out the <a href="/example.html">live example</a> or just look the source of this page.

## Notify on SSH login
Years ago my home server was broken into. That shook me hard, so every time someone logs into any machine that I
own, I now message myself. Here's an example of how to use <a href="https://en.wikipedia.org/wiki/Linux_PAM">PAM</a>
to notify yourself on SSH login.

=== "/etc/pam.d/sshd"
    ```
    # at the end of the file
    session optional pam_exec.so /usr/bin/ntfy-ssh-login.sh
    ```

=== "/usr/bin/ntfy-ssh-login.sh"
    ```bash
    #!/bin/bash
    if [ "${PAM_TYPE}" = "open_session" ]; then
      curl \
        -H prio:high \
        -H tags:warning \
        -d "SSH login: ${PAM_USER} from ${PAM_RHOST}" \
        ntfy.sh/alerts
    fi
    ```

## Collect data from multiple machines
The other day I was running tasks on 20 servers, and I wanted to collect the interim results
as a CSV in one place. Each of the servers was publishing to a topic as the results completed (`publish-result.sh`), 
and I had one central collector to grab the results as they came in (`collect-results.sh`).

It looked something like this:

=== "collect-results.sh"
    ```bash
    while read result; do
      [ -n "$result" ] && echo "$result" >> results.csv
    done < <(stdbuf -i0 -o0 curl -s ntfy.sh/results/raw)
    ```
=== "publish-result.sh" 
    ```bash
    // This script was run on each of the 20 servers. It was doing heavy processing ...
    
    // Publish script results
    curl -d "$(hostname),$count,$time" ntfy.sh/results
    ```

## Ansible, Salt and Puppet
You can easily integrate ntfy into Ansible, Salt, or Puppet to notify you when runs are done or are highstated.
One of my co-workers uses the following Ansible task to let him know when things are done:

```yml
- name: Send ntfy.sh update
  uri:
    url: "https://ntfy.sh/{{ ntfy_channel }}"
    method: POST
    body: "{{ inventory_hostname }} reseeding complete"
```

## Watchtower notifications (shoutrrr)
You can use `shoutrrr` generic webhook support to send watchtower notifications to your ntfy topic.

Example docker-compose.yml:
```yml
services:
  watchtower:
    image: containrrr/watchtower
    environment:
      - WATCHTOWER_NOTIFICATIONS=shoutrrr
      - WATCHTOWER_NOTIFICATION_URL=generic+https://ntfy.sh/my_watchtower_topic?title=WatchtowerUpdates
```

Or, if you only want to send notifications using shoutrrr:
```
shoutrrr send -u "generic+https://ntfy.sh/my_watchtower_topic?title=WatchtowerUpdates" -m "testMessage"
```

## Random cronjobs
Alright, here's one for the history books. I desperately want the `github.com/ntfy` organization, but all my tickets with
GitHub have been hopeless. In case it ever becomes available, I want to know immediately.

``` cron
# Check github/ntfy user
*/6 * * * * if curl -s https://api.github.com/users/ntfy | grep "Not Found"; then curl -d "github.com/ntfy is available" -H "Tags: tada" -H "Prio: high" ntfy.sh/my-alerts; fi
~           
```

## Download notifications (Sonarr, Radarr, Lidarr, Readarr, Prowlarr, SABnzbd)
It's possible to use custom scripts for all the *arr services, plus SABnzbd. Notifications for downloads, warnings, grabs etc.
Some simple bash scripts to achieve this are kindly provided in [nickexyz's repository](https://github.com/nickexyz/ntfy-shellscripts). 

## Node-RED
You can use the HTTP request node to send messages with [Node-RED](https://nodered.org), some examples:

Example: Send a message
<details>
<summary>Expand</summary>

```
[
    {
        "id": "c956e688cc74ad8e",
        "type": "http request",
        "z": "fabdd7a3.4045a",
        "name": "ntfy.sh",
        "method": "POST",
        "ret": "txt",
        "paytoqs": "ignore",
        "url": "https://ntfy.sh/mytopic",
        "tls": "",
        "persist": false,
        "proxy": "",
        "authType": "",
        "senderr": false,
        "credentials":
        {
            "user": "",
            "password": ""
        },
        "x": 590,
        "y": 3160,
        "wires":
        [
            []
        ]
    },
    {
        "id": "32ee1eade51fae50",
        "type": "function",
        "z": "fabdd7a3.4045a",
        "name": "data",
        "func": "msg.payload = \"Something happened\";\nmsg.headers = {};\nmsg.headers['tags'] = 'house';\nmsg.headers['X-Title'] = 'Home Assistant';\n\nreturn msg;",
        "outputs": 1,
        "noerr": 0,
        "initialize": "",
        "finalize": "",
        "libs": [],
        "x": 470,
        "y": 3160,
        "wires":
        [
            [
                "c956e688cc74ad8e"
            ]
        ]
    },
    {
        "id": "b287e59cd2311815",
        "type": "inject",
        "z": "fabdd7a3.4045a",
        "name": "Manual start",
        "props":
        [
            {
                "p": "payload"
            },
            {
                "p": "topic",
                "vt": "str"
            }
        ],
        "repeat": "",
        "crontab": "",
        "once": false,
        "onceDelay": "20",
        "topic": "",
        "payload": "",
        "payloadType": "date",
        "x": 330,
        "y": 3160,
        "wires":
        [
            [
                "32ee1eade51fae50"
            ]
        ]
    }
]
```

</details>

![Node red message flow](static/img/nodered-message.png)


Example: Send a picture
<details>
<summary>Expand</summary>

```
[
    {
        "id": "d135a13eadeb9d6d",
        "type": "http request",
        "z": "fabdd7a3.4045a",
        "name": "Download image",
        "method": "GET",
        "ret": "bin",
        "paytoqs": "ignore",
        "url": "https://www.google.com/images/branding/googlelogo/1x/googlelogo_color_272x92dp.png",
        "tls": "",
        "persist": false,
        "proxy": "",
        "authType": "",
        "senderr": false,
        "credentials":
        {
            "user": "",
            "password": ""
        },
        "x": 490,
        "y": 3320,
        "wires":
        [
            [
                "6e75bc41d2ec4a03"
            ]
        ]
    },
    {
        "id": "6e75bc41d2ec4a03",
        "type": "function",
        "z": "fabdd7a3.4045a",
        "name": "data",
        "func": "msg.payload = msg.payload;\nmsg.headers = {};\nmsg.headers['tags'] = 'house';\nmsg.headers['X-Title'] = 'Home Assistant - Picture';\n\nreturn msg;",
        "outputs": 1,
        "noerr": 0,
        "initialize": "",
        "finalize": "",
        "libs": [],
        "x": 650,
        "y": 3320,
        "wires":
        [
            [
                "eb160615b6ceda98"
            ]
        ]
    },
    {
        "id": "eb160615b6ceda98",
        "type": "http request",
        "z": "fabdd7a3.4045a",
        "name": "ntfy.sh",
        "method": "PUT",
        "ret": "bin",
        "paytoqs": "ignore",
        "url": "https://ntfy.sh/mytopic",
        "tls": "",
        "persist": false,
        "proxy": "",
        "authType": "",
        "senderr": false,
        "credentials":
        {
            "user": "",
            "password": ""
        },
        "x": 770,
        "y": 3320,
        "wires":
        [
            []
        ]
    },
    {
        "id": "5b8dbf15c8a7a3a5",
        "type": "inject",
        "z": "fabdd7a3.4045a",
        "name": "Manual start",
        "props":
        [
            {
                "p": "payload"
            },
            {
                "p": "topic",
                "vt": "str"
            }
        ],
        "repeat": "",
        "crontab": "",
        "once": false,
        "onceDelay": "20",
        "topic": "",
        "payload": "",
        "payloadType": "date",
        "x": 310,
        "y": 3320,
        "wires":
        [
            [
                "d135a13eadeb9d6d"
            ]
        ]
    }
]
```

</details>

![Node red picture flow](static/img/nodered-picture.png)

## Gatus service health check

An example for a custom alert with <a href="https://github.com/TwiN/gatus">Gatus</a>
```
alerting:
  custom:
    url: "https://ntfy.sh"
    method: "POST"
    body: |
      {
        "topic": "mytopic",
        "message": "[ENDPOINT_NAME] - [ALERT_DESCRIPTION]",
        "title": "Gatus",
        "tags": ["[ALERT_TRIGGERED_OR_RESOLVED]"],
        "priority": 3
      }
    default-alert:
      enabled: true
      description: "health check failed"
      send-on-resolved: true
      failure-threshold: 3
      success-threshold: 3
    placeholders:
      ALERT_TRIGGERED_OR_RESOLVED:
        TRIGGERED: "warning"
        RESOLVED: "white_check_mark"
```
