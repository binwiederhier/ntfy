# Integrations + community projects

There are quite a few projects that work with ntfy, integrate ntfy, or have been built around ntfy. It's super exciting to see what you guys have come up with. Feel free to [create a pull request on GitHub](https://github.com/binwiederhier/ntfy/issues) to add your own project here.

I've added a ⭐ to projects or posts that have a significant following, or had a lot of interaction by the community.

## Public ntfy servers

| URL                                               |  Country  |
|---------------------------------------------------|:---------:|
| [ntfy.sh](https://ntfy.sh/) (*Official*)          |   🇺🇸    |
| [ntfy.tedomum.net](https://ntfy.tedomum.net/)     | 🇫🇷 🇪🇺 |
| [ntfy.jae.fi](https://ntfy.jae.fi/)               | 🇫🇮 🇪🇺 |
| [ntfy.adminforge.de](https://ntfy.adminforge.de/) | 🇩🇪 🇪🇺 |

Thanks to everyone running a public server. **You guys rock!** To the users: Be aware that server operators can log your 
messages until I finally finish implementing end-to-end encryption.

## Official integrations

- [Healthchecks.io](https://healthchecks.io/) ⭐ - Online service for monitoring regularly running tasks such as cron jobs
- [Apprise](https://github.com/caronc/apprise/wiki/Notify_ntfy) ⭐ - Push notifications that work with just about every platform
- [Uptime Kuma](https://uptime.kuma.pet/) ⭐ - A self-hosted monitoring tool
- [Robusta](https://docs.robusta.dev/master/catalog/sinks/webhook.html) ⭐ - open source platform for Kubernetes troubleshooting
- [borgmatic](https://torsion.org/borgmatic/docs/how-to/monitor-your-backups/#third-party-monitoring-services) ⭐ - configuration-driven backup software for servers and workstations
- [Radarr](https://radarr.video/) ⭐ - Movie collection manager for Usenet and BitTorrent users
- [Sonarr](https://sonarr.tv/) ⭐ - PVR for Usenet and BitTorrent users
- [Gatus](https://gatus.io/) ⭐ - Automated service health dashboard
- [FlexGet](https://flexget.com/Plugins/Notifiers/ntfysh) ⭐ - Multipurpose automation tool for all of your media
- [Platypush](https://docs.platypush.tech/platypush/plugins/ntfy.html) - Automation platform aimed to run on any device that can run Python

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

## CLIs + GUIs

- [ntfy.sh.sh](https://github.com/mininmobile/ntfy.sh.sh) - Run scripts on ntfy.sh events
- [ntfy Desktop client](https://github.com/mininmobile/ntfy-desktop) - Cross-platform desktop application for ntfy
- [ntfy svelte front-end](https://github.com/novatorem/Ntfy) - Front-end built with svelte
- [wio-ntfy-ticker](https://github.com/nachotp/wio-ntfy-ticker) - Ticker display for a ntfy.sh topic
- [ntfysh-windows](https://github.com/lucas-bortoli/ntfysh-windows) - A ntfy client for Windows Desktop
- [ntfyr](https://github.com/haxwithaxe/ntfyr) - A simple commandline tool to send notifications to ntfy
- [ntfy.py](https://github.com/ioqy/ntfy-client-python) - ntfy.py is a simple nfty.sh client for sending notifications

## Projects + scripts 

- [Grafana-to-ntfy](https://github.com/kittyandrew/grafana-to-ntfy) - Grafana-to-ntfy alerts channel (Rust)
- [ntfy-long-zsh-command](https://github.com/robfox92/ntfy-long-zsh-command) - Notifies you once a long-running command completes (zsh)
- [ntfy-shellscripts](https://github.com/nickexyz/ntfy-shellscripts) - A few scripts for the ntfy project (Shell)
- [QuickStatus](https://github.com/corneliusroot/QuickStatus) - A shell script to alert to any immediate problems upon login (Shell)
- [ntfy.el](https://github.com/shombando/ntfy) - Send notifications from Emacs (Emacs)
- [backup-projects](https://gist.github.com/anthonyaxenov/826ba65abbabd5b00196bc3e6af76002) - Stupidly simple backup script for own projects (Shell)
- [grav-plugin-whistleblower](https://github.com/Himmlisch-Studios/grav-plugin-whistleblower) - Grav CMS plugin to get notifications via ntfy (PHP)
- [ntfy-server-status](https://github.com/filip2cz/ntfy-server-status) -  Checking if server is online and reporting through ntfy (C)
- [borg-based backup](https://github.com/davidhi7/backup) - Simple borg-based backup script with notifications based on ntfy.sh or Discord webhooks (Python/Shell)
- [ntfy.sh *arr script](https://github.com/agent-squirrel/nfty-arr-script) - Quick and hacky script to get sonarr/radarr to notify the ntfy.sh service (Shell)
- [siteeagle](https://github.com/tpanum/siteeagle) - A small Python script to monitor websites and notify changes (Python)
- [send_to_phone](https://github.com/whipped-cream/send_to_phone) - Scripts to upload a file to Transfer.sh and ping ntfy with the download link (Python)
- [ntfy Discord bot](https://github.com/R0dn3yS/ntfy-bot) - WIP ntfy discord bot (TypeScript)
- [ntfy Discord bot](https://github.com/binwiederhier/ntfy-bot) - ntfy Discord bot (Go)
- [Bettarr Notifications](https://github.com/NiNiyas/Bettarr-Notifications) - Better Notifications for Sonarr and Radarr (Python)
- [Notify me the intruders](https://github.com/nothingbutlucas/notify_me_the_intruders) - Notify you if they are intruders or new connections on your network (Shell)
- [Send GitHub Action to ntfy](https://github.com/NiNiyas/ntfy-action) - Send GitHub Action workflow notifications to ntfy (JS)
- [ntfy alertmanager bridge](https://github.com/aTable/ntfy_alertmanager_bridge) - Basic alertmanager bridge to ntfy (JS)
- [ntfy-alertmanager](https://hub.xenrox.net/~xenrox/ntfy-alertmanager) - A bridge between ntfy and Alertmanager (Go)
- [alertmanager-ntfy](https://github.com/pinpox/alertmanager-ntfy) - Relay prometheus alertmanager alerts to ntfy (Go)
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

## Blog + forum posts

- [Console #132](https://console.substack.com/p/console-132) ⭐ - console.substack.com - 11/2022
- [MeshCentral - Ntfy Push Notifications ](https://www.youtube.com/watch?v=wyE4rtUd4Bg) - youtube.com - 11/2022
- [Changelog | Tracking layoffs, tech worker demand still high, ntfy, ...](https://changelog.com/news/tracking-layoffs-tech-worker-demand-still-high-ntfy-devenv-markdoc-mike-bifulco-Y1jW) ⭐ - changelog.com - 11/2022
- [Pointer | Issue #367](https://www.pointer.io/archives/a9495a2a6f/) - pointer.io - 11/2022
- [Envie Push Notifications por POST (de graça e sem cadastro)](https://www.tabnews.com.br/filipedeschamps/envie-push-notifications-por-post-de-graca-e-sem-cadastro) - tabnews.com.br - 11/2022
- [Push Notifications for KDE](https://volkerkrause.eu/2022/11/12/kde-unifiedpush-push-notifications.html) - volkerkrause.eu - 11/2022
- [TLDR Newsletter Daily Update 2022-11-09](https://tldr.tech/tech/newsletter/2022-11-09) ⭐ - tldr.tech - 11/2022
- [Ntfy.sh – Send push notifications to your phone via PUT/POST](https://news.ycombinator.com/item?id=33517944) ⭐ - news.ycombinator.com - 11/2022
- [Ntfy et Jeedom : un plugin](https://lunarok-domotique.com/2022/11/ntfy-et-jeedom/) - lunarok-domotique.com - 11/2022
- [Crea tu propio servidor de notificaciones con Ntfy](https://blog.parravidales.es/crea-tu-propio-servidor-de-notificaciones-con-ntfy/) - blog.parravidales.es - 11/2022
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
