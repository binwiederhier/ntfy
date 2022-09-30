![ntfy](web/public/static/img/ntfy.png)

---

## 👶 Baby break - My baby girl was born!
Hey folks, my daughter was born on 8/30/22, so I'll be taking some time off from working on ntfy. I'll likely return 
to working on features and bugs in a few weeks. I hope you understand. I posted some pictures in [#387](https://github.com/binwiederhier/ntfy/issues/387) 🥰   

---

# ntfy.sh | Send push notifications to your phone or desktop via PUT/POST
[![Release](https://img.shields.io/github/release/binwiederhier/ntfy.svg?color=success&style=flat-square)](https://github.com/binwiederhier/ntfy/releases/latest)
[![Go Reference](https://pkg.go.dev/badge/heckel.io/ntfy.svg)](https://pkg.go.dev/heckel.io/ntfy)
[![Tests](https://github.com/binwiederhier/ntfy/workflows/test/badge.svg)](https://github.com/binwiederhier/ntfy/actions)
[![Go Report Card](https://goreportcard.com/badge/github.com/binwiederhier/ntfy)](https://goreportcard.com/report/github.com/binwiederhier/ntfy)
[![codecov](https://codecov.io/gh/binwiederhier/ntfy/branch/main/graph/badge.svg?token=A597KQ463G)](https://codecov.io/gh/binwiederhier/ntfy)
[![Discord](https://img.shields.io/discord/874398661709295626?label=Discord)](https://discord.gg/cT7ECsZj9w)
[![Matrix](https://img.shields.io/matrix/ntfy:matrix.org?label=Matrix)](https://matrix.to/#/#ntfy:matrix.org)
[![Matrix space](https://img.shields.io/matrix/ntfy-space:matrix.org?label=Matrix+space)](https://matrix.to/#/#ntfy-space:matrix.org)
[![Healthcheck](https://healthchecks.io/badge/68b65976-b3b0-4102-aec9-980921/kcoEgrLY.svg)](https://ntfy.statuspage.io/)

**ntfy** (pronounce: *notify*) is a simple HTTP-based [pub-sub](https://en.wikipedia.org/wiki/Publish%E2%80%93subscribe_pattern) notification service.
It allows you to **send notifications to your phone or desktop via scripts** from any computer, entirely **without signup or cost**.
It's also open source (as you can plainly see) if you want to run your own.

I run a free version of it at **[ntfy.sh](https://ntfy.sh)**. There's also an [open source Android app](https://github.com/binwiederhier/ntfy-android) (see [Google Play](https://play.google.com/store/apps/details?id=io.heckel.ntfy) or [F-Droid](https://f-droid.org/en/packages/io.heckel.ntfy/)), and an [open source iOS app](https://github.com/binwiederhier/ntfy-ios) (see [App Store](https://apps.apple.com/us/app/ntfy/id1625396347)).

<p>
  <img src="web/public/static/img/screenshot-curl.png" height="180">
  <img src="web/public/static/img/screenshot-web-detail.png" height="180">
  <img src="web/public/static/img/screenshot-phone-main.jpg" height="180">
  <img src="web/public/static/img/screenshot-phone-detail.jpg" height="180">
  <img src="web/public/static/img/screenshot-phone-notification.jpg" height="180">
</p>

## **[Documentation](https://ntfy.sh/docs/)**

[Getting started](https://ntfy.sh/docs/) |
[Android/iOS](https://ntfy.sh/docs/subscribe/phone/) |
[API](https://ntfy.sh/docs/publish/) |
[Install / Self-hosting](https://ntfy.sh/docs/install/) |
[Building](https://ntfy.sh/docs/develop/)

## Chat
You can directly contact me **[on Discord](https://discord.gg/cT7ECsZj9w)** or [on Matrix](https://matrix.to/#/#ntfy:matrix.org) 
(bridged from Discord), or via the [GitHub issues](https://github.com/binwiederhier/ntfy/issues), or find more contact information
[on my website](https://heckel.io/about).

## Announcements / beta testers
For announcements of new releases and cutting-edge beta versions, please subscribe to the [ntfy.sh/announcements](https://ntfy.sh/announcements) 
topic. If you'd like to test the iOS app, join [TestFlight](https://testflight.apple.com/join/P1fFnAm9). For Android betas,
join Discord/Matrix (I'll eventually make a testing channel in Google Play).

## Contributing
I welcome any and all contributions. Just create a PR or an issue. To contribute code, check out 
the [build instructions](https://ntfy.sh/docs/develop/) for the server and the Android app.
Or, if you'd like to help translate 🇩🇪 🇺🇸 🇧🇬, you can start immediately in
[Hosted Weblate](https://hosted.weblate.org/projects/ntfy/).

<a href="https://hosted.weblate.org/engage/ntfy/">
<img src="https://hosted.weblate.org/widgets/ntfy/-/multi-blue.svg" alt="Translation status" />
</a>

## Donations
I have just very recently started accepting donations via [GitHub Sponsors](https://github.com/sponsors/binwiederhier). 
I would be humbled if you helped me carry the server and developer account costs. Even small donations are very much 
appreciated. A big fat Thank You to the folks already sponsoring ntfy:

<a href="https://github.com/aspyct"><img src="https://github.com/aspyct.png" width="40px" /></a>
<a href="https://github.com/codinghipster"><img src="https://github.com/codinghipster.png" width="40px" /></a>
<a href="https://github.com/HinFort"><img src="https://github.com/HinFort.png" width="40px" /></a>
<a href="https://github.com/mckay115"><img src="https://github.com/mckay115.png" width="40px" /></a>
<a href="https://github.com/neutralinsomniac"><img src="https://github.com/neutralinsomniac.png" width="40px" /></a>
<a href="https://github.com/nickexyz"><img src="https://github.com/nickexyz.png" width="40px" /></a>
<a href="https://github.com/qcasey"><img src="https://github.com/qcasey.png" width="40px" /></a>
<a href="https://github.com/Salamafet"><img src="https://github.com/Salamafet.png" width="40px" /></a>

## License
Made with ❤️ by [Philipp C. Heckel](https://heckel.io).   
The project is dual licensed under the [Apache License 2.0](LICENSE) and the [GPLv2 License](LICENSE.GPLv2).

Third party libraries and resources:
* [github.com/urfave/cli](https://github.com/urfave/cli) (MIT) is used to drive the CLI
* [Mixkit sounds](https://mixkit.co/free-sound-effects/notification/) (Mixkit Free License) are used as notification sounds
* [Sounds from notificationsounds.com](https://notificationsounds.com) (Creative Commons Attribution) are used as notification sounds
* [Roboto Font](https://fonts.google.com/specimen/Roboto) (Apache 2.0) is used as a font in everything web
* [React](https://reactjs.org/) (MIT) is used for the web app
* [Material UI components](https://mui.com/) (MIT) are used in the web app
* [MUI dashboard template](https://github.com/mui/material-ui/tree/master/docs/data/material/getting-started/templates/dashboard) (MIT) was used as a basis for the web app
* [Dexie.js](https://github.com/dexie/Dexie.js) (Apache 2.0) is used for web app persistence in IndexedDB
* [GoReleaser](https://goreleaser.com/) (MIT) is used to create releases
* [go-smtp](https://github.com/emersion/go-smtp) (MIT) is used to receive e-mails
* [stretchr/testify](https://github.com/stretchr/testify) (MIT) is used for unit and integration tests
* [github.com/mattn/go-sqlite3](https://github.com/mattn/go-sqlite3) (MIT) is used to provide the persistent message cache
* [Firebase Admin SDK](https://github.com/firebase/firebase-admin-go) (Apache 2.0) is used to send FCM messages
* [github/gemoji](https://github.com/github/gemoji) (MIT) is used for emoji support (specifically the [emoji.json](https://raw.githubusercontent.com/github/gemoji/master/db/emoji.json) file)
* [Lightbox with vanilla JS](https://yossiabramov.com/blog/vanilla-js-lightbox) as a lightbox on the landing page 
* [HTTP middleware for gzip compression](https://gist.github.com/CJEnright/bc2d8b8dc0c1389a9feeddb110f822d7) (MIT) is used for serving static files
* [Regex for auto-linking](https://github.com/bryanwoods/autolink-js) (MIT) is used to highlight links (the library is not used)
* [Statically linking go-sqlite3](https://www.arp242.net/static-go.html)
* [Linked tabs in mkdocs](https://facelessuser.github.io/pymdown-extensions/extensions/tabbed/#linked-tabs)
