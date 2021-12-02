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

## Server-sent messages in your web app
Just as you can [subscribe to topics in the Web UI](subscribe/web.md), you can use ntfy in your own
web application. Check out the <a href="example.html">live example</a> or just look the source of this page.

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



