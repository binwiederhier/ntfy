# Integrations + community projects

There are quite a few projects that work with ntfy, integrate ntfy, or have been built around ntfy. It's super exciting to see what you guys have come up with. Feel free to [create a pull request on GitHub](https://github.com/binwiederhier/ntfy/issues) to add your own project here.

I've added a ⭐ to projects or posts that have a significant following, or had a lot of interaction by the community.

## Official integrations

- [Healthchecks.io](https://healthchecks.io/) ⭐ - Online service for monitoring regularly running tasks such as cron jobs
- [Apprise](https://github.com/caronc/apprise/wiki/Notify_ntfy) ⭐ - Push notifications that work with just about every platform
- [Uptime Kuma](https://uptime.kuma.pet/) ⭐ - A self-hosted monitoring tool
- [Robusta](https://docs.robusta.dev/master/catalog/sinks/webhook.html) ⭐ - open source platform for Kubernetes troubleshooting
- [borgmatic](https://torsion.org/borgmatic/docs/how-to/monitor-your-backups/#third-party-monitoring-services) ⭐ - configuration-driven backup software for servers and workstations
- [Radarr](https://radarr.video/) ⭐ - Movie collection manager for Usenet and BitTorrent users
- [Sonarr](https://sonarr.tv/) ⭐ - PVR for Usenet and BitTorrent users
- [Gatus](https://gatus.io/) ⭐ - Automated service health dashboard
- [Automatisch](https://automatisch.io/) ⭐ - Open source Zapier alternative / workflow automation tool
- [FlexGet](https://flexget.com/Plugins/Notifiers/ntfysh) ⭐ - Multipurpose automation tool for all of your media
- [Shoutrrr](https://containrrr.dev/shoutrrr/v0.7/services/ntfy/) ⭐ - Notification library for gophers and their furry friends.
- [Netdata](https://learn.netdata.cloud/docs/alerts-and-notifications/notifications/agent-alert-notifications/ntfy) ⭐ - Real-time performance monitoring
- [Deployer](https://github.com/deployphp/deployer) ⭐ - PHP deployment tool
- [Scrt.link](https://scrt.link/) - Share a secret
- [Platypush](https://docs.platypush.tech/platypush/plugins/ntfy.html) - Automation platform aimed to run on any device that can run Python
- [diun](https://crazymax.dev/diun/) - Docker Image Update Notifier
- [Cloudron](https://www.cloudron.io/store/sh.ntfy.cloudronapp.html) - Platform that makes it easy to manage web apps on your server

## Integration via HTTP/SMTP/etc.

- [Watchtower](https://containrrr.dev/watchtower/) ⭐ - Automating Docker container base image updates (see [integration example](examples.md#watchtower-shoutrrr))
- [Jellyfin](https://jellyfin.org/) ⭐ - The Free Software Media System (see [integration example](examples.md#))
- [Overseer](https://docs.overseerr.dev/using-overseerr/notifications/webhooks) ⭐ - a request management and media discovery tool for Plex (see [integration example](examples.md#jellyseerroverseerr-webhook))
- [Tautulli](https://github.com/Tautulli/Tautulli) ⭐ - Monitoring and tracking tool for Plex (integration [via webhook](https://github.com/Tautulli/Tautulli/wiki/Notification-Agents-Guide#webhook))
- [Mailrise](https://github.com/YoRyan/mailrise) - An SMTP gateway (integration via [Apprise](https://github.com/caronc/apprise/wiki/Notify_ntfy))

## [UnifiedPush](https://unifiedpush.org/users/apps/) integrations

- [Element](https://f-droid.org/packages/im.vector.app/) ⭐ - Matrix client
- [SchildiChat](https://schildi.chat/android/) ⭐ - Matrix client
- [Tusky](https://tusky.app/) ⭐ - Fediverse client
- [Fedilab](https://fedilab.app/) - Fediverse client
- [FindMyDevice](https://gitlab.com/Nulide/findmydevice/) - Find your Device with an SMS or online with the help of FMDServer
- [Tox Push Message App](https://github.com/zoff99/tox_push_msg_app) - Tox Push Message App

## Libraries 

- [ntfy-php-library](https://github.com/VerifiedJoseph/ntfy-php-library) - PHP library for sending messages using a ntfy server (PHP)
- [ntfy-notifier](https://github.com/DAcodedBEAT/ntfy-notifier) - Symfony Notifier integration for ntfy (PHP)
- [ntfpy](https://github.com/Nevalicjus/ntfpy) - API Wrapper for ntfy.sh (Python)
- [pyntfy](https://github.com/DP44/pyntfy) - A module for interacting with ntfy notifications (Python)
- [vntfy](https://github.com/lmangani/vntfy) - Barebone V client for ntfy (V)
- [ntfy-middleman](https://github.com/nachotp/ntfy-middleman) - Wraps APIs and send notifications using ntfy.sh on schedule (Python)
- [ntfy-dotnet](https://github.com/nwithan8/ntfy-dotnet) - .NET client library to interact with a ntfy server (C# / .NET)
- [node-ntfy-publish](https://github.com/cityssm/node-ntfy-publish) - A Node package to publish notifications to an ntfy server (Node)
- [ntfy](https://github.com/jonocarroll/ntfy) - Wraps the ntfy API with pipe-friendly tooling (R)
- [ntfy-for-delphi](https://github.com/hazzelnuts/ntfy-for-delphi) - A friendly library to push instant notifications ntfy (Delphi)
- [ntfy](https://github.com/ffflorian/ntfy) - Send notifications over ntfy (JS)
- [ntfy_dart](https://github.com/jr1221/ntfy_dart) - Dart wrapper around the ntfy API (Dart)
- [gotfy](https://github.com/AnthonyHewins/gotfy) - A Go wrapper for the ntfy API (Go)

## CLIs + GUIs

- [ntfy.sh.sh](https://github.com/mininmobile/ntfy.sh.sh) - Run scripts on ntfy.sh events
- [ntfy Desktop client](https://codeberg.org/zvava/ntfy-desktop) - Cross-platform desktop application for ntfy
- [ntfy svelte front-end](https://github.com/novatorem/Ntfy) - Front-end built with svelte
- [wio-ntfy-ticker](https://github.com/nachotp/wio-ntfy-ticker) - Ticker display for a ntfy.sh topic
- [ntfysh-windows](https://github.com/lucas-bortoli/ntfysh-windows) - A ntfy client for Windows Desktop
- [ntfyr](https://github.com/haxwithaxe/ntfyr) - A simple commandline tool to send notifications to ntfy
- [ntfy.py](https://github.com/ioqy/ntfy-client-python) - ntfy.py is a simple nfty.sh client for sending notifications

## Projects + scripts 

- [Grafana-to-ntfy](https://github.com/kittyandrew/grafana-to-ntfy) - Grafana-to-ntfy alerts channel (Rust)
- [Grafana-ntfy-webhook-integration](https://github.com/academo/grafana-alerting-ntfy-webhook-integration) - Integrates Grafana alerts webhooks (Go)
- [Grafana-to-ntfy](https://gitlab.com/Saibe1111/grafana-to-ntfy) - Grafana-to-ntfy alerts channel (Node Js)
- [ntfy-long-zsh-command](https://github.com/robfox92/ntfy-long-zsh-command) - Notifies you once a long-running command completes (zsh)
- [ntfy-shellscripts](https://github.com/nickexyz/ntfy-shellscripts) - A few scripts for the ntfy project (Shell)
- [QuickStatus](https://github.com/corneliusroot/QuickStatus) - A shell script to alert to any immediate problems upon login (Shell)
- [ntfy.el](https://github.com/shombando/ntfy) - Send notifications from Emacs (Emacs)
- [backup-projects](https://gist.github.com/anthonyaxenov/826ba65abbabd5b00196bc3e6af76002) - Stupidly simple backup script for own projects (Shell)
- [grav-plugin-whistleblower](https://github.com/Himmlisch-Studios/grav-plugin-whistleblower) - Grav CMS plugin to get notifications via ntfy (PHP)
- [ntfy-server-status](https://github.com/filip2cz/ntfy-server-status) -  Checking if server is online and reporting through ntfy (C)
- [borg-based backup](https://github.com/davidhi7/backup) - Simple borg-based backup script with notifications based on ntfy.sh or Discord webhooks (Python/Shell)
- [ntfy.sh *arr script](https://github.com/agent-squirrel/nfty-arr-script) - Quick and hacky script to get sonarr/radarr to notify the ntfy.sh service (Shell)
- [website-watcher](https://github.com/muety/website-watcher) - A small tool to watch websites for changes (with XPath support) (Python)
- [siteeagle](https://github.com/tpanum/siteeagle) - A small Python script to monitor websites and notify changes (Python)
- [send_to_phone](https://github.com/whipped-cream/send_to_phone) - Scripts to upload a file to Transfer.sh and ping ntfy with the download link (Python)
- [ntfy Discord bot](https://github.com/R0dn3yS/ntfy-bot) - WIP ntfy discord bot (TypeScript)
- [ntfy Discord bot](https://github.com/binwiederhier/ntfy-bot) - ntfy Discord bot (Go)
- [ntfy Discord bot](https://github.com/jr1221/ntfy_discord_bot) - An advanced modal-based bot for interacting with the ntfy.sh API (Dart)
- [Bettarr Notifications](https://github.com/NiNiyas/Bettarr-Notifications) - Better Notifications for Sonarr and Radarr (Python)
- [Notify me the intruders](https://github.com/nothingbutlucas/notify_me_the_intruders) - Notify you if they are intruders or new connections on your network (Shell)
- [Send GitHub Action to ntfy](https://github.com/NiNiyas/ntfy-action) - Send GitHub Action workflow notifications to ntfy (JS)
- [aTable/ntfy alertmanager bridge](https://github.com/aTable/ntfy_alertmanager_bridge) - Basic alertmanager bridge to ntfy (JS)
- [~xenrox/ntfy-alertmanager](https://hub.xenrox.net/~xenrox/ntfy-alertmanager) - A bridge between ntfy and Alertmanager (Go)
- [pinpox/alertmanager-ntfy](https://github.com/pinpox/alertmanager-ntfy) - Relay prometheus alertmanager alerts to ntfy (Go)
- [alexbakker/alertmanager-ntfy](https://github.com/alexbakker/alertmanager-ntfy) - Service that forwards Prometheus Alertmanager notifications to ntfy (Go)
- [restreamchat2ntfy](https://github.com/kurohuku7/restreamchat2ntfy) - Send restream.io chat to ntfy to check on the Meta Quest (JS)
- [k8s-ntfy-deployment-service](https://github.com/Christian42/k8s-ntfy-deployment-service) - Automatic Kubernetes (k8s) ntfy deployment
- [huginn-global-entry-notif](https://github.com/kylezoa/huginn-global-entry-notif) - Checks CBP API for available appointments with Huginn (JSON)
- [ntfyer](https://github.com/KikyTokamuro/ntfyer) - Sending various information to your ntfy topic by time (TypeScript)
- [git-simple-notifier](https://github.com/plamenjm/git-simple-notifier) - Script running git-log, checking for new repositories (Shell)
- [ntfy-to-slack](https://github.com/ozskywalker/ntfy-to-slack) - Tool to subscribe to a ntfy topic and send the messages to a Slack webhook (Go)
- [ansible-ntfy](https://github.com/jpmens/ansible-ntfy) - Ansible action plugin to post JSON messages to ntfy (Python)
- [ntfy-notification-channel](https://github.com/wijourdil/ntfy-notification-channel) - Laravel Notification channel for ntfy (PHP)
- [ntfy_on_a_chip](https://github.com/gergepalfi/ntfy_on_a_chip) - ESP8266 and ESP32 client code to communicate with ntfy
- [ntfy-sdk](https://github.com/yukibtc/ntfy-sdk) - ntfy client library to send notifications (Rust)
- [ntfy_ynh](https://github.com/YunoHost-Apps/ntfy_ynh) - ntfy app for YunoHost 
- [woodpecker-ntfy](https://codeberg.org/l-x/woodpecker-ntfy)- Woodpecker CI plugin for sending ntfy notfication from a pipeline (Go)
- [drone-ntfy](https://github.com/Clortox/drone-ntfy) - Drone.io plugin for sending ntfy notifications from a pipeline (Shell)
- [ignition-ntfy-module](https://github.com/Kyvis-Labs/ignition-ntfy-module) - Adds support for sending notifications via a ntfy server to Ignition (Java)
- [maubot-ntfy](https://gitlab.com/999eagle/maubot-ntfy) - Matrix bot to subscribe to ntfy topics and send messages to Matrix (Python)
- [ntfy-wrapper](https://github.com/vict0rsch/ntfy-wrapper) - Wrapper around ntfy (Python)
- [nodebb-plugin-ntfy](https://github.com/NodeBB/nodebb-plugin-ntfy) - Push notifications for NodeBB forums
- [n8n-ntfy](https://github.com/raghavanand98/n8n-ntfy.sh) - n8n community node that lets you use ntfy in your workflows
- [nlog-ntfy](https://github.com/MichelMichels/nlog-ntfy) - Send NLog messages over ntfy (C# / .NET / NLog)
- [helm-charts](https://github.com/sarab97/helm-charts) - Helm charts of some of the selfhosted services, incl. ntfy
- [ntfy_ansible_role](https://github.com/stevenengland/ntfy_ansible_role) (on [Ansible Galaxy](https://galaxy.ansible.com/stevenengland/ntfy)) - Ansible role to install ntfy
- [easy2ntfy](https://github.com/chromoxdor/easy2ntfy) - Gateway for ESPeasy to receive commands through ntfy and using easyfetch (HTML/JS)
- [ntfy_lite](https://github.com/MPI-IS/ntfy_lite) - Minimalist python API for pushing ntfy notifications (Python)
- [notify](https://github.com/guanguans/notify) - 推送通知 (PHP)
- [zpool-events](https://github.com/maglar0/zpool-events) - Notify on ZFS pool events (Python)
- [ntfyd](https://github.com/joachimschmidt557/ntfyd) - ntfy desktop daemon (Zig)
- [ntfy-browser](https://github.com/johman10/ntfy-browser) - browser extension to receive notifications without having the page open (TypeScript)
- [ntfy-electron](https://github.com/xdpirate/ntfy-electron) - Electron wrapper for the ntfy web app (JS)
- [systemd-ntfy-poweronoff](https://github.com/stendler/systemd-ntfy-poweronoff) -  Systemd services to send notifications on system startup and shutdown (Go)
- [msgdrop](https://github.com/jbrubake/msgdrop) - Send and receive encrypted messages (Bash)
- [vigilant](https://github.com/VerifiedJoseph/vigilant) - Monitor RSS/ATOM and JSON feeds, and send push notifications on new entries (PHP)

## Blog + forum posts

- [Pingdom alternative in one line of curl through ntfy.sh](https://piqoni.bearblog.dev/uptime-monitoring-in-one-line-of-curl/) - bearblog.dev - 6/2023 
- [#OpenSourceDiscovery 78: ntfy.sh](https://opensourcedisc.substack.com/p/opensourcediscovery-78-ntfysh) - opensourcedisc.substack.com - 6/2023 
- [ntfy: des notifications instantanées](https://blogmotion.fr/diy/ntfy-notification-push-domotique-20708) - blogmotion.fr - 5/2023
- [桌面通知：ntfy](https://www.cnblogs.com/xueweihan/archive/2023/05/04/17370060.html) - cnblogs.com - 5/2023 
- [ntfy.sh - Open source push notifications via PUT/POST](https://lobste.rs/s/5drapz/ntfy_sh_open_source_push_notifications) - lobste.rs - 5/2023 
- [Install ntfy Inside Docker Container in Linux](https://lindevs.com/install-ntfy-inside-docker-container-in-linux) - lindevs.com - 4/2023 
- [ntfy.sh](https://neo-sahara.com/wp/2023/03/25/ntfy-sh/) - neo-sahara.com - 3/2023 
- [Using Ntfy to send and receive push notifications - Samuel Rosa de Oliveria - Delphicon 2023](https://www.youtube.com/watch?v=feu0skpI9QI) - youtube.com - 3/2023 
- [ntfy: własny darmowy system powiadomień](https://sprawdzone.it/ntfy-wlasny-darmowy-system-powiadomien/) - sprawdzone.it - 3/2023 
- [Deploying ntfy on railway](https://www.youtube.com/watch?v=auJICXtxoNA) - youtube.com - 3/2023
- [Start-Job,Variables, and ntfy.sh](https://klingele.dev/2023/03/01/start-jobvariables-and-ntfy-sh/) - klingele.dev - 3/2023 
- [enviar notificaciones automáticas usando ntfy.sh](https://osiux.com/2023-02-15-send-automatic-notifications-using-ntfy.html) - osiux.com - 2/2023 
- [Carnet IP动态解析以及通过ntfy推送IP信息](https://blog.wslll.cn/index.php/archives/201/) - blog.wslll.cn - 2/2023 
- [Open-Source-Brieftaube: ntfy verschickt Push-Meldungen auf Smartphone und PC](https://www.heise.de/news/Open-Source-Brieftaube-ntfy-verschickt-Push-Meldungen-auf-Smartphone-und-PC-7521583.html) ⭐ - heise.de - 2/2023 
- [Video: Simple Push Notifications ntfy](https://www.youtube.com/watch?v=u9EcWrsjE20) ⭐ - youtube.com - 2/2023
- [Use ntfy.sh with Home Assistant](https://diecknet.de/en/2023/02/12/ntfy-sh-with-homeassistant/) - diecknet.de - 2/2023 
- [On installe Ntfy sur Synology Docker](https://www.maison-et-domotique.com/140356-serveur-notification-jeedom-ntfy-synology-docker/) - maison-et-domotique.co - 1/2023 
- [January 2023 Developer Update](https://community.nodebb.org/topic/16908/january-2023-developer-update) - nodebb.org - 1/2023
- [Comment envoyer des notifications push sur votre téléphone facilement et gratuitement?](https://korben.info/notifications-push-telephone.html) - 1/2023
- [UnifiedPush: a decentralized, open-source push notification protocol](https://f-droid.org/en/2022/12/18/unifiedpush.html) ⭐ - 12/2022
- [ntfy setup instructions](https://docs.benjamin-altpeter.de/network/vms/1001029-ntfy/) - benjamin-altpeter.de - 12/2022
- [Ntfy Self-Hosted Push Notifications](https://lachlanlife.net/posts/2022-12-ntfy/) - lachlanlife.net - 12/2022 
- [NTFY - système de notification hyper simple et complet](https://www.youtube.com/watch?v=UieZYWVVgA4) - youtube.com - 12/2022
- [ntfy.sh](https://paramdeo.com/til/ntfy-sh) - paramdeo.com - 11/2022
- [Using ntfy to warn me when my computer is discharging](https://ulysseszh.github.io/programming/2022/11/28/ntfy-warn-discharge.html) - ulysseszh.github.io - 11/2022
- [ntfy - Push Notification Service](https://dizzytech.de/posts/ntfy/) - dizzytech.de - 11/2022 
- [Console #132](https://console.substack.com/p/console-132) ⭐ - console.substack.com - 11/2022
- [How to make my phone buzz*](https://evbogue.com/howtomakemyphonebuzz) - evbogue.com - 11/2022
- [MeshCentral - Ntfy Push Notifications ](https://www.youtube.com/watch?v=wyE4rtUd4Bg) - youtube.com - 11/2022
- [Changelog | Tracking layoffs, tech worker demand still high, ntfy, ...](https://changelog.com/news/tracking-layoffs-tech-worker-demand-still-high-ntfy-devenv-markdoc-mike-bifulco-Y1jW) ⭐ - changelog.com - 11/2022
- [Pointer | Issue #367](https://www.pointer.io/archives/a9495a2a6f/) - pointer.io - 11/2022
- [Envie Push Notifications por POST (de graça e sem cadastro)](https://www.tabnews.com.br/filipedeschamps/envie-push-notifications-por-post-de-graca-e-sem-cadastro) - tabnews.com.br - 11/2022
- [Push Notifications for KDE](https://volkerkrause.eu/2022/11/12/kde-unifiedpush-push-notifications.html) - volkerkrause.eu - 11/2022
- [TLDR Newsletter Daily Update 2022-11-09](https://tldr.tech/tech/newsletter/2022-11-09) ⭐ - tldr.tech - 11/2022
- [Ntfy.sh – Send push notifications to your phone via PUT/POST](https://news.ycombinator.com/item?id=33517944) ⭐ - news.ycombinator.com - 11/2022
- [Ntfy et Jeedom : un plugin](https://lunarok-domotique.com/2022/11/ntfy-et-jeedom/) - lunarok-domotique.com - 11/2022
- [Crea tu propio servidor de notificaciones con Ntfy](https://blog.parravidales.es/crea-tu-propio-servidor-de-notificaciones-con-ntfy/) - blog.parravidales.es - 11/2022
- [unRAID Notifications with ntfy.sh](https://lder.dev/posts/ntfy-Notifications-With-unRAID/) - lder.dev - 10/2022
- [Zero-cost push notifications to your phone or desktop via PUT/POST ](https://lobste.rs/s/41dq13/zero_cost_push_notifications_your_phone) - lobste.rs - 10/2022
- [A nifty push notification system: ntfy](https://jpmens.net/2022/10/30/a-nifty-push-notification-system-ntfy/) - jpmens.net - 10/2022 
- [Alarmanlage der dritten Art (YouTube video)](https://www.youtube.com/watch?v=altb5QLHbaU&feature=youtu.be) - youtube.com - 10/2022
- [Neue Services: Ntfy, TikTok und RustDesk](https://adminforge.de/tools/neue-services-ntfy-tiktok-und-rustdesk/) - adminforge.de - 9/2022
- [Ntfy, le service de notifications qu’il vous faut](https://www.cachem.fr/ntfy-le-service-de-notifications-quil-vous-faut/) - cachem.fr - 9/2022
- [NAS Synology et notifications avec ntfy](https://www.cachem.fr/synology-notifications-ntfy/) - cachem.fr - 9/2022 
- [Self hosted Mobile Push Notifications using NTFY | Thejesh GN](https://thejeshgn.com/2022/08/23/self-hosted-mobile-push-notifications-using-ntfy/) - thejeshgn.com - 8/2022
- [Fedora Magazine | 4 cool new projects to try in Copr](https://fedoramagazine.org/4-cool-new-projects-to-try-in-copr-for-august-2022/) - fedoramagazine.org - 8/2022
- [Docker로 오픈소스 푸시알람 프로젝트 ntfy.sh 설치 및 사용하기.(Feat. Uptimekuma)](https://svrforum.com/svr/398979) - svrforum.com - 8/2022
- [Easy notifications from R](https://sometimesir.com/posts/easy-notifications-from-r/) - sometimesir.com - 6/2022
- [ntfy is finally coming to iOS, and Matrix/UnifiedPush gateway support](https://www.reddit.com/r/selfhosted/comments/vdzvxi/ntfy_is_finally_coming_to_ios_with_full/) ⭐ - reddit.com - 6/2022
- [Install guide (with Docker)](https://chowdera.com/2022/150/202205301257379077.html) - chowdera.com - 5/2022
- [无需注册的通知服务ntfy](https://blog.csdn.net/wbsu2004/article/details/125040247) - blog.csdn.net - 5/2022
- [Updated review post (Jan-Lukas Else)](https://jlelse.blog/thoughts/2022/04/ntfy) - jlelse.blog - 4/2022
- [Using ntfy and Tasker together](https://lachlanlife.net/posts/2022-04-tasker-ntfy/) - lachlanlife.net - 4/2022 
- [Reddit feature update post](https://www.reddit.com/r/selfhosted/comments/uetlso/ntfy_is_a_tool_to_send_push_notifications_to_your/) ⭐ - reddit.com - 4/2022
- [無料で簡単に通知の送受信ができつつオープンソースでセルフホストも可能な「ntfy」を使ってみた](https://gigazine.net/news/20220404-ntfy-push-notification/) - gigazine.net - 4/2022
- [Pocketmags ntfy review](https://pocketmags.com/us/linux-format-magazine/march-2022/articles/1104187/ntfy) - pocketmags.com - 3/2022
- [Reddit web app release post](https://www.reddit.com/r/selfhosted/comments/tc0p0u/say_hello_to_the_brand_new_ntfysh_web_app_push/) ⭐ - reddit.com- 3/2022
- [Lemmy post (Jakob)](https://lemmy.eus/post/15541) - lemmy.eus - 1/2022
- [Reddit UnifiedPush release post](https://www.reddit.com/r/selfhosted/comments/s5jylf/my_open_source_notification_android_app_and/) ⭐ - reddit.com - 1/2022
- [ntfy: send notifications from your computer to your phone](https://rs1.es/tutorials/2022/01/19/ntfy-send-notifications-phone.html) - rs1.es - 1/2022
- [Short ntfy review (Jan-Lukas Else)](https://jlelse.blog/links/2021/12/ntfy-sh) - jlelse.blog - 12/2021
- [Free MacroDroid webhook alternative (FrameXX)](https://www.macrodroidforum.com/index.php?threads/ntfy-sh-free-macrodroid-webhook-alternative.1505/) - macrodroidforum.com - 12/2021
- [ntfy otro sistema de notificaciones pub-sub simple basado en HTTP](https://ugeek.github.io/blog/post/2021-11-05-ntfy-sh-otro-sistema-de-notificaciones-pub-sub-simple-basado-en-http.html) - ugeek.github.io - 11/2021
- [Show HN: A tool to send push notifications to your phone, written in Go](https://news.ycombinator.com/item?id=29715464) ⭐ - news.ycombinator.com - 12/2021
- [Reddit selfhostable post](https://www.reddit.com/r/selfhosted/comments/qxlsm9/my_open_source_notification_android_app_and/) ⭐ - reddit.com - 11/2021


## Alternative ntfy servers

Here's a list of public ntfy servers. As of right now, there is only one official server. The others are provided by the
ntfy community. Thanks to everyone running a public server. **You guys rock!**

| URL                                               | Country            |
|---------------------------------------------------|--------------------|
| [ntfy.sh](https://ntfy.sh/) (*Official*)          | 🇺🇸 United States |
| [ntfy.tedomum.net](https://ntfy.tedomum.net/)     | 🇫🇷 France        |
| [ntfy.jae.fi](https://ntfy.jae.fi/)               | 🇫🇮 Finland       |
| [ntfy.adminforge.de](https://ntfy.adminforge.de/) | 🇩🇪 Germany       |
| [ntfy.envs.net](https://ntfy.envs.net)            | 🇩🇪 Germany       |
| [ntfy.mzte.de](https://ntfy.mzte.de/)             | 🇩🇪 Germany       |
| [ntfy.hostux.net](https://ntfy.hostux.net/)       | 🇫🇷 France        |

Please be aware that **server operators can log your messages**. The project also cannot guarantee the reliability
and uptime of third party servers, so use of each server is **at your own discretion**.
