# Known issues
This is an incomplete list of known issues with the ntfy server, Android app, and iOS app. You can find a complete
list [on GitHub](https://github.com/binwiederhier/ntfy/labels/%F0%9F%AA%B2%20bug), but I thought it may be helpful
to have the prominent ones here to link to.

## iOS app not refreshing (see [#267](https://github.com/binwiederhier/ntfy/issues/267))
For some (many?) users, the iOS app is not refreshing the view when new notifications come in. Until you manually
swipe down, you do not see the newly arrived messages, even though the popup appeared before.

This is caused by some weirdness between the Notification Service Extension (NSE), SwiftUI and Core Data. I am entirely
clueless on how to fix it, sadly, as it is ephemeral and now clear to me what is causing it.

Please send experienced iOS developers my way to help me figure this out.

## iOS app not receiving notifications (anymore)
If notifications do not show up at all anymore, there are a few causes for it (that I know of):

**Firebase+APNS are being weird and buggy**:    
If this is the case, usually it helps to **remove the topic/subscription and re-add it**. That will force Firebase to 
re-subscribe to the Firebase topic.

**Self-hosted only: No `upstream-base-url` set, or `base-url` mismatch**:   
To make self-hosted servers work with the iOS
app, I had to do some horrible things (see [iOS instant notifications](config.md#ios-instant-notifications) for details).
Be sure that in your selfhosted server:

* Set `upstream-base-url: "https://ntfy.sh"` (**not your own hostname!**)
* Ensure that the URL you set in `base-url` **matches exactly** what you set the Default Server in iOS to 
