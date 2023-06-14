# Release notes
Binaries for all releases can be found on the GitHub releases pages for the [ntfy server](https://github.com/binwiederhier/ntfy/releases)
and the [ntfy Android app](https://github.com/binwiederhier/ntfy-android/releases).

## ntfy server v2.5.0
Released May 18, 2023

This release brings a number of new features, including support for text-to-speech style [phone calls](publish.md#phone-calls), 
an admin API to manage users and ACL (currently in beta, and hence undocumented), and support for authorized access to 
upstream servers via the `upstream-access-token` config option.

❤️ If you like ntfy, **please consider sponsoring me** via [GitHub Sponsors](https://github.com/sponsors/binwiederhier)
and [Liberapay](https://en.liberapay.com/ntfy/), or by buying a [paid plan via the web app](https://ntfy.sh/app) (20% off
if you use promo code `MYTOPIC`). ntfy will always remain open source.

**Features:**

* Support for text-to-speech style [phone calls](publish.md#phone-calls) using the `X-Call` header (no ticket)
* Admin API to manage users and ACL, `v1/users` + `v1/users/access` (intentionally undocumented as of now, [#722](https://github.com/binwiederhier/ntfy/issues/722), thanks to [@CreativeWarlock](https://github.com/CreativeWarlock) for sponsoring this ticket)
* Added `upstream-access-token` config option to allow authorized access to upstream servers (no ticket)

**Bug fixes + maintenance:**

* Removed old ntfy website from ntfy entirely (no ticket)
* Make emoji lookup for emails more efficient ([#725](https://github.com/binwiederhier/ntfy/pull/725), thanks to [@adamantike](https://github.com/adamantike))
* Fix potential subscriber ID clash ([#712](https://github.com/binwiederhier/ntfy/issues/712), thanks to [@peterbourgon](https://github.com/peterbourgon) for reporting, and [@dropdevrahul](https://github.com/dropdevrahul) for fixing)
* Support for `quoted-printable` in incoming emails ([#719](https://github.com/binwiederhier/ntfy/pull/719), thanks to [@Aerion](https://github.com/Aerion))
* Attachments with filenames that are downloaded using a browser will now download with the proper filename ([#726](https://github.com/binwiederhier/ntfy/issues/726), thanks to [@un99known99](https://github.com/un99known99) for reporting, and [@wunter8](https://github.com/wunter8) for fixing)
* Fix web app i18n issue in account preferences ([#730](https://github.com/binwiederhier/ntfy/issues/730), thanks to [@codebude](https://github.com/codebude) for reporting)

## ntfy server v2.4.0
Released Apr 26, 2023

This release adds a tiny `v1/stats` endpoint to expose how many messages have been published, and adds suport to encode the `X-Title`,
`X-Message` and `X-Tags` header as RFC 2047. It's a pretty small release, and mainly enables the release of the new ntfy.sh website.

❤️ If you like ntfy, **please consider sponsoring me** via [GitHub Sponsors](https://github.com/sponsors/binwiederhier)
and [Liberapay](https://en.liberapay.com/ntfy/), or by buying a [paid plan via the web app](https://ntfy.sh/app). ntfy
will always remain open source.

**Features:**

* [ntfy CLI](subscribe/cli.md) (`ntfy publish` and `ntfy subscribe` only) can now be installed via Homebrew (thanks to [@Moulick](https://github.com/Moulick))
* Added `v1/stats` endpoint to expose messages stats (no ticket)
* Support [RFC 2047](https://datatracker.ietf.org/doc/html/rfc2047#section-2) encoded headers (no ticket, honorable mention to [mqttwarn](https://github.com/jpmens/mqttwarn/pull/638) and [@amotl](https://github.com/amotl))

**Bug fixes + maintenance:**

* Hide country flags on Windows ([#606](https://github.com/binwiederhier/ntfy/issues/606), thanks to [@cmeis](https://github.com/cmeis) for reporting, and to [@pokej6](https://github.com/pokej6) for fixing it)
* `ntfy sub` now uses default auth credentials as defined in `client.yml` ([#698](https://github.com/binwiederhier/ntfy/issues/698), thanks to [@CrimsonFez](https://github.com/CrimsonFez) for reporting, and to [@wunter8](https://github.com/wunter8) for fixing it)

**Documentation:**

* Updated PowerShell examples ([#697](https://github.com/binwiederhier/ntfy/pull/697), thanks to [@Natfan](https://github.com/Natfan))

**Additional languages:**

* Swedish (thanks to [@hellbown](https://hosted.weblate.org/user/Shjosan/))

## ntfy server v2.3.1 
Released March 30, 2023

This release disables server-initiated polling of iOS devices entirely, thereby eliminating the thundering herd problem
on ntfy.sh that we observe every 20 minutes. The polling was never strictly necessary, and has actually caused duplicate
delivery issues as well, so disabling it should not have any negative effects. iOS users, please reach out via Discord
or Matrix if there are issues.

**Bug fixes + maintenance:**

* Disable iOS polling entirely ([#677](https://github.com/binwiederhier/ntfy/issues/677)/[#509](https://github.com/binwiederhier/ntfy/issues/509))

## ntfy server v2.3.0
Released March 29, 2023

This release primarily fixes an issue with delayed messages, and it adds support for Go's profiler (if enabled), which
will allow investigating usage spikes in more detail. There will likely be a follow-up release this week to fix the
actual spikes [caused by iOS devices](https://github.com/binwiederhier/ntfy/issues/677).

**Features:**

* ntfy now supports Go's `pprof` profiler, if enabled (relates to [#677](https://github.com/binwiederhier/ntfy/issues/677))

**Bug fixes + maintenance:**

* Fix delayed message sending from authenticated users ([#679](https://github.com/binwiederhier/ntfy/issues/679))
* Fixed plural for Polish and other translations ([#678](https://github.com/binwiederhier/ntfy/pull/678), thanks to [@bmoczulski](https://github.com/bmoczulski))

## ntfy server v2.2.0
Released March 17, 2023

With this release, ntfy is now able to expose metrics via a `/metrics` endpoint for [Prometheus](https://prometheus.io/), if enabled.
The endpoint exposes about 20 different counters and gauges, from the number of published messages and emails, to active subscribers,
visitors and topics. If you'd like more metrics, pop in the Discord/Matrix or file an issue on GitHub. 

On top of this, you can now use access tokens in the ntfy CLI (defined in the `client.yml` file), fixed a bug in `ntfy subscribe`,
removed the dependency on Google Fonts, and more.

🔥 Reminder: Purchase one of three **ntfy Pro plans** for **50% off** for a limited time (if you use promo code `MYTOPIC`). 
ntfy Pro gives you higher rate limits and lets you reserve topic names. [Buy through web app](https://ntfy.sh/app).

❤️ If you don't need ntfy Pro, please consider sponsoring ntfy via [GitHub Sponsors](https://github.com/sponsors/binwiederhier)
and [Liberapay](https://en.liberapay.com/ntfy/). ntfy will stay open source forever.

**Features:**

* Monitoring: ntfy now exposes a `/metrics` endpoint for [Prometheus](https://prometheus.io/) if [configured](config.md#monitoring) ([#210](https://github.com/binwiederhier/ntfy/issues/210), thanks to [@rogeliodh](https://github.com/rogeliodh) for reporting)
* You can now use tokens in `client.yml` for publishing and subscribing ([#653](https://github.com/binwiederhier/ntfy/issues/653), thanks to [@wunter8](https://github.com/wunter8))

**Bug fixes + maintenance:**

* `ntfy sub --poll --from-config` will now include authentication headers from client.yml (if applicable) ([#658](https://github.com/binwiederhier/ntfy/issues/658), thanks to [@wunter8](https://github.com/wunter8))
* Docs: Removed dependency on Google Fonts in docs ([#554](https://github.com/binwiederhier/ntfy/issues/554), thanks to [@bt90](https://github.com/bt90) for reporting, and [@ozskywalker](https://github.com/ozskywalker) for implementing)
* Increase allowed auth failure attempts per IP address to 30 (no ticket)
* Web app: Increase maximum incremental backoff retry interval to 2 minutes (no ticket)

**Documentation:**

* Make query parameter description more clear ([#630](https://github.com/binwiederhier/ntfy/issues/630), thanks to [@bbaa-bbaa](https://github.com/bbaa-bbaa) for reporting, and to [@wunter8](https://github.com/wunter8) for a fix)

## ntfy server v2.1.2
Released March 4, 2023

This is a hotfix release, mostly to combat the ridiculous amount of Matrix requests with invalid/dead pushkeys, and the
corresponding HTTP 507 responses the ntfy.sh server is sending out. We're up to >600k HTTP 507 responses per day 🤦. This 
release solves this issue by rejecting Matrix pushkeys, if nobody has subscribed to the corresponding topic for 12 hours.

The release furthermore reverts the default rate limiting behavior for UnifiedPush to be publisher-based, and introduces
a flag to enable [subscriber-based rate limiting](config.md#subscriber-based-rate-limiting) for high volume servers.

**Features:**

* Support SMTP servers without auth ([#645](https://github.com/binwiederhier/ntfy/issues/645), thanks to [@Sharknoon](https://github.com/Sharknoon) for reporting)

**Bug fixes + maintenance:**

* Token auth doesn't work if default user credentials are defined in `client.yml` ([#650](https://github.com/binwiederhier/ntfy/issues/650), thanks to [@Xinayder](https://github.com/Xinayder))
* Add `visitor-subscriber-rate-limiting` flag to allow enabling [subscriber-based rate limiting](config.md#subscriber-based-rate-limiting) (off by default now, [#649](https://github.com/binwiederhier/ntfy/issues/649)/[#655](https://github.com/binwiederhier/ntfy/pull/655), thanks to [@barathrm](https://github.com/barathrm) for reporting, and to [@karmanyaahm](https://github.com/karmanyaahm) and [@p1gp1g](https://github.com/p1gp1g) for help with the design)
* Reject Matrix pushkey after 12 hours of inactivity on a topic, if `visitor-subscriber-rate-limiting` is enabled ([#643](https://github.com/binwiederhier/ntfy/pull/643), thanks to [@karmanyaahm](https://github.com/karmanyaahm) and [@p1gp1g](https://github.com/p1gp1g) for help with the design)  

**Additional languages:**

* Danish (thanks to [@Andersbiha](https://hosted.weblate.org/user/Andersbiha/))

## ntfy server v2.1.1
Released March 1, 2023

This is a tiny release with a few bug fixes, but it's big for me personally. After almost three months of work, 
**today I am finally launching the paid plans on ntfy.sh** 🥳 🎉. 

You are now able to purchase one of three plans that'll give you **higher rate limits** (messages, emails, attachment sizes, ...), 
as well as the ability to **reserve topic names** for your personal use, while at the same time supporting me and the
ntfy open source project ❤️. You can check out the pricing, and [purchase plans through the web app](https://ntfy.sh/app) (use
promo code `MYTOPIC` for a **50% discount**, limited time only).

And as I've said many times: Do not worry. **ntfy will always stay open source**, and that includes all features. There
are no closed-source features. So if you'd like to run your own server, you can!

**Bug fixes + maintenance:**

* Fix panic when using Firebase without users ([#641](https://github.com/binwiederhier/ntfy/issues/641), thanks to [u/heavybell](https://www.reddit.com/user/heavybell/) for reporting)
* Remove health check from `Dockerfile` and [document it](config.md#health-checks) ([#635](https://github.com/binwiederhier/ntfy/issues/635), thanks to [@Andersbiha](https://github.com/Andersbiha)) 
* Upgrade dialog: Disable submit button for free tier (no ticket)
* Allow multiple `log-level-overrides` on the same field (no ticket)
* Actually remove `ntfy publish --env-topic` flag (as per [deprecations](deprecations.md), no ticket)
* Added `billing-contact` config option (no ticket)

## ntfy server v2.1.0
Released February 25, 2023

This release changes the way UnifiedPush (UP) topics are rate limited from publisher-based rate limiting to subscriber-based
rate limiting. This allows UP application servers to send higher volumes, since the subscribers carry the rate limits.
However, it also means that UP clients have to subscribe to a topic first before they are allowed to publish. If they do
no, clients will receive an HTTP 507 response from the server.

We also fixed another issue with UnifiedPush: Some Mastodon servers were sending unsupported `Authorization` headers, 
which ntfy rejected with an HTTP 401. We now ignore unsupported header values. 

As of this release, ntfy also supports sending emails to protected topics, and it ships code to support annual billing
cycles (not live yet).

As part of this release, I also enabled sign-up and login (free accounts only), and I also started reducing the rate 
limits for anonymous & free users a bit. With the next release and the launch of the paid plan, I'll reduce the limits
a bit more. For 90% of users, you should not feel the difference.

**Features:**

* UnifiedPush: Subscriber-based rate limiting for `up*` topics ([#584](https://github.com/binwiederhier/ntfy/pull/584)/[#609](https://github.com/binwiederhier/ntfy/pull/609)/[#633](https://github.com/binwiederhier/ntfy/pull/633), thanks to [@karmanyaahm](https://github.com/karmanyaahm))
* Support for publishing to protected topics via email with access tokens ([#612](https://github.com/binwiederhier/ntfy/pull/621), thanks to [@tamcore](https://github.com/tamcore))
* Support for base64-encoded and nested multipart emails ([#610](https://github.com/binwiederhier/ntfy/issues/610), thanks to [@Robert-litts](https://github.com/Robert-litts))
* Payments: Add support for annual billing intervals (no ticket)

**Bug fixes + maintenance:**

* Web: Do not disable "Reserve topic" checkbox for admins (no ticket, thanks to @xenrox for reporting)
* UnifiedPush: Treat non-Basic/Bearer `Authorization` header like header was not sent ([#629](https://github.com/binwiederhier/ntfy/issues/629), thanks to [@Boebbele](https://github.com/Boebbele) and [@S1m](https://github.com/S1m) for reporting)

**Documentation:**

* Added example for [Traccar](https://ntfy.sh/docs/examples/#traccar) ([#631](https://github.com/binwiederhier/ntfy/pull/631), thanks to [tamcore](https://github.com/tamcore))

**Additional languages:**

* Arabic (thanks to [@ButterflyOfFire](https://hosted.weblate.org/user/ButterflyOfFire/))

## ntfy server v2.0.1
Released February 17, 2023

This is a quick bugfix release to address a panic that happens when `attachment-cache-dir` is not set.

**Bug fixes + maintenance:**

* Avoid panic in manager when `attachment-cache-dir` is not set ([#617](https://github.com/binwiederhier/ntfy/issues/617), thanks to [@ksurl](https://github.com/ksurl))  
* Ensure that calls to standard logger `log.Println` also output JSON (no ticket)

## ntfy server v2.0.0
Released February 16, 2023

This is the biggest ntfy server release I've ever done 🥳 . Lots of new and exciting features. 

**Brand-new features:**

* **User signup/login & account sync**: If enabled, users can now register to create a user account, and then login to 
  the web app. Once logged in, topic subscriptions and user settings are stored server-side in the user account (as 
  opposed to only in the browser storage). So far, this is implemented only in the web app only. Once it's in the Android/iOS
  app, you can easily keep your account in sync. Relevant [config options](config.md#config-options) are `enable-signup` and 
  `enable-login`.
  <div id="account-screenshots" class="screenshots">
    <a href="../../static/img/web-signup.png"><img src="../../static/img/web-signup.png"/></a>
    <a href="../../static/img/web-account.png"><img src="../../static/img/web-account.png"/></a>
  </div>
* **Topic reservations** 🎉: If enabled, users can now **reserve topics and restrict access to other users**.
  Once this is fully rolled out, you may reserve `ntfy.sh/philbackups` and define access so that only you can publish/subscribe
  to the topic. Reservations let you claim ownership of a topic, and you can define access permissions for others as
  `deny-all` (only you have full access), `read-only` (you can publish/subscribe, others can subscribe), `write-only` (you 
  can publish/subscribe, others can publish), `read-write` (everyone can publish/subscribe, but you remain the owner).
  Topic reservations can be [configured](config.md#config-options) in the web app if `enable-reservations` is enabled, and 
  only if the user has a [tier](config.md#tiers) that supports reservations.
  <div id="reserve-screenshots" class="screenshots">
    <a href="../../static/img/web-reserve-topic.png"><img src="../../static/img/web-reserve-topic.png"/></a> 
    <a href="../../static/img/web-reserve-topic-dialog.png"><img src="../../static/img/web-reserve-topic-dialog.png"/></a>
  </div>
* **Access tokens:** It is now possible to create user access tokens for a user account. Access tokens are useful
  to avoid having to paste your password to various applications or scripts. For instance, you may want to use a 
  dedicated token to publish from your backup host, and one from your home automation system. Tokens can be configured
  in the web app, or via the `ntfy token` command. See [creating tokens](config.md#access-tokens),
  and [publishing using tokens](publish.md#access-tokens).
  <div id="token-screenshots" class="screenshots">
    <a href="../../static/img/web-token-create.png"><img src="../../static/img/web-token-create.png"/></a> 
    <a href="../../static/img/web-token-list.png"><img src="../../static/img/web-token-list.png"/></a>
  </div>
* **Structured logging:** I've redone a lot of the logging to make it more structured, and to make it easier to debug and
  troubleshoot. Logs can now be written to a file, and as JSON (if configured). Each log event carries context fields
  that you can filter and search on using tools like `jq`. On top of that, you can override the log level if certain fields
  match. For instance, you can say `user_name=phil -> debug` to log everything related to a certain user with debug level.
  See [logging & debugging](config.md#logging-debugging).
* **Tiers:** You can now define and associate usage tiers to users. Tiers can be used to grant users higher limits, such as
  daily message limits, attachment size, or make it possible for users to reserve topics. You could, for instance, have
  a tier `Standard` that allows 500 messages/day, 15 MB attachments and 5 allowed topic reservations, and another
  tier `Friends & Family` with much higher limits. For ntfy.sh, I'll mostly use these tiers to facilitate paid plans (see below).
  Tiers can be configured via the `ntfy tier ...` command. See [tiers](config.md#tiers).
* **Paid tiers:** Starting very soon, I will be offering paid tiers for ntfy.sh on top of the free service. You'll be
  able to subscribe to tiers with higher rate limits (more daily messages, bigger attachments) and topic reservations.
  Paid tiers are facilitated by integrating [Stripe](https://stripe.com) as a payment provider. See [payments](config.md#payments)
  for details.

**ntfy is forever open source!**   
Yes, I will be offering some paid plans. But you don't need to panic! I won't be taking any features away, and everything 
will remain forever open source, so you can self-host if you like. Similar to the donations via [GitHub Sponsors](https://github.com/sponsors/binwiederhier)
and [Liberapay](https://en.liberapay.com/ntfy/), paid plans will help pay for the service and keep me motivated to keep
going. It'll only make ntfy better.

**Other tickets:**

* User account signup, login, topic reservations, access tokens, tiers etc. ([#522](https://github.com/binwiederhier/ntfy/issues/522))
* `OPTIONS` method calls are not serviced when the UI is disabled ([#598](https://github.com/binwiederhier/ntfy/issues/598), thanks to [@enticedwanderer](https://github.com/enticedwanderer) for reporting)

**Special thanks:**

A big Thank-you goes to everyone who tested the user account and payments work. I very much appreciate all the feedback,
suggestions, and bug reports. Thank you, @nwithan8, @deadcade, @xenrox, @cmeis, @wunter8 and the others who I forgot.

## ntfy server v1.31.0
Released February 14, 2023

This is a tiny release before the really big release, and also the last before the big v2.0.0. The most interesting 
things in this release are the new preliminary health endpoint to allow monitoring in K8s (and others), and the removal
of `upx` binary packing (which was causing erroneous virus flagging). Aside from that, the `go-smtp` library did a 
breaking-change upgrade, which required some work to get working again.

**Features:**

* Preliminary `/v1/health` API endpoint for service monitoring (no ticket)
* Add basic health check to `Dockerfile` ([#555](https://github.com/binwiederhier/ntfy/pull/555), thanks to [@bt90](https://github.com/bt90))

**Bug fixes + maintenance:**

* Fix `chown` issues with RHEL-like based systems ([#566](https://github.com/binwiederhier/ntfy/issues/566)/[#565](https://github.com/binwiederhier/ntfy/pull/565), thanks to [@danieldemus](https://github.com/danieldemus))
* Removed `upx` (binary packing) for all builds due to false virus warnings ([#576](https://github.com/binwiederhier/ntfy/issues/576), thanks to [@shawnhwei](https://github.com/shawnhwei) for reporting)
* Upgraded `go-smtp` library and tests to v0.16.0 ([#569](https://github.com/binwiederhier/ntfy/issues/569))

**Documentation:**

* Add HTTP/2 and TLSv1.3 support to nginx docs ([#553](https://github.com/binwiederhier/ntfy/issues/553), thanks to [@bt90](https://github.com/bt90))
* Small wording change for `client.yml` ([#562](https://github.com/binwiederhier/ntfy/pull/562), thanks to [@fleopaulD](https://github.com/fleopaulD))
* Fix K8s install docs ([#582](https://github.com/binwiederhier/ntfy/pull/582), thanks to [@Remedan](https://github.com/Remedan))
* Updated Jellyseer docs ([#604](https://github.com/binwiederhier/ntfy/pull/604), thanks to [@Y0ngg4n](https://github.com/Y0ngg4n))
* Updated iOS developer docs ([#605](https://github.com/binwiederhier/ntfy/pull/605), thanks to [@SticksDev](https://github.com/SticksDev))

**Additional languages:**

* Portuguese (thanks to [@ssantos](https://hosted.weblate.org/user/ssantos/))

## ntfy server v1.30.1
Released December 23, 2022 🎅

This is a special holiday edition version of ntfy, with all sorts of holiday fun and games, and hidden quests.
Nahh, just kidding. This release is an intermediate release mainly to eliminate warnings in the logs, so I can
roll out the TLSv1.3, HTTP/2 and Unix mode changes on ntfy.sh (see [#552](https://github.com/binwiederhier/ntfy/issues/552)).

**Features:**

* Web: Generate random topic name button ([#453](https://github.com/binwiederhier/ntfy/issues/453), thanks to [@yardenshoham](https://github.com/yardenshoham))
* Add [Gitpod config](https://github.com/binwiederhier/ntfy/blob/main/.gitpod.yml) ([#540](https://github.com/binwiederhier/ntfy/pull/540), thanks to [@yardenshoham](https://github.com/yardenshoham)) 

**Bug fixes + maintenance:**

* Remove `--env-topic` option from `ntfy publish` as per [deprecation](deprecations.md) (no ticket)
* Prepared statements for message cache writes ([#542](https://github.com/binwiederhier/ntfy/pull/542), thanks to [@nicois](https://github.com/nicois))
* Do not warn about invalid IP address when behind proxy in unix socket mode (relates to [#552](https://github.com/binwiederhier/ntfy/issues/552))
* Upgrade nginx/ntfy config on ntfy.sh to work with TLSv1.3, HTTP/2 ([#552](https://github.com/binwiederhier/ntfy/issues/552), thanks to [@bt90](https://github.com/bt90))

## ntfy Android app v1.16.0
Released December 11, 2022

This is a feature and platform/dependency upgrade release. You can now have per-subscription notification settings
(including sounds, DND, etc.), and you can make notifications continue ringing until they are dismissed. There's also
support for thematic/adaptive launcher icon for Android 13.

There are a few more Android 13 specific things, as well as many bug fixes: No more crashes from large images, no more
opening the wrong subscription, and we also fixed the icon color issue.

**Features:**

* Custom per-subscription notification settings incl. sounds, DND, etc. ([#6](https://github.com/binwiederhier/ntfy/issues/6), thanks to [@doits](https://github.com/doits))
* Insistent notifications that ring until dismissed ([#417](https://github.com/binwiederhier/ntfy/issues/417), thanks to [@danmed](https://github.com/danmed) for reporting)
* Add thematic/adaptive launcher icon ([#513](https://github.com/binwiederhier/ntfy/issues/513), thanks to [@daedric7](https://github.com/daedric7) for reporting)

**Bug fixes + maintenance:**

* Upgrade Android dependencies and build toolchain to SDK 33 (no ticket)
* Simplify F-Droid build: Disable tasks for Google Services ([#516](https://github.com/binwiederhier/ntfy/issues/516), thanks to [@markosopcic](https://github.com/markosopcic))
* Android 13: Ask for permission to post notifications ([#508](https://github.com/binwiederhier/ntfy/issues/508))
* Android 13: Do not allow swiping away the foreground notification ([#521](https://github.com/binwiederhier/ntfy/issues/521), thanks to [@alexhorner](https://github.com/alexhorner) for reporting)
* Android 5 (SDK 21): Fix crash on unsubscribing ([#528](https://github.com/binwiederhier/ntfy/issues/528), thanks to Roger M.)
* Remove timestamp when copying message text ([#471](https://github.com/binwiederhier/ntfy/issues/471), thanks to [@wunter8](https://github.com/wunter8))
* Fix auto-delete if some icons do not exist anymore ([#506](https://github.com/binwiederhier/ntfy/issues/506))
* Fix notification icon color ([#480](https://github.com/binwiederhier/ntfy/issues/480), thanks to [@s-h-a-r-d](https://github.com/s-h-a-r-d) for reporting)
* Fix topics do not re-subscribe to Firebase after restoring from backup ([#511](https://github.com/binwiederhier/ntfy/issues/511))
* Fix crashes from large images ([#474](https://github.com/binwiederhier/ntfy/issues/474), thanks to [@daedric7](https://github.com/daedric7) for reporting)
* Fix notification click opens wrong subscription ([#261](https://github.com/binwiederhier/ntfy/issues/261), thanks to [@SMAW](https://github.com/SMAW) for reporting)
* Fix Firebase-only "link expired" issue ([#529](https://github.com/binwiederhier/ntfy/issues/529))
* Remove "Install .apk" feature in Google Play variant due to policy change ([#531](https://github.com/binwiederhier/ntfy/issues/531))
* Add donate button (no ticket)

**Additional translations:**

* Korean (thanks to [@YJSofta0f97461d82447ac](https://hosted.weblate.org/user/YJSofta0f97461d82447ac/))
* Portuguese (thanks to [@victormagalhaess](https://hosted.weblate.org/user/victormagalhaess/))

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

## Not released yet

### ntfy Android app v1.16.1 (UNRELEASED)

**Features:**

* You can now disable UnifiedPush so ntfy does not act as a UnifiedPush distributor ([#646](https://github.com/binwiederhier/ntfy/issues/646), thanks to [@ollien](https://github.com/ollien) for reporting and to [@wunter8](https://github.com/wunter8) for implementing) 

**Bug fixes + maintenance:**

* UnifiedPush subscriptions now include the `Rate-Topics` header to facilitate subscriber-based billing ([#652](https://github.com/binwiederhier/ntfy/issues/652), thanks to [@wunter8](https://github.com/wunter8))
* Subscriptions without icons no longer appear to use another subscription's icon ([#634](https://github.com/binwiederhier/ntfy/issues/634), thanks to [@topcaser](https://github.com/topcaser) for reporting and to [@wunter8](https://github.com/wunter8) for fixing)
* Bumped all dependencies to the latest versions (no ticket)

**Additional languages:**

* Swedish (thanks to [@hellbown](https://hosted.weblate.org/user/hellbown/))

### ntfy server v2.6.0 (UNRELEASED)

**Features:**

* The web app now supports web push, and is installable on Chrome, Edge, Android, and iOS. Look at the [web app docs](https://docs.ntfy.sh/subscribe/web/) for more information ([#751](https://github.com/binwiederhier/ntfy/pull/751), thanks to [@nimbleghost](https://github.com/nimbleghost))

**Bug fixes:**

* Support encoding any header as RFC 2047 ([#737](https://github.com/binwiederhier/ntfy/issues/737), thanks to [@cfouche3005](https://github.com/cfouche3005) for reporting)
* Do not forward poll requests for UnifiedPush messages (no ticket, thanks to NoName for reporting)
* Fix `ntfy pub %` segfaulting ([#760](https://github.com/binwiederhier/ntfy/issues/760), thanks to [@clesmian](https://github.com/clesmian) for reporting)
* Newly created access tokens are now lowercase only to fully support `<topic>+<token>@<domain>` email syntax ([#773](https://github.com/binwiederhier/ntfy/issues/773), thanks to gingervitiz for reporting)

**Maintenance:**

* Improved GitHub Actions flow ([#745](https://github.com/binwiederhier/ntfy/pull/745), thanks to [@nimbleghost](https://github.com/nimbleghost))
* Web: Add JS formatter "prettier" ([#746](https://github.com/binwiederhier/ntfy/pull/746), thanks to [@nimbleghost](https://github.com/nimbleghost))
* Web: Add eslint with eslint-config-airbnb ([#748](https://github.com/binwiederhier/ntfy/pull/748), thanks to [@nimbleghost](https://github.com/nimbleghost))
* Web: Switch to Vite ([#749](https://github.com/binwiederhier/ntfy/pull/749), thanks to [@nimbleghost](https://github.com/nimbleghost))
