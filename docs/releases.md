# Release notes
Binaries for all releases can be found on the GitHub releases pages for the [ntfy server](https://github.com/binwiederhier/ntfy/releases)
and the [ntfy Android app](https://github.com/binwiederhier/ntfy-android/releases).

## ntfy Android app v1.14.0 (UNRELEASED)

**Bug fixes:**

* Remove timestamp when copying message text ([#471](https://github.com/binwiederhier/ntfy/issues/471), thanks to [@wunter8](https://github.com/wunter8))

**Additional translations:**

* Korean (thanks to [@YJSofta0f97461d82447ac](https://hosted.weblate.org/user/YJSofta0f97461d82447ac/))

## ntfy server v1.29.1
Released November 17, 2022

This is mostly a bugfix release to address the high load on ntfy.sh. There are now two new options that allow
synchronous batch-writing of messages to the cache. This avoids database locking, and subsequent pileups of waiting
requests.

**Bug fixes:**

* High-load servers: Allow asynchronous batch-writing of messages to cache via `cache-batch-*` options ([#498](https://github.com/binwiederhier/ntfy/issues/498)/[#502](https://github.com/binwiederhier/ntfy/pull/502))
* Sender column in cache.db shows invalid IP ([#503](https://github.com/binwiederhier/ntfy/issues/503))

**Documentation:**

* GitHub Actions example ([#492](https://github.com/binwiederhier/ntfy/pull/492), thanks to [@ksurl](https://github.com/ksurl))
* UnifiedPush ACL clarification ([#497](https://github.com/binwiederhier/ntfy/issues/497), thanks to [@bt90](https://github.com/bt90)) 
* Install instructions for Kustomize ([#463](https://github.com/binwiederhier/ntfy/pull/463), thanks to [@l-maciej](https://github.com/l-maciej))

**Other things:**

* Put ntfy.sh docs on GitHub pages to reduce AWS outbound traffic cost ([#491](https://github.com/binwiederhier/ntfy/issues/491))
* The ntfy.sh server hardware was upgraded to a bigger box. If you'd like to help out carrying the server cost, **[sponsorships and donations](https://github.com/sponsors/binwiederhier)** 💸 would be very much appreciated

## ntfy server v1.29.0
Released November 12, 2022

This release adds the ability to add rate limit exemptions for IP ranges instead of just specific IP addresses. It also fixes 
a few bugs in the web app and the CLI and adds lots of new examples and install instructions.

Thanks to [some love on HN](https://news.ycombinator.com/item?id=33517944), we got so many new ntfy users trying out ntfy
and joining the [chat rooms](https://github.com/binwiederhier/ntfy#chat--forum). **Welcome to the ntfy community to all of you!** 
We also got a ton of new **[sponsors and donations](https://github.com/sponsors/binwiederhier)** 💸, which is amazing. I'd like to thank
all of you for believing in the project, and for helping me pay the server cost. The HN spike increased the AWS cost quite a bit.

**Features:**

* Allow IP CIDRs in `visitor-request-limit-exempt-hosts` ([#423](https://github.com/binwiederhier/ntfy/issues/423), thanks to [@karmanyaahm](https://github.com/karmanyaahm))

**Bug fixes + maintenance:**

* Subscriptions can now have a display name ([#370](https://github.com/binwiederhier/ntfy/issues/370), thanks to [@tfheen](https://github.com/tfheen) for reporting)
* Bump Go version to Go 18.x ([#422](https://github.com/binwiederhier/ntfy/issues/422))
* Web: Strip trailing slash when subscribing ([#428](https://github.com/binwiederhier/ntfy/issues/428), thanks to [@raining1123](https://github.com/raining1123) for reporting, and [@wunter8](https://github.com/wunter8) for fixing)
* Web: Strip trailing slash after server URL in publish dialog ([#441](https://github.com/binwiederhier/ntfy/issues/441), thanks to [@wunter8](https://github.com/wunter8))
* Allow empty passwords in `client.yml` ([#374](https://github.com/binwiederhier/ntfy/issues/374), thanks to [@cyqsimon](https://github.com/cyqsimon) for reporting, and [@wunter8](https://github.com/wunter8) for fixing)
* `ntfy pub` will now use default username and password from `client.yml` ([#431](https://github.com/binwiederhier/ntfy/issues/431), thanks to [@wunter8](https://github.com/wunter8) for fixing)
* Make `ntfy sub` work with `NTFY_USER` env variable ([#447](https://github.com/binwiederhier/ntfy/pull/447), thanks to [SuperSandro2000](https://github.com/SuperSandro2000))
* Web: Disallow GET/HEAD requests with body in actions ([#468](https://github.com/binwiederhier/ntfy/issues/468), thanks to [@ollien](https://github.com/ollien))

**Documentation:**

* Updated developer docs, bump nodejs and go version ([#414](https://github.com/binwiederhier/ntfy/issues/414), thanks to [@YJSoft](https://github.com/YJSoft) for reporting)
* Officially document `?auth=..` query parameter ([#433](https://github.com/binwiederhier/ntfy/pull/433), thanks to [@wunter8](https://github.com/wunter8))
* Added Rundeck example ([#427](https://github.com/binwiederhier/ntfy/pull/427), thanks to [@demogorgonz](https://github.com/demogorgonz))
* Fix Debian installation instructions ([#237](https://github.com/binwiederhier/ntfy/issues/237), thanks to [@Joeharrison94](https://github.com/Joeharrison94) for reporting)
* Updated [example](https://ntfy.sh/docs/examples/#gatus) with official [Gatus](https://github.com/TwiN/gatus) integration (thanks to [@TwiN](https://github.com/TwiN))
* Added [Kubernetes install instructions](https://ntfy.sh/docs/install/#kubernetes) ([#452](https://github.com/binwiederhier/ntfy/pull/452), thanks to [@gmemstr](https://github.com/gmemstr))
* Added [additional NixOS links for self-hosting](https://ntfy.sh/docs/install/#nixos-nix) ([#462](https://github.com/binwiederhier/ntfy/pull/462), thanks to [@wamserma](https://github.com/wamserma))
* Added additional [more secure nginx config example](https://ntfy.sh/docs/config/#nginxapache2caddy) ([#451](https://github.com/binwiederhier/ntfy/pull/451), thanks to [SuperSandro2000](https://github.com/SuperSandro2000))
* Minor fixes in the config table ([#470](https://github.com/binwiederhier/ntfy/pull/470), thanks to [snh](https://github.com/snh))
* Fix broken link ([#476](https://github.com/binwiederhier/ntfy/pull/476), thanks to [@shuuji3](https://github.com/shuuji3))

**Additional translations:**

* Korean (thanks to [@YJSofta0f97461d82447ac](https://hosted.weblate.org/user/YJSofta0f97461d82447ac/))

**Sponsorships:**:

Thank you to the amazing folks who decided to [sponsor ntfy](https://github.com/sponsors/binwiederhier). Thank you for 
helping carry the cost of the public server and developer licenses, and more importantly: Thank you for believing in ntfy! 
You guys rock! 

A list of all the sponsors can be found in the [README](https://github.com/binwiederhier/ntfy/blob/main/README.md).

## ntfy Android app v1.14.0 
Released September 27, 2022

This release adds the ability to set a custom icon to each notification, as well as a display name to subscriptions. We
also moved the action buttons in the detail view to a more logical place, fixed a bunch of bugs, and added four more
languages. Hurray!

**Features:**

* Subscriptions can now have a display name ([#313](https://github.com/binwiederhier/ntfy/issues/313), thanks to [@wunter8](https://github.com/wunter8))
* Display name for UnifiedPush subscriptions ([#355](https://github.com/binwiederhier/ntfy/issues/355), thanks to [@wunter8](https://github.com/wunter8))
* Polling is now done with `since=<id>` API, which makes deduping easier ([#165](https://github.com/binwiederhier/ntfy/issues/165))
* Turned JSON stream deprecation banner into "Use WebSockets" banner (no ticket)
* Move action buttons in notification cards ([#236](https://github.com/binwiederhier/ntfy/issues/236), thanks to [@wunter8](https://github.com/wunter8))
* Icons can be set for each individual notification ([#126](https://github.com/binwiederhier/ntfy/issues/126), thanks to [@wunter8](https://github.com/wunter8))

**Bug fixes:**

* Long-click selecting of notifications doesn't scroll to the top anymore ([#235](https://github.com/binwiederhier/ntfy/issues/235), thanks to [@wunter8](https://github.com/wunter8))
* Add attachment and click URL extras to MESSAGE_RECEIVED broadcast ([#329](https://github.com/binwiederhier/ntfy/issues/329), thanks to [@wunter8](https://github.com/wunter8))
* Accessibility: Clear/choose service URL button in base URL dropdown now has a label ([#292](https://github.com/binwiederhier/ntfy/issues/292), thanks to [@mhameed](https://github.com/mhameed) for reporting)

**Additional translations:**

* Italian (thanks to [@Genio2003](https://hosted.weblate.org/user/Genio2003/))
* Dutch (thanks to [@SchoNie](https://hosted.weblate.org/user/SchoNie/))
* Ukranian (thanks to [@v.kopitsa](https://hosted.weblate.org/user/v.kopitsa/))
* Polish (thanks to [@Namax0r](https://hosted.weblate.org/user/Namax0r/))

Thank you to [@wunter8](https://github.com/wunter8) for proactively picking up some Android tickets, and fixing them! You rock!

## ntfy server v1.28.0
Released September 27, 2022

This release primarily adds icon support for the Android app, and adds a display name to subscriptions in the web app.
Aside from that, we fixed a few random bugs, most importantly the `Priority` header bug that allows the use behind
Cloudflare. We also added a ton of documentation. Most prominently, an [integrations + projects page](https://ntfy.sh/docs/integrations/).

As of now, I also have started accepting **[donations and sponsorships](https://github.com/sponsors/binwiederhier)** 💸. 
I would be very humbled if you consider donating.

**Features:**

* Subscription display name for the web app ([#348](https://github.com/binwiederhier/ntfy/pull/348))
* Allow setting socket permissions via `--listen-unix-mode` ([#356](https://github.com/binwiederhier/ntfy/pull/356), thanks to [@koro666](https://github.com/koro666))
* Icons can be set for each individual notification ([#126](https://github.com/binwiederhier/ntfy/issues/126), thanks to [@wunter8](https://github.com/wunter8))
* CLI: Allow default username/password in `client.yml` ([#372](https://github.com/binwiederhier/ntfy/pull/372), thanks to [@wunter8](https://github.com/wunter8))
* Build support for other Unix systems ([#393](https://github.com/binwiederhier/ntfy/pull/393), thanks to [@la-ninpre](https://github.com/la-ninpre))

**Bug fixes:**

* `ntfy user` commands don't work with `auth_file` but works with `auth-file` ([#344](https://github.com/binwiederhier/ntfy/issues/344), thanks to [@Histalek](https://github.com/Histalek) for reporting)
* Ignore new draft HTTP `Priority` header  ([#351](https://github.com/binwiederhier/ntfy/issues/351), thanks to [@ksurl](https://github.com/ksurl) for reporting)
* Delete expired attachments based on mod time instead of DB entry to avoid races (no ticket)
* Better logging for Matrix push key errors ([#384](https://github.com/binwiederhier/ntfy/pull/384), thanks to [@christophehenry](https://github.com/christophehenry))
* Web: Switched "Pop" and "Pop Swoosh" sounds ([#352](https://github.com/binwiederhier/ntfy/issues/352), thanks to [@coma-toast](https://github.com/coma-toast) for reporting)

**Documentation:**

* Added [integrations + projects page](https://ntfy.sh/docs/integrations/) (**so many integrations, whoa!**)
* Added example for [UptimeRobot](https://ntfy.sh/docs/examples/#uptimerobot)
* Fix some PowerShell publish docs ([#345](https://github.com/binwiederhier/ntfy/pull/345), thanks to [@noahpeltier](https://github.com/noahpeltier))
* Clarified Docker install instructions ([#361](https://github.com/binwiederhier/ntfy/issues/361), thanks to [@barart](https://github.com/barart) for reporting)
* Mismatched quotation marks ([#392](https://github.com/binwiederhier/ntfy/pull/392)], thanks to [@connorlanigan](https://github.com/connorlanigan))

**Additional translations:**

* Ukranian (thanks to [@v.kopitsa](https://hosted.weblate.org/user/v.kopitsa/))
* Polish (thanks to [@Namax0r](https://hosted.weblate.org/user/Namax0r/))

## ntfy server v1.27.2
Released June 23, 2022

This release brings two new CLI options to wait for a command to finish, or for a PID to exit. It also adds more detail
to trace debug output. Aside from other bugs, it fixes a performance issue that occurred in large installations every 
minute or so, due to competing stats gathering (personal installations will likely be unaffected by this). 

**Features:**

* Add `cache-startup-queries` option to allow custom [SQLite performance tuning](config.md#wal-for-message-cache) (no ticket)
* ntfy CLI can now [wait for a command or PID](subscribe/cli.md#wait-for-pidcommand) before publishing ([#263](https://github.com/binwiederhier/ntfy/issues/263), thanks to the [original ntfy](https://github.com/dschep/ntfy) for the idea)
* Trace: Log entire HTTP request to simplify debugging (no ticket)
* Allow setting user password via `NTFY_PASSWORD` env variable ([#327](https://github.com/binwiederhier/ntfy/pull/327), thanks to [@Kenix3](https://github.com/Kenix3))

**Bug fixes:**

* Fix slow requests due to excessive locking ([#338](https://github.com/binwiederhier/ntfy/issues/338))
* Return HTTP 500 for `GET /_matrix/push/v1/notify` when `base-url` is not configured (no ticket)
* Disallow setting `upstream-base-url` to the same value as `base-url` ([#334](https://github.com/binwiederhier/ntfy/issues/334), thanks to [@oester](https://github.com/oester) for reporting)
* Fix `since=<id>` implementation for multiple topics ([#336](https://github.com/binwiederhier/ntfy/issues/336), thanks to [@karmanyaahm](https://github.com/karmanyaahm) for reporting)
* Simple parsing in `Actions` header now supports settings Android `intent=` key ([#341](https://github.com/binwiederhier/ntfy/pull/341), thanks to [@wunter8](https://github.com/wunter8))

**Deprecations:**

* The `ntfy publish --env-topic` option is deprecated as of now (see [deprecations](deprecations.md) for details)

## ntfy server v1.26.0
Released June 16, 2022

This release adds a Matrix Push Gateway directly into ntfy, to make self-hosting a Matrix server easier. The Windows
CLI is now available via Scoop, and ntfy is now natively supported in Uptime Kuma. 

**Features:**

* ntfy now is a [Matrix Push Gateway](https://spec.matrix.org/v1.2/push-gateway-api/) (in combination with [UnifiedPush](https://unifiedpush.org) as the [Provider Push Protocol](https://unifiedpush.org/developers/gateway/), [#319](https://github.com/binwiederhier/ntfy/issues/319)/[#326](https://github.com/binwiederhier/ntfy/pull/326), thanks to [@MayeulC](https://github.com/MayeulC) for reporting)
* Windows CLI is now available via [Scoop](https://scoop.sh) ([ScoopInstaller#3594](https://github.com/ScoopInstaller/Main/pull/3594), [#311](https://github.com/binwiederhier/ntfy/pull/311), [#269](https://github.com/binwiederhier/ntfy/issues/269), thanks to [@kzshantonu](https://github.com/kzshantonu))
* [Uptime Kuma](https://github.com/louislam/uptime-kuma) now allows publishing to ntfy ([uptime-kuma#1674](https://github.com/louislam/uptime-kuma/pull/1674), thanks to [@philippdormann](https://github.com/philippdormann))
* Display ntfy version in `ntfy serve` command  ([#314](https://github.com/binwiederhier/ntfy/issues/314), thanks to [@poblabs](https://github.com/poblabs))

**Bug fixes:**

* Web app: Show "notifications not supported" alert on HTTP ([#323](https://github.com/binwiederhier/ntfy/issues/323), thanks to [@milksteakjellybeans](https://github.com/milksteakjellybeans) for reporting)
* Use last address in `X-Forwarded-For` header as visitor address ([#328](https://github.com/binwiederhier/ntfy/issues/328))

**Documentation**

* Added [example](examples.md) for [Uptime Kuma](https://github.com/louislam/uptime-kuma) integration ([#315](https://github.com/binwiederhier/ntfy/pull/315), thanks to [@philippdormann](https://github.com/philippdormann))
* Fix Docker install instructions  ([#320](https://github.com/binwiederhier/ntfy/issues/320), thanks to [@milksteakjellybeans](https://github.com/milksteakjellybeans) for reporting)
* Add clarifying comments to base-url ([#322](https://github.com/binwiederhier/ntfy/issues/322), thanks to [@milksteakjellybeans](https://github.com/milksteakjellybeans) for reporting)
* Update FAQ for iOS app ([#321](https://github.com/binwiederhier/ntfy/issues/321), thanks to [@milksteakjellybeans](https://github.com/milksteakjellybeans) for reporting)

## ntfy iOS app v1.2
Released June 16, 2022

This release adds support for authentication/authorization for self-hosted servers. It also allows you to
set your server as the default server for new topics.

**Features:**

* Support for auth and user management ([#277](https://github.com/binwiederhier/ntfy/issues/277))
* Ability to add default server ([#295](https://github.com/binwiederhier/ntfy/issues/295))

**Bug fixes:**

* Add validation for selfhosted server URL ([#290](https://github.com/binwiederhier/ntfy/issues/290))

## ntfy server v1.25.2
Released June 2, 2022

This release adds the ability to set a log level to facilitate easier debugging of live systems. It also solves a 
production problem with a few over-users that resulted in Firebase quota problems (only applying to the over-users). 
We now block visitors from using Firebase if they trigger a quota exceeded response.

On top of that, we updated the Firebase SDK and are now building the release in GitHub Actions. We've also got two
more translations: Chinese/Simplified and Dutch.

**Features:**

* Advanced logging, with different log levels and hot reloading of the log level ([#284](https://github.com/binwiederhier/ntfy/pull/284))

**Bugs**:

* Respect Firebase "quota exceeded" response for topics, block Firebase publishing for user for 10min ([#289](https://github.com/binwiederhier/ntfy/issues/289))
* Fix documentation header blue header due to mkdocs-material theme update (no ticket) 

**Maintenance:**

* Upgrade Firebase Admin SDK to 4.x ([#274](https://github.com/binwiederhier/ntfy/issues/274))
* CI: Build from pipeline instead of locally ([#36](https://github.com/binwiederhier/ntfy/issues/36))

**Documentation**:

* ⚠️ [Privacy policy](privacy.md) updated to reflect additional debug/tracing feature (no ticket)
* [Examples](examples.md) for [Home Assistant](https://www.home-assistant.io/) ([#282](https://github.com/binwiederhier/ntfy/pull/282), thanks to [@poblabs](https://github.com/poblabs))
* Install instructions for [NixOS/Nix](https://ntfy.sh/docs/install/#nixos-nix) ([#282](https://github.com/binwiederhier/ntfy/pull/282), thanks to [@arjan-s](https://github.com/arjan-s))
* Clarify `poll_request` wording for [iOS push notifications](https://ntfy.sh/docs/config/#ios-instant-notifications) ([#300](https://github.com/binwiederhier/ntfy/issues/300), thanks to [@prabirshrestha](https://github.com/prabirshrestha) for reporting)
* Example for using ntfy with docker-compose.yml without root privileges ([#304](https://github.com/binwiederhier/ntfy/pull/304), thanks to [@ksurl](https://github.com/ksurl))

**Additional translations:**

* Chinese/Simplified (thanks to [@yufei.im](https://hosted.weblate.org/user/yufei.im/))
* Dutch (thanks to [@SchoNie](https://hosted.weblate.org/user/SchoNie/))

## ntfy iOS app v1.1
Released May 31, 2022

In this release of the iOS app, we add message priorities (mapped to iOS interruption levels), tags and emojis,
action buttons to open websites or perform HTTP requests (in the notification and the detail view), a custom click
action when the notification is tapped, and various other fixes.

It also adds support for self-hosted servers (albeit not supporting auth yet). The self-hosted server needs to be
configured to forward poll requests to upstream ntfy.sh for push notifications to work (see [iOS push notifications](https://ntfy.sh/docs/config/#ios-instant-notifications)
for details).

**Features:**

* [Message priority](https://ntfy.sh/docs/publish/#message-priority) support (no ticket)
* [Tags/emojis](https://ntfy.sh/docs/publish/#tags-emojis) support (no ticket)
* [Action buttons](https://ntfy.sh/docs/publish/#action-buttons) support (no ticket)
* [Click action](https://ntfy.sh/docs/publish/#click-action) support (no ticket)
* Open topic when notification clicked (no ticket)
* Notification now makes a sound and vibrates (no ticket)
* Cancel notifications when navigating to topic (no ticket)
* iOS 14.0 support (no ticket, [PR#1](https://github.com/binwiederhier/ntfy-ios/pull/1), thanks to [@callum-99](https://github.com/callum-99))

**Bug fixes:**

* iOS UI not always updating properly ([#267](https://github.com/binwiederhier/ntfy/issues/267))

## ntfy server v1.24.0
Released May 28, 2022

This release of the ntfy server brings supporting features for the ntfy iOS app. Most importantly, it
enables support for self-hosted servers in combination with the iOS app. This is to overcome the restrictive
Apple development environment.

**Features:**

* Regularly send Firebase keepalive messages to ~poll topic to support self-hosted servers (no ticket)
* Add subscribe filter to query exact messages by ID (no ticket)
* Support for `poll_request` messages to support [iOS push notifications](https://ntfy.sh/docs/config/#ios-instant-notifications) for self-hosted servers (no ticket)

**Bug fixes:**

* Support emails without `Content-Type` ([#265](https://github.com/binwiederhier/ntfy/issues/265), thanks to [@dmbonsall](https://github.com/dmbonsall))

**Additional translations:**

* Italian (thanks to [@Genio2003](https://hosted.weblate.org/user/Genio2003/))

## ntfy iOS app v1.0
Released May 25, 2022

This is the first version of the ntfy iOS app. It supports only ntfy.sh (no selfhosted servers) and only messages + title
(no priority, tags, attachments, ...). I'll rapidly add (hopefully) most of the other ntfy features, and then I'll focus
on self-hosted servers.

The app is now available in the [App Store](https://apps.apple.com/us/app/ntfy/id1625396347).

**Tickets:**

* iOS app ([#4](https://github.com/binwiederhier/ntfy/issues/4), see also: [TestFlight summary](https://github.com/binwiederhier/ntfy/issues/4#issuecomment-1133767150))

**Thanks:**

* Thank you to all the testers who tried out the app. You guys gave me the confidence that it's ready to release (albeit with
  some known issues which will be addressed in follow-up releases).

## ntfy server v1.23.0
Released May 21, 2022

This release ships a CLI for Windows and macOS, as well as the ability to disable the web app entirely. On top of that, 
it adds support for APNs, the iOS messaging service. This is needed for the (soon to be released) iOS app.

**Features:**

* [Windows](https://ntfy.sh/docs/install/#windows) and [macOS](https://ntfy.sh/docs/install/#macos) builds for the [ntfy CLI](https://ntfy.sh/docs/subscribe/cli/) ([#112](https://github.com/binwiederhier/ntfy/issues/112))
* Ability to disable the web app entirely ([#238](https://github.com/binwiederhier/ntfy/issues/238)/[#249](https://github.com/binwiederhier/ntfy/pull/249), thanks to [@Curid](https://github.com/Curid))
* Add APNs config to Firebase messages to support [iOS app](https://github.com/binwiederhier/ntfy/issues/4) ([#247](https://github.com/binwiederhier/ntfy/pull/247), thanks to [@Copephobia](https://github.com/Copephobia))

**Bug fixes:**

* Support underscores in server.yml config options ([#255](https://github.com/binwiederhier/ntfy/issues/255), thanks to [@ajdelgado](https://github.com/ajdelgado))
* Force MAKEFLAGS to --jobs=1 in `Makefile` ([#257](https://github.com/binwiederhier/ntfy/pull/257), thanks to [@oddlama](https://github.com/oddlama))

**Documentation:**

* Typo in install instructions ([#252](https://github.com/binwiederhier/ntfy/pull/252)/[#251](https://github.com/binwiederhier/ntfy/issues/251), thanks to [@oddlama](https://github.com/oddlama))
* Fix typo in private server example ([#262](https://github.com/binwiederhier/ntfy/pull/262), thanks to [@MayeulC](https://github.com/MayeulC))
* [Examples](examples.md) for [jellyseerr](https://github.com/Fallenbagel/jellyseerr)/[overseerr](https://overseerr.dev/) ([#264](https://github.com/binwiederhier/ntfy/pull/264), thanks to [@Fallenbagel](https://github.com/Fallenbagel))

**Additional translations:**

* Portuguese/Brazil (thanks to [@tiagotriques](https://hosted.weblate.org/user/tiagotriques/) and [@pireshenrique22](https://hosted.weblate.org/user/pireshenrique22/))

Thank you to the many translators, who helped translate the new strings so quickly. I am humbled and amazed by your help.  

## ntfy Android app v1.13.0
Released May 11, 2022

This release brings a slightly altered design for the detail view, featuring a card layout to make notifications more easily
distinguishable from one another. It also ships per-topic settings that allow overriding minimum priority, auto delete threshold
and custom icons. Aside from that, we've got tons of bug fixes as usual.

**Features:**

* Per-subscription settings, custom subscription icons ([#155](https://github.com/binwiederhier/ntfy/issues/155), thanks to [@mztiq](https://github.com/mztiq) for reporting)
* Cards in notification detail view ([#175](https://github.com/binwiederhier/ntfy/issues/175), thanks to [@cmeis](https://github.com/cmeis) for reporting)

**Bug fixes:**

* Accurate naming of "mute notifications" from "pause notifications" ([#224](https://github.com/binwiederhier/ntfy/issues/224), thanks to [@shadow00](https://github.com/shadow00) for reporting)
* Make messages with links selectable ([#226](https://github.com/binwiederhier/ntfy/issues/226), thanks to [@StoyanDimitrov](https://github.com/StoyanDimitrov) for reporting)
* Restoring topics or settings from backup doesn't work ([#223](https://github.com/binwiederhier/ntfy/issues/223), thanks to [@shadow00](https://github.com/shadow00) for reporting)
* Fix app icon on old Android versions ([#128](https://github.com/binwiederhier/ntfy/issues/128), thanks to [@shadow00](https://github.com/shadow00) for reporting)
* Fix races in UnifiedPush registration ([#230](https://github.com/binwiederhier/ntfy/issues/230), thanks to @Jakob for reporting)
* Prevent view action from crashing the app ([#233](https://github.com/binwiederhier/ntfy/issues/233))
* Prevent long topic names and icons from overlapping ([#240](https://github.com/binwiederhier/ntfy/issues/240), thanks to [@cmeis](https://github.com/cmeis) for reporting)

**Additional translations:**

* Dutch (*incomplete*, thanks to [@diony](https://hosted.weblate.org/user/diony/))

**Thank you:**

Thanks to [@cmeis](https://github.com/cmeis), [@StoyanDimitrov](https://github.com/StoyanDimitrov), [@Fallenbagel](https://github.com/Fallenbagel) for testing, and
to [@Joeharrison94](https://github.com/Joeharrison94) for the input. And thank you very much to all the translators for catching up so quickly.

## ntfy server v1.22.0
Released May 7, 2022

This release makes the web app more accessible to people with disabilities, and introduces a "mark as read" icon in the web app.
It also fixes a curious bug with WebSockets and Apache and makes the notification sounds in the web app a little quieter.

We've also improved the documentation a little and added translations for three more languages.

**Features:**

* Make web app more accessible ([#217](https://github.com/binwiederhier/ntfy/issues/217))
* Better parsing of the user actions, allowing quotes (no ticket)
* Add "mark as read" icon button to notification ([#243](https://github.com/binwiederhier/ntfy/pull/243), thanks to [@wunter8](https://github.com/wunter8))

**Bug fixes:**

* `Upgrade` header check is now case in-sensitive ([#228](https://github.com/binwiederhier/ntfy/issues/228), thanks to [@wunter8](https://github.com/wunter8) for finding it)
* Made web app sounds quieter ([#222](https://github.com/binwiederhier/ntfy/issues/222))
* Add "private browsing"-specific error message for Firefox/Safari ([#208](https://github.com/binwiederhier/ntfy/issues/208), thanks to [@julianfoad](https://github.com/julianfoad) for reporting)

**Documentation:**

* Improved caddy configuration (no ticket, thanks to @Stnby)
* Additional multi-line examples on the [publish page](https://ntfy.sh/docs/publish/) ([#234](https://github.com/binwiederhier/ntfy/pull/234), thanks to [@aTable](https://github.com/aTable))
* Fixed PowerShell auth example to use UTF-8 ([#242](https://github.com/binwiederhier/ntfy/pull/242), thanks to [@SMAW](https://github.com/SMAW))

**Additional translations:**

* Czech (thanks to [@waclaw66](https://hosted.weblate.org/user/waclaw66/))
* French (thanks to [@nathanaelhoun](https://hosted.weblate.org/user/nathanaelhoun/))
* Hungarian (thanks to [@agocsdaniel](https://hosted.weblate.org/user/agocsdaniel/))

**Thanks for testing:**

Thanks to [@wunter8](https://github.com/wunter8) for testing.

## ntfy Android app v1.12.0
Released Apr 25, 2022

The main feature in this Android release is [Action Buttons](https://ntfy.sh/docs/publish/#action-buttons), a feature
that allows users to add actions to the notifications. Actions can be to view a website or app, send a broadcast, or
send a HTTP request. 

We also added support for [ntfy:// deep links](https://ntfy.sh/docs/subscribe/phone/#ntfy-links), added three more 
languages and fixed a ton of bugs. 

**Features:**

* Custom notification [action buttons](https://ntfy.sh/docs/publish/#action-buttons) ([#134](https://github.com/binwiederhier/ntfy/issues/134),
  thanks to [@mrherman](https://github.com/mrherman) for reporting)
* Support for [ntfy:// deep links](https://ntfy.sh/docs/subscribe/phone/#ntfy-links) ([#20](https://github.com/binwiederhier/ntfy/issues/20), thanks
  to [@Copephobia](https://github.com/Copephobia) for reporting)
* [Fastlane metadata](https://hosted.weblate.org/projects/ntfy/android-fastlane/) can now be translated too ([#198](https://github.com/binwiederhier/ntfy/issues/198),
  thanks to [@StoyanDimitrov](https://github.com/StoyanDimitrov) for reporting)
* Channel settings option to configure DND override, sounds, etc. ([#91](https://github.com/binwiederhier/ntfy/issues/91))

**Bug fixes:**

* Validate URLs when changing default server and server in user management ([#193](https://github.com/binwiederhier/ntfy/issues/193),
  thanks to [@StoyanDimitrov](https://github.com/StoyanDimitrov) for reporting)
* Error in sending test notification in different languages ([#209](https://github.com/binwiederhier/ntfy/issues/209),
  thanks to [@StoyanDimitrov](https://github.com/StoyanDimitrov) for reporting)
* "[x] Instant delivery in doze mode" checkbox does not work properly ([#211](https://github.com/binwiederhier/ntfy/issues/211))
* Disallow "http" GET/HEAD actions with body ([#221](https://github.com/binwiederhier/ntfy/issues/221), thanks to
  [@cmeis](https://github.com/cmeis) for reporting)
* Action "view" with "clear=true" does not work on some phones ([#220](https://github.com/binwiederhier/ntfy/issues/220), thanks to
  [@cmeis](https://github.com/cmeis) for reporting)
* Do not group foreground service notification with others ([#219](https://github.com/binwiederhier/ntfy/issues/219), thanks to
  [@s-h-a-r-d](https://github.com/s-h-a-r-d) for reporting)

**Additional translations:**

* Czech (thanks to [@waclaw66](https://hosted.weblate.org/user/waclaw66/))
* French (thanks to [@nathanaelhoun](https://hosted.weblate.org/user/nathanaelhoun/))
* Japanese (thanks to [@shak](https://hosted.weblate.org/user/shak/))
* Russian (thanks to [@flamey](https://hosted.weblate.org/user/flamey/) and [@ilya.mikheev.coder](https://hosted.weblate.org/user/ilya.mikheev.coder/))

**Thanks for testing:**

Thanks to [@s-h-a-r-d](https://github.com/s-h-a-r-d) (aka @Shard), [@cmeis](https://github.com/cmeis),
@poblabs, and everyone I forgot for testing.

## ntfy server v1.21.2
Released Apr 24, 2022

In this release, the web app got translation support and was translated into 9 languages already 🇧🇬 🇩🇪 🇺🇸 🌎. 
It also re-adds support for ARMv6, and adds server-side support for Action Buttons. [Action Buttons](https://ntfy.sh/docs/publish/#action-buttons)
is a feature that will be released in the Android app soon. It allows users to add actions to the notifications. 
Limited support is available in the web app.

**Features:**

* Custom notification [action buttons](https://ntfy.sh/docs/publish/#action-buttons) ([#134](https://github.com/binwiederhier/ntfy/issues/134),
  thanks to [@mrherman](https://github.com/mrherman) for reporting)
* Added ARMv6 build ([#200](https://github.com/binwiederhier/ntfy/issues/200), thanks to [@jcrubioa](https://github.com/jcrubioa) for reporting)
* Web app internationalization support 🇧🇬 🇩🇪 🇺🇸 🌎 ([#189](https://github.com/binwiederhier/ntfy/issues/189))

**Bug fixes:**

* Web app: English language strings fixes, additional descriptions for settings ([#203](https://github.com/binwiederhier/ntfy/issues/203), thanks to [@StoyanDimitrov](https://github.com/StoyanDimitrov))
* Web app: Show error message snackbar when sending test notification fails ([#205](https://github.com/binwiederhier/ntfy/issues/205), thanks to [@cmeis](https://github.com/cmeis))
* Web app: basic URL validation in user management ([#204](https://github.com/binwiederhier/ntfy/issues/204), thanks to [@cmeis](https://github.com/cmeis))
* Disallow "http" GET/HEAD actions with body ([#221](https://github.com/binwiederhier/ntfy/issues/221), thanks to
  [@cmeis](https://github.com/cmeis) for reporting)

**Translations (web app):**

* Bulgarian (thanks to [@StoyanDimitrov](https://github.com/StoyanDimitrov))
* German (thanks to [@cmeis](https://github.com/cmeis))
* Indonesian (thanks to [@linerly](https://hosted.weblate.org/user/linerly/))
* Japanese (thanks to [@shak](https://hosted.weblate.org/user/shak/))
* Norwegian Bokmål (thanks to [@comradekingu](https://github.com/comradekingu))
* Russian (thanks to [@flamey](https://hosted.weblate.org/user/flamey/) and [@ilya.mikheev.coder](https://hosted.weblate.org/user/ilya.mikheev.coder/))
* Spanish (thanks to [@rogeliodh](https://github.com/rogeliodh))
* Turkish (thanks to [@ersen](https://ersen.moe/))

**Integrations:**

[Apprise](https://github.com/caronc/apprise) support was fully released in [v0.9.8.2](https://github.com/caronc/apprise/releases/tag/v0.9.8.2)
of Apprise. Thanks to [@particledecay](https://github.com/particledecay) and [@caronc](https://github.com/caronc) for their fantastic work. 
You can try it yourself like this (detailed usage in the [Apprise wiki](https://github.com/caronc/apprise/wiki/Notify_ntfy)):

```
pip3 install apprise
apprise -b "Hi there" ntfys://mytopic
```

## ntfy Android app v1.11.0
Released Apr 7, 2022

**Features:**

* Download attachments to cache folder ([#181](https://github.com/binwiederhier/ntfy/issues/181))
* Regularly delete attachments for deleted notifications ([#142](https://github.com/binwiederhier/ntfy/issues/142))
* Translations to different languages ([#188](https://github.com/binwiederhier/ntfy/issues/188), thanks to
  [@StoyanDimitrov](https://github.com/StoyanDimitrov) for initiating things)

**Bug fixes:**

* IllegalStateException: Failed to build unique file ([#177](https://github.com/binwiederhier/ntfy/issues/177), thanks to [@Fallenbagel](https://github.com/Fallenbagel) for reporting)
* SQLiteConstraintException: Crash during UP registration ([#185](https://github.com/binwiederhier/ntfy/issues/185))
* Refresh preferences screen after settings import (#183, thanks to [@cmeis](https://github.com/cmeis) for reporting)
* Add priority strings to strings.xml to make it translatable (#192, thanks to [@StoyanDimitrov](https://github.com/StoyanDimitrov))

**Translations:**

* English language improvements (thanks to [@comradekingu](https://github.com/comradekingu))
* Bulgarian (thanks to [@StoyanDimitrov](https://github.com/StoyanDimitrov))
* Chinese/Simplified (thanks to [@poi](https://hosted.weblate.org/user/poi) and [@PeterCxy](https://hosted.weblate.org/user/PeterCxy))
* Dutch (*incomplete*, thanks to [@diony](https://hosted.weblate.org/user/diony))
* French (thanks to [@Kusoneko](https://kusoneko.moe/) and [@mlcsthor](https://hosted.weblate.org/user/mlcsthor/))
* German (thanks to [@cmeis](https://github.com/cmeis))
* Italian (thanks to [@theTranslator](https://hosted.weblate.org/user/theTranslator/))
* Indonesian (thanks to [@linerly](https://hosted.weblate.org/user/linerly/))
* Norwegian Bokmål (*incomplete*, thanks to [@comradekingu](https://github.com/comradekingu))
* Portuguese/Brazil (thanks to [@LW](https://hosted.weblate.org/user/LW/))
* Spanish (thanks to [@rogeliodh](https://github.com/rogeliodh))
* Turkish (thanks to [@ersen](https://ersen.moe/))

**Thanks:**

* Many thanks to [@cmeis](https://github.com/cmeis), [@Fallenbagel](https://github.com/Fallenbagel), [@Joeharrison94](https://github.com/Joeharrison94),
  and [@rogeliodh](https://github.com/rogeliodh) for input on the new attachment logic, and for testing the release

## ntfy server v1.20.0
Released Apr 6, 2022

**Features:**:

* Added message bar and publish dialog ([#196](https://github.com/binwiederhier/ntfy/issues/196)) 

**Bug fixes:**

* Added `EXPOSE 80/tcp` to Dockerfile to support auto-discovery in [Traefik](https://traefik.io/) ([#195](https://github.com/binwiederhier/ntfy/issues/195), thanks to [@s-h-a-r-d](https://github.com/s-h-a-r-d))

**Documentation:**

* Added docker-compose example to [install instructions](install.md#docker) ([#194](https://github.com/binwiederhier/ntfy/pull/194), thanks to [@s-h-a-r-d](https://github.com/s-h-a-r-d))

**Integrations:**

* [Apprise](https://github.com/caronc/apprise) has added integration into ntfy ([#99](https://github.com/binwiederhier/ntfy/issues/99), [apprise#524](https://github.com/caronc/apprise/pull/524),
  thanks to [@particledecay](https://github.com/particledecay) and [@caronc](https://github.com/caronc) for their fantastic work)

## ntfy server v1.19.0
Released Mar 30, 2022

**Bug fixes:**

* Do not pack binary with `upx` for armv7/arm64 due to `illegal instruction` errors ([#191](https://github.com/binwiederhier/ntfy/issues/191), thanks to [@iexos](https://github.com/iexos))
* Do not allow comma in topic name in publish via GET endpoint (no ticket)
* Add "Access-Control-Allow-Origin: *" for attachments (no ticket, thanks to @FrameXX)
* Make pruning run again in web app ([#186](https://github.com/binwiederhier/ntfy/issues/186))
* Added missing params `delay` and `email` to publish as JSON body (no ticket)

**Documentation:**

* Improved [e-mail publishing](config.md#e-mail-publishing) documentation

## ntfy server v1.18.1
Released Mar 21, 2022   
_This release ships no features or bug fixes. It's merely a documentation update._

**Documentation:**

* Overhaul of [developer documentation](https://ntfy.sh/docs/develop/)
* PowerShell examples for [publish documentation](https://ntfy.sh/docs/publish/) ([#138](https://github.com/binwiederhier/ntfy/issues/138), thanks to [@Joeharrison94](https://github.com/Joeharrison94))
* Additional examples for [NodeRED, Gatus, Sonarr, Radarr, ...](https://ntfy.sh/docs/examples/) (thanks to [@nickexyz](https://github.com/nickexyz))
* Fixes in developer instructions (thanks to [@Fallenbagel](https://github.com/Fallenbagel) for reporting)

## ntfy Android app v1.10.0
Released Mar 21, 2022

**Features:**

* Support for UnifiedPush 2.0 specification (bytes messages, [#130](https://github.com/binwiederhier/ntfy/issues/130))
* Export/import settings and subscriptions ([#115](https://github.com/binwiederhier/ntfy/issues/115), thanks [@cmeis](https://github.com/cmeis) for reporting)
* Open "Click" link when tapping notification ([#110](https://github.com/binwiederhier/ntfy/issues/110), thanks [@cmeis](https://github.com/cmeis) for reporting)
* JSON stream deprecation banner ([#164](https://github.com/binwiederhier/ntfy/issues/164))

**Bug fixes:**

* Display locale-specific times, with AM/PM or 24h format ([#140](https://github.com/binwiederhier/ntfy/issues/140), thanks [@hl2guide](https://github.com/hl2guide) for reporting)

## ntfy server v1.18.0
Released Mar 16, 2022

**Features:**

* [Publish messages as JSON](https://ntfy.sh/docs/publish/#publish-as-json) ([#133](https://github.com/binwiederhier/ntfy/issues/133), 
  thanks [@cmeis](https://github.com/cmeis) for reporting, thanks to [@Joeharrison94](https://github.com/Joeharrison94) and 
  [@Fallenbagel](https://github.com/Fallenbagel) for testing)

**Bug fixes:**

* rpm: do not overwrite server.yaml on package upgrade ([#166](https://github.com/binwiederhier/ntfy/issues/166), thanks [@waclaw66](https://github.com/waclaw66) for reporting)
* Typo in [ntfy.sh/announcements](https://ntfy.sh/announcements) topic ([#170](https://github.com/binwiederhier/ntfy/pull/170), thanks to [@sandebert](https://github.com/sandebert))
* Readme image URL fixes ([#156](https://github.com/binwiederhier/ntfy/pull/156), thanks to [@ChaseCares](https://github.com/ChaseCares))

**Deprecations:**

* Removed the ability to run server as `ntfy` (as opposed to `ntfy serve`) as per [deprecation](deprecations.md)

## ntfy server v1.17.1
Released Mar 12, 2022

**Bug fixes:**

* Replace `crypto.subtle` with `hashCode` to errors with Brave/FF-Windows (#157, thanks for reporting @arminus)

## ntfy server v1.17.0
Released Mar 11, 2022

**Features & bug fixes:**

* Replace [web app](https://ntfy.sh/app) with a React/MUI-based web app from the 21st century (#111)
* Web UI broken with auth (#132, thanks for reporting @arminus)
* Send static web resources as `Content-Encoding: gzip`, i.e. docs and web app (no ticket)
* Add support for auth via `?auth=...` query param, used by WebSocket in web app (no ticket) 

## ntfy server v1.16.0
Released Feb 27, 2022

**Features & Bug fixes:**

* Add [auth support](https://ntfy.sh/docs/subscribe/cli/#authentication) for subscribing with CLI (#147/#148, thanks @lrabane)
* Add support for [?since=<id>](https://ntfy.sh/docs/subscribe/api/#fetch-cached-messages) (#151, thanks for reporting @nachotp)

**Documentation:**

* Add [watchtower/shoutrr examples](https://ntfy.sh/docs/examples/#watchtower-notifications-shoutrrr) (#150, thanks @rogeliodh)
* Add [release notes](https://ntfy.sh/docs/releases/)

**Technical notes:**

* As of this release, message IDs will be 12 characters long (as opposed to 10 characters). This is to be able to 
  distinguish them from Unix timestamps for #151.

## ntfy Android app v1.9.1
Released Feb 16, 2022

**Features:**

* Share to topic feature (#131, thanks u/emptymatrix for reporting)
* Ability to pick a default server (#127, thanks to @poblabs for reporting and testing)
* Automatically delete notifications (#71, thanks @arjan-s for reporting)
* Dark theme: Improvements around style and contrast (#119, thanks @kzshantonu for reporting)

**Bug fixes:**

* Do not attempt to download attachments if they are already expired (#135)
* Fixed crash in AddFragment as seen per stack trace in Play Console (no ticket)

**Other thanks:**

* Thanks to @rogeliodh, @cmeis and @poblabs for testing

## ntfy server v1.15.0
Released Feb 14, 2022

**Features & bug fixes:**

* Compress binaries with `upx` (#137)
* Add `visitor-request-limit-exempt-hosts` to exempt friendly hosts from rate limits (#144)
* Double default requests per second limit from 1 per 10s to 1 per 5s (no ticket)
* Convert `\n` to new line for `X-Message` header as prep for sharing feature (see #136)
* Reduce bcrypt cost to 10 to make auth timing more reasonable on slow servers (no ticket)
* Docs update to include [public test topics](https://ntfy.sh/docs/publish/#public-topics) (no ticket)

## ntfy server v1.14.1
Released Feb 9, 2022

**Bug fixes:**

* Fix ARMv8 Docker build (#113, thanks to @djmaze)
* No other significant changes

## ntfy Android app v1.8.1
Released Feb 6, 2022

**Features:**

* Support [auth / access control](https://ntfy.sh/docs/config/#access-control) (#19, thanks to @cmeis, @drsprite/@poblabs, 
  @gedw99, @karmanyaahm, @Mek101, @gc-ss, @julianfoad, @nmoseman, Jakob, PeterCxy, Techlosopher)
* Export/upload log now allows censored/uncensored logs (no ticket)
* Removed wake lock (except for notification dispatching, no ticket)
* Swipe to remove notifications (#117)

**Bug fixes:**

* Fix download issues on SDK 29 "Movement not allowed" (#116, thanks Jakob)
* Fix for Android 12 crashes (#124, thanks @eskilop)
* Fix WebSocket retry logic bug with multiple servers (no ticket)
* Fix race in refresh logic leading to duplicate connections (no ticket)
* Fix scrolling issue in subscribe to topic dialog (#131, thanks @arminus)
* Fix base URL text field color in dark mode, and size with large fonts (no ticket)
* Fix action bar color in dark mode (make black, no ticket)

**Notes:**

* Foundational work for per-subscription settings

## ntfy server v1.14.0
Released Feb 3, 2022

**Features**:

* Server-side for [authentication & authorization](https://ntfy.sh/docs/config/#access-control) (#19, thanks for testing @cmeis, and for input from @gedw99, @karmanyaahm, @Mek101, @gc-ss, @julianfoad, @nmoseman, Jakob, PeterCxy, Techlosopher)
* Support `NTFY_TOPIC` env variable in `ntfy publish` (#103)

**Bug fixes**:

* Binary UnifiedPush messages should not be converted to attachments (part 1, #101)

**Docs**:

* Clarification regarding attachments (#118, thanks @xnumad)

## ntfy Android app v1.7.1
Released Jan 21, 2022

**New features:**

* Battery improvements: wakelock disabled by default (#76)
* Dark mode: Allow changing app appearance (#102)
* Report logs: Copy/export logs to help troubleshooting (#94)
* WebSockets (experimental): Use WebSockets to subscribe to topics (#96, #100, #97)
* Show battery optimization banner (#105)

**Bug fixes:**

* (Partial) support for binary UnifiedPush messages (#101)

**Notes:**

* The foreground wakelock is now disabled by default
* The service restarter is now scheduled every 3h instead of every 6h

## ntfy server v1.13.0
Released Jan 16, 2022

**Features:**

* [Websockets](https://ntfy.sh/docs/subscribe/api/#websockets) endpoint
* Listen on Unix socket, see [config option](https://ntfy.sh/docs/config/#config-options) `listen-unix`

## ntfy Android app v1.6.0
Released Jan 14, 2022

**New features:**

* Attachments: Send files to the phone (#25, #15)
* Click action: Add a click action URL to notifications (#85)
* Battery optimization: Allow disabling persistent wake-lock (#76, thanks @MatMaul)
* Recognize imported user CA certificate for self-hosted servers (#87, thanks @keith24)
* Remove mentions of "instant delivery" from F-Droid to make it less confusing (no ticket)

**Bug fixes:**

* Subscription "muted until" was not always respected (#90)
* Fix two stack traces reported by Play console vitals (no ticket)
* Truncate FCM messages >4,000 bytes, prefer instant messages (#84)

## ntfy server v1.12.1
Released Jan 14, 2022

**Bug fixes:**

* Fix security issue with attachment peaking (#93)

## ntfy server v1.12.0
Released Jan 13, 2022

**Features:**

* [Attachments](https://ntfy.sh/docs/publish/#attachments) (#25, #15)
* [Click action](https://ntfy.sh/docs/publish/#click-action) (#85)
* Increase FCM priority for high/max priority messages (#70)

**Bug fixes:**

* Make postinst script work properly for rpm-based systems (#83, thanks @cmeis)
* Truncate FCM messages longer than 4000 bytes (#84)
* Fix `listen-https` port (no ticket)

## ntfy Android app v1.5.2
Released Jan 3, 2022

**New features:**

* Allow using ntfy as UnifiedPush distributor (#9)
* Support for longer message up to 4096 bytes (#77)
* Minimum priority: show notifications only if priority X or higher (#79)
* Allowing disabling broadcasts in global settings (#80)

**Bug fixes:**

* Allow int/long extras for SEND_MESSAGE intent (#57)
* Various battery improvement fixes (#76)

## ntfy server v1.11.2
Released Jan 1, 2022

**Features & bug fixes:**

* Increase message limit to 4096 bytes (4k) #77
* Docs for [UnifiedPush](https://unifiedpush.org) #9
* Increase keepalive interval to 55s #76
* Increase Firebase keepalive to 3 hours #76

## ntfy server v1.10.0
Released Dec 28, 2021

**Features & bug fixes:**

* [Publish messages via e-mail](ntfy.sh/docs/publish/#e-mail-publishing) #66
* Server-side work to support [unifiedpush.org](https://unifiedpush.org) #64
* Fixing the Santa bug #65

## Older releases
For older releases, check out the GitHub releases pages for the [ntfy server](https://github.com/binwiederhier/ntfy/releases)
and the [ntfy Android app](https://github.com/binwiederhier/ntfy-android/releases).
