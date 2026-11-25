# iOS app internals

How the iOS app is structured: its two targets, the shared Core Data store, subscriptions and polling. Code is in
[ntfy-ios](https://github.com/binwiederhier/ntfy-ios); `ntfy/`, `ntfyNSE/` and `ntfyTests/` paths are there,
`server/` paths are in this repo.

How a message gets from a server to the phone (FCM, APNs, `poll_request`, `upstream-base-url`) is covered in
[ios-push-architecture.md](ios-push-architecture.md) and not repeated here. See
[web-app.md](web-app.md#cross-client-feature-matrix) for how iOS compares to the other clients.

## Overview

| Item | Value |
|---|---|
| Language / UI | Swift, SwiftUI, Combine |
| Deployment target | iOS 15.0 (the project-level default is still 14.0, and some `#available(iOS 15.0, *)` checks remain) |
| Dependency | Firebase iOS SDK via SwiftPM (`FirebaseMessaging`), the only one |
| Server | `APP_BASE_URL = "https://ntfy.sh"` build setting, exposed as `AppBaseURL` in both `Info.plist` files and read by `Config.appBaseUrl` (`ntfy/Utils/Config.swift`) |
| Firebase config | `GoogleService-Info.plist` is not in the repo (see `docs/GETTING_STARTED.md`); CI writes a dummy one (`scripts/dummy-firebase-plist.sh`) |
| Tests | `ntfyTests/` (Swift Testing), run with SwiftLint in `.github/workflows/ci.yml`; conventions in `docs/dev/testing.md` |

## Targets

| Target | Type | Bundle ID | Role |
|---|---|---|---|
| `ntfy` | app | `io.heckel.ntfy` | UI, subscriptions, FCM registration, polling, `~poll` background pushes, notification taps |
| `ntfyNSE` | notification service extension | `io.heckel.ntfy.ntfyNSE` | runs in its own process for each alert push; saves the message, fetches it for `poll_request`, rewrites the notification |
| `ntfyTests` | unit tests | `io.heckel.ntfyTests` | `HelpersTests`, `MessageTests`, `NotificationContentTests`, `StoreTests` |

Both app and extension carry the App Group `group.io.heckel.ntfy` and the critical alerts entitlement
(`ntfy/ntfy.entitlements`, `ntfyNSE/ntfyNSE.entitlements`). Both `Info.plist` files set `NSAllowsArbitraryLoads`,
because self-hosted servers may be plain HTTP. The only background mode is `remote-notification`; there is no
`BGTaskScheduler` use.

### App layout

| Path | Contents |
|---|---|
| `ntfy/App/` | `AppMain.swift` (entry), `AppDelegate.swift` (Firebase, APNs, notification delegate, `~poll`) |
| `ntfy/Persistence/` | `Store.swift` (Core Data), `SubscriptionManager.swift`, `Notification.swift` (wire types `Message`, `MessageAttachment`, `Action`), `SubscriptionsObservable` / `NotificationsObservable`, the model in `ntfy.xcdatamodeld/` |
| `ntfy/Utils/` | `ApiService.swift` (HTTP), `NotificationContent.swift` (builds `UNNotificationContent`), `Helpers.swift` (URLs, topic hashing), `Actions.swift` / `ActionExecutor.swift`, `AttachmentFileStore.swift`, `DownloadDelegate.swift`, `Log.swift` |
| `ntfy/Views/` | `ContentView` -> `MainView` (tabs): `SubscriptionListView`, `NotificationListView`, `SettingsView` and friends |
| `ntfyNSE/` | `NotificationService.swift` |

Lists are driven by `NSFetchedResultsController` wrappers (`SubscriptionsObservable`, `NotificationsObservable`);
`@FetchRequest` appears only in a few settings views.

## Core Data store shared by app and NSE

`ntfy/Persistence/Store.swift`, singleton `Store.shared`. `Store.init` opens `ntfy.sqlite` in the App Group container
(or `/dev/null` for `Store(inMemory: true)` in tests and previews), so the app and the NSE read and write the same
file from two processes.

How the two processes stay in sync:

- `NSPersistentStoreRemoteChangeNotificationPostOptionKey` is enabled, and the app listens for
  `.NSPersistentStoreRemoteChange` and calls `hardRefresh()` (toggles `stalenessInterval` and refreshes all objects).
  The code comment calls this a hack.
- `automaticallyMergesChangesFromParent = true` with a `mergeByPropertyStoreTrumpMergePolicyType` policy.
- `transactionAuthor` is `ntfy.appex` in the extension and `ntfy` in the app.
- `rollbackAndRefresh()` exists because a failed save usually means the NSE stored the same notification first.
- Lightweight migration (`shouldMigrateStoreAutomatically`, `shouldInferMappingModelAutomatically`); there is a
  single model version.

Besides Core Data, the App Group holds a shared `UserDefaults` (`criticalAlertsAuthorized`, so the NSE can check it
without asking `UNUserNotificationCenter`) and downloaded attachments under `attachments/`
(`AttachmentFileStore.swift`).

### Entities (`Model.xcdatamodel/contents`)

| Entity | Unique on | Fields |
|---|---|---|
| `Subscription` | `(baseUrl, topic)` | `baseUrl`, `topic` (regex `^[-_A-Za-z0-9]{1,64}$`), `lastNotificationId`; to-many `notifications` with cascade delete |
| `Notification` | `id` | `id`, `time`, `title`, `message`, `priority` (1-5, default 3), `tags`, `click`, `actions` (JSON string), `attachment*` (name, type, size, expires, url, localPath, progress) |
| `User` | `baseUrl` | `baseUrl`, `username`, `password` |
| `Preference` | | `key`, `value`: `defaultBaseUrl`, `attachmentAutoDownloadMaxSize`, `criticalAlertsEnabled` |

`attachmentProgress` is an Int16 state (`AttachmentProgressState.swift`): negative values are terminal states
(none, failed, deleted, canceled, skipped), 0-99 is percent, 100 is done.

There is no `sequenceId` and no `icon` on `Notification`, so message updates/deletes and per-message icons are not
supported.

## Subscriptions

`ntfy/Persistence/SubscriptionManager.swift`:

- `subscribe(baseUrl:topic:)` subscribes to the FCM topic `firebaseTopic(baseUrl, topic)`
  (`ntfy/Utils/Helpers.swift`: the plain topic for `Config.appBaseUrl`, else `sha256("<baseUrl>/<topic>")` as hex),
  saves the `Subscription`, then polls once to fetch cached messages.
- `unsubscribe` removes the FCM topic subscription and deletes the subscription with its notifications.
- `AppDelegate.messaging(_:didReceiveRegistrationToken:)` re-subscribes `~poll` and every stored subscription on
  each FCM token refresh.

Adding a subscription on another server works, but push only arrives if that server forwards poll requests via
`upstream-base-url` (explained in `SubscriptionAddView` and `DefaultServerView`, and in
[ios-push-architecture.md](ios-push-architecture.md)).

## Polling

iOS has no long-lived connection (no WebSocket, SSE or JSON stream, see `docs/TECHNICAL_LIMITATIONS.md`). Every
request in `ntfy/Utils/ApiService.swift` is a one-shot `URLSession` data task. Messages are fetched by polling:

| Trigger | Code | Request |
|---|---|---|
| After subscribing | `SubscriptionManager.subscribe` | `GET <base>/<topic>/json?poll=1&since=<lastNotificationId or all>` |
| Subscription list appears | `SubscriptionListView.pollSubscriptions` | same, for every subscription |
| Pull to refresh | `NotificationListView` `.refreshable` | same, for one subscription |
| Background `~poll` push | `AppDelegate.application(_:didReceiveRemoteNotification:fetchCompletionHandler:)` | same, for every subscription, then local notifications are posted one by one. The server currently does not send these (see [ios-push-architecture.md](ios-push-architecture.md#4-event-types)) |
| `poll_request` alert push | `ntfyNSE/NotificationService.swift` `handlePollRequest` | `GET <base>/<topic>/json?poll=1&id=<poll_id>`, decoded as a single `Message`, 30 s timeout |

The `since` poll returns NDJSON, which `fetchJsonData` splits on newlines. `SubscriptionManager.poll` saves new
messages; `Store` then advances the subscription's `lastNotificationId`.

For `poll_request`, the NSE resolves which subscription is meant with `Store.findSubscriptionMatch`: a subscription
matches if its `urlHash()` or its plain topic equals the push topic; an exact `base_url` match wins, then the same
topic on the default server, then the first match.

Other requests: `POST <base>/<topic>` with `Title`, `Priority` and `Tags` headers to send a test notification, and
`GET <base>/<topic>/auth` to check credentials when adding a user (401/403 map to unauthorized). All requests go
through `newRequest(url:user:)`, which sets the User-Agent and, if a user is stored for that server, HTTP Basic
auth. There are no `/v1/account*` calls and no bearer tokens.

## Showing notifications

`ntfy/Utils/NotificationContent.swift` (`modify(message:baseUrl:)`) turns a `Message` into notification content, used by
the NSE and by the app for locally posted notifications:

| ntfy priority | Interruption level |
|---|---|
| 1, 2 | `.passive` |
| 3 | `.active` |
| 4 | `.timeSensitive` |
| 5 | `.critical` with the critical sound if the user enabled critical alerts and iOS granted them, else `.timeSensitive` |

- Notifications are grouped by `threadIdentifier` = topic URL.
- Actions must be registered as a category, so `configureNotificationActions` redefines the `ntfyActions` category
  for each message, with at most 4 actions. Only `view` and `http` actions are parsed (`Actions.swift`);
  `ActionExecutor.swift` runs them.
- Image attachments are downloaded inside the NSE (20 s timeout, ephemeral session) and attached; other
  attachments are summarized in the body. Auto-download respects the `attachmentAutoDownloadMaxSize` preference.
- The push payload is a flat string dictionary; `Message.from(userInfo:)` and `Message.toUserInfo()`
  (`Notification.swift`) convert it, mirroring `toFirebaseMessage` in `server/server_firebase.go`.
- Taps are handled in `AppDelegate.userNotificationCenter(_:didReceive:)`: an action, else `click`, else open the
  topic. In the foreground the app shows `[.banner, .sound]`.

## Gotchas

- **Debugging the NSE** needs Xcode's "Attach to Process by PID or Name"; its logs don't show in the app's console.
  Push doesn't work in the simulator (`docs/GETTING_STARTED.md`).
- **Credentials are stored in Core Data in plaintext**, not in the Keychain (`Store.saveUser`).
- **No `message_delete` / `message_clear` handling**; deleted or cleared messages stay on the phone.
- **Stale comment**: the header of `ntfyNSE/NotificationService.swift` says poll requests are "not implemented yet",
  but `handlePollRequest` implements them.
- **Stale doc**: `docs/FEATURE_PARITY.md` (last updated 2023-05-23) lists attachments and user authentication as not
  fully implemented; both are implemented now.
