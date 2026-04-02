# Privacy policy

**Last updated:** March 31, 2026

This privacy policy describes how ntfy ("we", "us", or "our") collects, uses, and handles your information
when you use the ntfy.sh service, web app, and mobile applications (Android and iOS).

## Our commitment to privacy

We love free software, and we're doing this because it's fun. We have no bad intentions, and **we will
never monetize or sell your information**. The ntfy service and software will always stay free and open source.
If you don't trust us or your messages are sensitive, you can [self-host your own ntfy server](install.md).

## Information we collect

### Account information (optional)

If you create an account on ntfy.sh, we collect:

- **Username** - A unique identifier you choose
- **Password** - Stored as a secure bcrypt hash (we never store your plaintext password)
- **Email address** - If you subscribe to a paid plan (for billing purposes via Stripe), or if you add a verified
  email address for use with the email notification feature
- **Phone number** - Only if you enable the phone call notification feature (verified via SMS/call)

You can use ntfy without creating an account. Anonymous usage is fully supported.

### Messages and notifications

- **Message content** - Messages you publish are temporarily cached on our servers (default: 12 hours) to support 
  message polling and to overcome client network disruptions. Messages are deleted after the cache duration expires.
- **Attachments** - File attachments are temporarily stored (default: 3 hours) and then automatically deleted.
- **Topic names** - The topic names you publish to or subscribe to are processed by our servers.

### Technical information

- **IP addresses** - Used for rate limiting to prevent abuse. May be temporarily logged for debugging purposes,
  though this is typically turned off.
- **Access tokens** - If you create access tokens, we store the token value, an optional label, last access time, 
  and the IP address of the last access.
- **Web push subscriptions** - If you enable browser notifications, we store your browser's push subscription 
  endpoint to deliver notifications.

### Billing information (paid plans only)

If you subscribe to a paid plan, payment processing is handled by Stripe. We store:

- Stripe customer ID
- Subscription status and billing period

We do not store your credit card numbers or payment details directly. These are handled entirely by Stripe.

## Third-party services

To provide the ntfy.sh service, we use the following third-party services:

### Firebase Cloud Messaging (FCM)

We use Google's Firebase Cloud Messaging to deliver push notifications to Android and iOS devices. When you 
receive a notification through the mobile apps (Google Play or App Store versions):

- Message metadata and content may be transmitted through Google's FCM infrastructure
- Google's [privacy policy](https://policies.google.com/privacy) applies to their handling of this data

**To avoid FCM entirely:** Download the [F-Droid version](https://f-droid.org/en/packages/io.heckel.ntfy/) of 
the Android app and use a self-hosted server, or use the instant delivery feature with your own server.

### Twilio (phone calls)

If you use the phone call notification feature (`X-Call` header), we use Twilio to:

- Make voice calls to your verified phone number
- Send SMS or voice calls for phone number verification

Your phone number is shared with Twilio to deliver these services. Twilio's 
[privacy policy](https://www.twilio.com/legal/privacy) applies.

### Amazon SES (email delivery)

If you use the email notification feature (`X-Email` header), we use Amazon Simple Email Service (SES) to 
deliver emails. The recipient email address and message content are transmitted through Amazon's infrastructure. 
Amazon's [privacy policy](https://aws.amazon.com/privacy/) applies.

### Stripe (payments)

If you subscribe to a paid plan, payments are processed by Stripe. Your payment information is handled directly 
by Stripe and is subject to Stripe's [privacy policy](https://stripe.com/privacy).

Note: We have explicitly disabled Stripe's telemetry features in our integration.

### Web push providers

If you enable browser notifications in the ntfy web app, push messages are delivered through your browser 
vendor's push service:

- Google (Chrome)
- Mozilla (Firefox)
- Apple (Safari)
- Microsoft (Edge)

Your browser's push subscription endpoint is shared with these providers to deliver notifications.

## Mobile applications

### Android app

The Android app is available from two sources:

- **Google Play Store** - Uses Firebase Cloud Messaging for push notifications. Firebase Analytics is 
  **explicitly disabled** in our app.
- **F-Droid** - Does not include any Google services or Firebase. Uses a foreground service to maintain 
  a direct connection to the server.

The Android app stores the following data locally on your device:

- Subscribed topics and their settings
- Cached notifications
- User credentials (if you add a server with authentication)
- Application logs (for debugging, stored locally only)

### iOS app

The iOS app uses Firebase Cloud Messaging (via Apple Push Notification service) to deliver notifications. 
The app stores the following data locally on your device:

- Subscribed topics
- Cached notifications
- User credentials (if configured)

## Web application

The ntfy web app is a static website that stores all data locally in your browser:

- **IndexedDB** - Stores your subscriptions and cached notifications
- **Local Storage** - Stores your preferences and session information

No cookies are used for tracking. The web app does not have a backend beyond the ntfy API.

## Data retention

| Data type              | Retention period                                  |
|------------------------|---------------------------------------------------|
| Messages               | 12 hours (configurable by server operators)       |
| Attachments            | 3 hours (configurable by server operators)        |
| User accounts          | Until you delete your account                     |
| Access tokens          | Until you revoke them or delete your account      |
| Email addresses        | Until you remove them or delete your account      |
| Phone numbers          | Until you remove them or delete your account      |
| Web push subscriptions | 60 days of inactivity, then automatically removed |
| Server logs            | Varies; debugging logs are typically temporary    |

## Self-hosting

If you prefer complete control over your data, you can [self-host your own ntfy server](install.md). 
When self-hosting:

- You control all data storage and retention
- You can choose whether to use Firebase, Twilio, email delivery, or any other integrations
- No data is shared with ntfy.sh or any third party (unless you configure those integrations)

The server and all apps are fully open source:

- Server: [github.com/binwiederhier/ntfy](https://github.com/binwiederhier/ntfy)
- Android app: [github.com/binwiederhier/ntfy-android](https://github.com/binwiederhier/ntfy-android)
- iOS app: [github.com/binwiederhier/ntfy-ios](https://github.com/binwiederhier/ntfy-ios)

## Data security

- All connections to ntfy.sh are encrypted using TLS/HTTPS
- Passwords are hashed using bcrypt before storage
- Access tokens are generated using cryptographically secure random values
- The server does not log message content by default

## Your rights

You have the right to:

- **Access** - View your account information and data
- **Delete** - Delete your account and associated data via the web app
- **Export** - Your messages are available via the API while cached

To delete your account, use the account settings in the web app or contact us.

## Changes to this policy

We may update this privacy policy from time to time. Changes will be posted on this page with an updated 
"Last updated" date. You may also review all changes in the [Git history](https://github.com/binwiederhier/ntfy/commits/main/docs/privacy.md). 

For significant changes, we may provide additional notice on Discord/Matrix or through the
[announcements](https://ntfy.sh/announcements) ntfy topic.

## Contact

For privacy-related inquiries, please email [privacy@mail.ntfy.sh](mailto:privacy@mail.ntfy.sh).

For all other contact options, see the [contact page](contact.md).
