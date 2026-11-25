# iOS app internals

How the iOS app is structured: its two targets, the shared Core Data store, subscriptions and polling. Code is in
[ntfy-ios](https://github.com/binwiederhier/ntfy-ios); `ntfy/`, `ntfyNSE/` and `ntfyTests/` paths are there,
`server/` paths are in this repo.

How a message gets from a server to the phone (FCM, APNs, `poll_request`, `upstream-base-url`) is in
[ios-push-architecture.md](ios-push-architecture.md) and not repeated here. See
[web-app.md](web-app.md#cross-client-feature-matrix) for how iOS compares to the other clients.

## Overview

| Item | Value |
|---|---|
| Language / UI | Swift, SwiftUI, Combine |
| Deployment target | iOS 15.0 (project default still 14.0; some `#available(iOS 15.0, *)` checks remain) |
| Dependency | Firebase iOS SDK via SwiftPM (`FirebaseMessaging`), the only one |
| Server | build setting `APP_BASE_URL = "https://ntfy.sh"` -> `AppBaseURL` in both `Info.plist` -> `Config.appBaseUrl` (`ntfy/Utils/Config.swift`) |
| Firebase config | `GoogleService-Info.plist` not in the repo (`docs/GETTING_STARTED.md`); CI writes a dummy (`scripts/dummy-firebase-plist.sh`) |
| Tests | `ntfyTests/` (Swift Testing) + SwiftLint in `.github/workflows/ci.yml`; see `docs/dev/testing.md` |

## Targets

```mermaid
flowchart LR
    subgraph Phone
        subgraph AppP["ntfy app process"]
            Views["Views/<br/>SubscriptionListView,<br/>NotificationListView, Settings"]
            AD["AppDelegate<br/>FCM token, ~poll, taps"]
            SubM[SubscriptionManager]
            APIa[ApiService]
        end
        subgraph NSEP["ntfyNSE process, per alert push"]
            NSE["NotificationService<br/>didReceive"]
            APIn[ApiService]
        end
        subgraph Group["App Group group.io.heckel.ntfy"]
            CD[("Core Data ntfy.sqlite")]
            UD[("shared UserDefaults")]
            Att[("attachments/")]
        end
        iOS["iOS notification center"]
    end
    FCM["FCM / APNs"]
    Server["ntfy server"]

    Views --> SubM
    SubM -->|subscribe topic| FCM
    SubM --> APIa -->|"GET /topic/json?poll=1"| Server
    FCM -->|alert push| iOS --> NSE
    FCM -->|"~poll background push"| AD
    NSE --> APIn -->|"GET /topic/json?poll=1&id="| Server
    NSE -->|modified content| iOS
    AD & SubM & Views --> CD
    NSE --> CD & UD & Att
```

| Target | Type | Bundle ID | Role |
|---|---|---|---|
| `ntfy` | app | `io.heckel.ntfy` | UI, subscriptions, FCM registration, polling, `~poll` pushes, taps |
| `ntfyNSE` | notification service extension | `io.heckel.ntfy.ntfyNSE` | own process per alert push: save, fetch `poll_request`, rewrite content |
| `ntfyTests` | unit tests | `io.heckel.ntfyTests` | `HelpersTests`, `MessageTests`, `NotificationContentTests`, `StoreTests` |

Both carry the App Group and the critical alerts entitlement, and set `NSAllowsArbitraryLoads` (self-hosted
servers may be plain HTTP). Only background mode: `remote-notification`; no `BGTaskScheduler`.

| Path | Contents |
|---|---|
| `ntfy/App/` | `AppMain.swift`, `AppDelegate.swift` |
| `ntfy/Persistence/` | `Store.swift`, `SubscriptionManager.swift`, `Notification.swift` (wire types `Message`, `MessageAttachment`, `Action`), `SubscriptionsObservable` / `NotificationsObservable` (`NSFetchedResultsController`), model in `ntfy.xcdatamodeld/` |
| `ntfy/Utils/` | `ApiService.swift`, `NotificationContent.swift`, `Helpers.swift` (URLs, topic hashing), `Actions.swift` / `ActionExecutor.swift`, `AttachmentFileStore.swift` |
| `ntfy/Views/` | `ContentView` -> `MainView` tabs |
| `ntfyNSE/` | `NotificationService.swift` |

## Core Data store shared by app and NSE

`Store.shared` (`ntfy/Persistence/Store.swift`) opens `ntfy.sqlite` in the App Group container (`/dev/null` for
`Store(inMemory: true)`), so two processes share one file.

| Mechanism | Why |
|---|---|
| `NSPersistentStoreRemoteChangeNotificationPostOptionKey` + `.NSPersistentStoreRemoteChange` -> `hardRefresh()` | app sees NSE writes; the code calls it a hack |
| `automaticallyMergesChangesFromParent`, `mergeByPropertyStoreTrumpMergePolicyType` | store wins on conflict |
| `transactionAuthor` `ntfy.appex` / `ntfy` | tells the writers apart |
| `rollbackAndRefresh()` | a failed save usually means the NSE stored the same notification first |
| Lightweight migration | single model version so far |

The App Group also holds a shared `UserDefaults` (`criticalAlertsAuthorized`, read by the NSE) and downloaded
attachments under `attachments/`.

### Entities (`Model.xcdatamodel/contents`)

```mermaid
erDiagram
    Subscription ||--o{ Notification : "notifications, cascade"
    Subscription }o--o| User : "baseUrl"
    Subscription {
        string baseUrl UK "unique with topic"
        string topic UK "regex A-Za-z0-9_- 1-64"
        string lastNotificationId "since="
    }
    Notification {
        string id UK
        int64 time
        string title
        string message
        int16 priority "1-5, default 3"
        string tags
        string click
        string actions "JSON string"
        string attachmentName
        string attachmentType
        int64 attachmentSize
        int64 attachmentExpires
        string attachmentUrl
        string attachmentLocalPath
        int16 attachmentProgress "default -1"
    }
    User {
        string baseUrl UK
        string username
        string password
    }
    Preference {
        string key
        string value
    }
```

| Entity | Example row |
|---|---|
| `Subscription` | `baseUrl=https://ntfy.sh, topic=backups-alerts, lastNotificationId=aB3dE5fG7hJ9` |
| `Notification` | `id=aB3dE5fG7hJ9, time=1764000000, title=Backup, message=Nightly backup done, priority=4, tags=warning, attachmentProgress=-1` |
| `User` | `baseUrl=https://ntfy.example.com, username=alice, password=s3cret` |
| `Preference` | `key=defaultBaseUrl, value=https://ntfy.example.com` (also `attachmentAutoDownloadMaxSize`, `criticalAlertsEnabled`) |

- `attachmentProgress` (`AttachmentProgressState.swift`): negative = terminal (none, failed, deleted, canceled,
  skipped), 0-99 = percent, 100 = done.
- No `sequenceId` and no `icon`: message updates/deletes and per-message icons are not supported.

## Subscriptions

```mermaid
sequenceDiagram
    autonumber
    participant V as SubscriptionAddView
    participant SM as SubscriptionManager
    participant F as FirebaseMessaging
    participant St as Store
    participant S as ntfy server
    V->>SM: subscribe baseUrl, topic
    SM->>SM: firebaseTopic: plain topic on Config.appBaseUrl, else sha256 of baseUrl/topic
    SM->>F: subscribe toTopic
    SM->>St: saveSubscription
    SM->>S: poll: GET /topic/json?poll=1&since=lastNotificationId or all, Basic auth
    S-->>SM: NDJSON
    SM->>St: save notificationsFromMessages, advance lastNotificationId
```

- `unsubscribe` removes the FCM topic and deletes the subscription with its notifications.
- `AppDelegate.messaging(_:didReceiveRegistrationToken:)` re-subscribes `~poll` and every subscription on each FCM
  token refresh.
- Other servers work only if they forward poll requests via `upstream-base-url` (explained in
  `SubscriptionAddView`, `DefaultServerView`, and [ios-push-architecture.md](ios-push-architecture.md)).

## Polling

No long-lived connection (no WebSocket, SSE or JSON stream; `docs/TECHNICAL_LIMITATIONS.md`). Every request in
`ntfy/Utils/ApiService.swift` is a one-shot `URLSession` task.

| Trigger | Code | Request |
|---|---|---|
| After subscribing | `SubscriptionManager.subscribe` | `GET /<topic>/json?poll=1&since=<id or all>` |
| List appears | `SubscriptionListView.pollSubscriptions` | same, every subscription |
| Pull to refresh | `NotificationListView` `.refreshable` | same, one subscription |
| `~poll` background push | `AppDelegate.application(_:didReceiveRemoteNotification:...)` | same, every subscription, then local notifications one by one. The server does not send these today ([ios-push-architecture.md](ios-push-architecture.md#4-event-types)) |
| `poll_request` alert push | `NotificationService.handlePollRequest` | `GET /<topic>/json?poll=1&id=<poll_id>`, one `Message`, 30 s timeout |

### Polling after a push (NSE)

```mermaid
sequenceDiagram
    autonumber
    participant A as APNs
    participant NSE as ntfyNSE NotificationService
    participant St as Store, shared Core Data
    participant S as ntfy server of the subscription
    participant NC as Notification center
    participant App as ntfy app
    A->>NSE: didReceive alert push, userInfo flat strings
    NSE->>NSE: Message.from userInfo
    alt event poll_request
        NSE->>St: findSubscriptionMatch topic, preferred base_url
        Note over NSE,St: urlHash or plain topic matches, exact base_url wins, then default server, then first
        NSE->>S: GET /topic/json?poll=1&id=poll_id, Basic auth
        S-->>NSE: Message
    end
    NSE->>St: save notificationFromMessage
    NSE->>NSE: content.modify, attachImageIfNeeded, 20 s
    NSE->>NC: contentHandler modified content
    St-->>App: NSPersistentStoreRemoteChange -> hardRefresh
    opt timeout or error
        NSE->>NC: original or best attempt content
    end
```

Other requests: `POST /<topic>` with `Title`, `Priority`, `Tags` headers (test notification) and
`GET /<topic>/auth` (credential check, 401/403 = unauthorized). `newRequest(url:user:)` sets User-Agent and Basic
auth; no `/v1/account*`, no bearer tokens.

## Showing notifications

`NotificationContent.swift` (`modify(message:baseUrl:)`), used by the NSE and for locally posted notifications:

| ntfy priority | Interruption level |
|---|---|
| 1, 2 | `.passive` |
| 3 | `.active` |
| 4 | `.timeSensitive` |
| 5 | `.critical` + critical sound if enabled and granted, else `.timeSensitive` |

- `threadIdentifier` = topic URL. `configureNotificationActions` redefines category `ntfyActions` per message, max
  4; only `view` and `http` (`Actions.swift`, run by `ActionExecutor.swift`).
- Images are downloaded in the NSE (ephemeral session) respecting `attachmentAutoDownloadMaxSize`; other
  attachments are summarized in the body.
- `Message.from(userInfo:)` / `toUserInfo()` (`Notification.swift`) mirror `toFirebaseMessage` in
  `server/server_firebase.go`.
- Taps: `AppDelegate.userNotificationCenter(_:didReceive:)` runs an action, else `click`, else opens the topic.
  Foreground presentation is `[.banner, .sound]`.

## Gotchas

- **Debugging the NSE** needs Xcode "Attach to Process by PID or Name"; push doesn't work in the simulator.
- **Credentials are plaintext in Core Data**, not the Keychain (`Store.saveUser`).
- **No `message_delete` / `message_clear` handling.**
- **Stale comment**: the `ntfyNSE/NotificationService.swift` header says poll requests are "not implemented yet";
  `handlePollRequest` implements them.
- **Stale doc**: `docs/FEATURE_PARITY.md` (2023-05-23) lists attachments and auth as incomplete; both work now.
