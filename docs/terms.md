# Terms of Service

**Last updated:** November 24, 2026

Please read these Terms of Service ("Terms") carefully before using the ntfy.sh website and service (the "Service") 
operated by ntfy LLC ("us", "we", or "our").

Your access to and use of the Service is conditioned on your acceptance of and compliance with these Terms. These 
Terms apply to all visitors, users, and others who access or use the Service.

**By accessing or using the Service, you agree to be bound by these Terms. If you disagree with any part of the 
Terms, you may not access the Service.**

## Service description

ntfy (pronounced "notify") is a simple HTTP-based pub-sub notification service. It allows you to send push 
notifications to your phone or desktop via scripts from any computer, using a REST API. The Service includes:

- The ntfy.sh hosted server
- The ntfy web application
- The ntfy mobile applications (Android and iOS)
- The ntfy command-line interface (CLI)

The server software and mobile applications are open source and can be [self-hosted](install.md). These Terms 
apply specifically to the ntfy.sh hosted service.

## Subscriptions and billing

### Free tier

You may use the Service without creating an account or subscribing to a paid plan. Free usage is subject to 
rate limits and other restrictions as described in our documentation.

### Paid plans

Some features of the Service are available only through paid subscription plans ("Subscriptions"). You will 
be billed in advance on a recurring basis ("Billing Cycle"). Billing cycles are available on a monthly or 
annual basis.

At the end of each Billing Cycle, your Subscription will automatically renew under the same conditions unless 
you cancel it or we cancel it. You may cancel your Subscription renewal through your account settings in the 
web application.

A valid payment method is required to process payment for your Subscription. You shall provide us with accurate 
and complete billing information. By submitting such payment information, you authorize us to charge all 
Subscription fees incurred through your account to your payment method.

Payment processing is handled by Stripe. Your payment information is subject to Stripe's 
[privacy policy](https://stripe.com/privacy) and [terms of service](https://stripe.com/legal).

Should automatic billing fail to occur for any reason, we will retry the payment according to Stripe's retry 
schedule. If payment continues to fail after multiple attempts, your Subscription will be canceled and your 
account will revert to the free tier.

### Fee changes

We may, in our sole discretion and at any time, modify the Subscription fees for paid plans. Any fee change 
will become effective at the end of the then-current Billing Cycle.

We will provide you with reasonable prior notice of any change in Subscription fees to give you an opportunity 
to cancel your Subscription before such change becomes effective.

Your continued use of the Service after a fee change comes into effect constitutes your agreement to pay the 
modified Subscription fee.

## Refunds

Refund requests for Subscriptions may be considered on a case-by-case basis and granted at the sole discretion 
of ntfy LLC. To request a refund, please contact us at [billing@mail.ntfy.sh](mailto:billing@mail.ntfy.sh).

## User accounts

When you create an account with us, you must provide information that is accurate, complete, and current at 
all times. Failure to do so constitutes a breach of the Terms, which may result in immediate termination of 
your account.

You are responsible for:

- Safeguarding the password that you use to access the Service
- Any activities or actions under your account, whether your password is with our Service or a third-party service
- Keeping your account credentials confidential

You agree not to disclose your password to any third party. You must notify us immediately upon becoming aware 
of any breach of security or unauthorized use of your account.

You represent that you are at least 18 years old, or that you are at least the minimum age required to form
a binding contract in your jurisdiction, and have the legal authority to enter into these Terms.

## Acceptable use

You agree not to use the Service to:

- Send spam, unsolicited messages, or messages to recipients who have not consented to receive them
- Distribute malware, viruses, or any other malicious software
- Transmit illegal content or content that violates the rights of others
- Harass, abuse, or harm another person or group
- Impersonate any person or entity, or falsely state or misrepresent your affiliation with a person or entity
- Interfere with or disrupt the Service or servers or networks connected to the Service
- Attempt to gain unauthorized access to the Service, other accounts, or computer systems
- Use the Service for any illegal purpose or in violation of any applicable laws or regulations
- Circumvent rate limits or other technical restrictions
- Use the Service in a manner that could reasonably be expected to impose an unreasonable or disproportionately
  large load on our infrastructure

We reserve the right to investigate and take appropriate action against anyone who, in our sole discretion, 
violates this provision, including removing content, terminating accounts, and reporting to law enforcement.

### No critical or high-risk use

The Service is not designed, tested, or intended for use where a missed, delayed, or incorrect notification
could lead to death, personal injury, property damage, environmental damage, or significant financial loss.
You must not use the Service as the sole or primary means of delivering:

- Life-safety, medical, or health alerts (for example, patient monitoring or medication reminders)
- Emergency, fire, intrusion, or security alarms
- Alerts for controlling or monitoring industrial systems, vehicles, or critical infrastructure
- Any other alert where you or someone else depends on timely delivery to avoid serious harm

If you use the Service in any of these ways anyway, you do so entirely at your own risk, and you must have
an independent, reliable backup means of notification in place.

### Topic names

Topic names on ntfy.sh are public. If you use the Service without access controls, your topic name functions 
as a password. You are responsible for choosing topic names that cannot be easily guessed. We are not responsible 
for any unauthorized access to messages published to easily guessable topic names.

For reserved topics and access control features, consider subscribing to a paid plan.

## Intellectual property

### Open source software

The ntfy server, web application, and mobile applications are open source software, dual-licensed under the 
[Apache License 2.0](https://github.com/binwiederhier/ntfy/blob/main/LICENSE) and 
[GPLv2](https://github.com/binwiederhier/ntfy/blob/main/LICENSE.GPLv2). You are free to use, modify, and 
distribute the software in accordance with these licenses.

### Trademarks

The ntfy name, logo, and branding are trademarks of ntfy LLC. Our trademarks may not be used in connection 
with any product or service without our prior written consent.

### Your content

You retain ownership of any content you transmit through the Service. By using the Service, you grant us a 
limited license to process and transmit your content solely for the purpose of providing the Service.

## Service availability

The Service is provided on a "best effort" basis, for free and paid plans alike. We do not guarantee any
specific uptime, availability, delivery time, or delivery at all, and we do not offer a service level agreement.

We strive to maintain high availability, but the Service may be interrupted for maintenance, updates, or 
due to circumstances beyond our control. Interruptions may last minutes, hours, or in rare cases several
days. During or after an interruption, messages may be delayed, delivered out of order, delivered more than
once, or lost entirely. We will make reasonable efforts to notify users of planned maintenance when possible.

Downtime, delays, or lost messages do not entitle you to a refund, service credit, or any other compensation.

For applications requiring guaranteed uptime or specific service level agreements, we recommend 
[self-hosting your own ntfy server](install.md).

A [status page](https://ntfy.statuspage.io/) is available to check the current operational status of the Service.

## Third-party services

The Service relies on third-party services to provide certain functionality:

- **Firebase Cloud Messaging (FCM)** - For push notifications to Android and iOS devices
- **Twilio** - For phone call notifications
- **Amazon SES** - For email notifications
- **Stripe** - For payment processing

Your use of these features is subject to the respective third-party terms and privacy policies. For more 
details, see our [privacy policy](privacy.md).

## Links to other websites

Our Service may contain links to third-party websites or services that are not owned or controlled by us.

We have no control over, and assume no responsibility for, the content, privacy policies, or practices of 
any third-party websites or services. You acknowledge and agree that we shall not be responsible or liable, 
directly or indirectly, for any damage or loss caused by or in connection with the use of any such content, 
goods, or services available through any such websites or services.

## Termination

We may terminate or suspend your account immediately, without prior notice or liability, for any reason 
whatsoever, including without limitation if you breach these Terms.

Upon termination, your right to use the Service will immediately cease. If you wish to terminate your account, 
you may do so through your account settings or by simply discontinuing use of the Service.

Termination of your account will result in the deletion of your account data in accordance with our 
[privacy policy](privacy.md).

We may retain certain data as required to comply with legal obligations, resolve disputes, and enforce our
agreements, as described in our privacy policy.

## Limitation of liability

In no event shall ntfy LLC, nor its owner, employees, partners, agents, suppliers, or affiliates, be liable 
for any indirect, incidental, special, consequential, or punitive damages, including without limitation:

- Loss of profits, data, use, goodwill, or other intangible losses
- Damages resulting from your access to, use of, or inability to access or use the Service
- Damages resulting from any conduct or content of any third party on the Service
- Damages resulting from any content obtained from the Service
- Damages resulting from unauthorized access, use, or alteration of your transmissions or content

To the maximum extent permitted by law, the total, aggregate liability of ntfy LLC and its owner, employees, 
partners, agents, suppliers, and affiliates for all claims arising out of or relating to these Terms or the 
Service is limited to the greater of (a) the fees you actually paid us for the Service in the 12 months before 
the event giving rise to the claim, or (b) US $5. If you use the Service without a paid plan (including 
anonymous use without an account), you have paid us nothing, and our total liability to you is limited to 
US $5.

This limitation applies whether based on warranty, contract, tort (including negligence), or any other legal 
theory, whether or not we have been informed of the possibility of such damage, and even if a remedy set 
forth herein is found to have failed of its essential purpose.

These limitations are a fundamental part of the basis of the bargain between you and us. The Service is 
offered for free or at a low price only because of them.

Some jurisdictions do not allow the exclusion or limitation of certain damages. In those jurisdictions, our 
liability is limited to the fullest extent permitted by law. Nothing in these Terms excludes or limits 
liability that cannot be excluded or limited under applicable law, such as liability for fraud or, where 
applicable consumer law requires it, for death or personal injury caused by negligence.

## Indemnification

You agree to defend, indemnify, and hold harmless ntfy LLC and its owner, employees, partners, agents, suppliers,
and affiliates from and against any claims, damages, obligations, losses, liabilities, costs, or debt, and
expenses (including but not limited to attorney's fees) arising from:

- Your use of and access to the Service
- Your violation of any term of these Terms
- Your violation of any applicable law or regulation
- Your content, including any claim that your content infringes or misappropriates the rights of any third party

## Disclaimer

Your use of the Service is at your sole risk. The Service is provided on an "AS IS" and "AS AVAILABLE" basis, 
without warranties of any kind, whether express or implied, including but not limited to implied warranties of 
merchantability, fitness for a particular purpose, non-infringement, or course of performance.

ntfy LLC does not warrant that:

- The Service will function uninterrupted, secure, or available at any particular time or location
- Any errors or defects will be corrected
- The Service is free of viruses or other harmful components
- The results of using the Service will meet your requirements
- Messages will be delivered successfully or in a timely manner

If your use case requires guaranteed message delivery, high availability, or handling of sensitive data, we 
strongly recommend [self-hosting your own ntfy server](install.md) where you have full control over the 
infrastructure and data.

## Governing law

These Terms shall be governed and construed in accordance with the laws of the State of Connecticut, United States,
without regard to its conflict of law provisions.

Except for disputes that are resolved by arbitration as described below, any legal action or proceeding
arising under these Terms shall be brought exclusively in the federal or state courts located in Connecticut,
and the parties hereby consent to personal jurisdiction and venue therein.

Our failure to enforce any right or provision of these Terms will not be considered a waiver of those rights. 
If any provision of these Terms is held to be invalid or unenforceable by a court, the remaining provisions 
of these Terms will remain in effect.

These Terms constitute the entire agreement between us regarding our Service and supersede any prior agreements 
we might have had regarding the Service.

## Dispute resolution and class action waiver

**Please read this section carefully. It affects your rights, including your right to sue in court and to
take part in a class action.**

**Informal resolution first.** Before starting any formal proceeding, you agree to contact us at
[support@mail.ntfy.sh](mailto:support@mail.ntfy.sh) with a description of your claim and to try in good faith
to resolve it informally for at least 60 days.

**Binding arbitration.** If we cannot resolve a dispute informally, any dispute, claim, or controversy arising
out of or relating to these Terms or the Service shall be resolved by final and binding individual
arbitration, administered by the American Arbitration Association (AAA) under its applicable rules (the
Consumer Arbitration Rules for consumer disputes), rather than in court. The Federal Arbitration Act governs
this section. The arbitration may be conducted by video conference or, if in person, in Connecticut. Either
party may instead bring an individual claim in small claims court if it qualifies.

**Class action waiver.** You and ntfy LLC agree that each may bring claims against the other only in an
individual capacity, and not as a plaintiff or class member in any purported class, collective,
consolidated, or representative proceeding. The arbitrator may not consolidate claims of more than one
person and may award relief only to the individual party seeking it.

**Mass filings.** If 25 or more similar arbitration demands are filed against us by or with the assistance of
the same law firm or organization, they shall be administered in batches of up to 25 demands, with one
arbitrator per batch, to the extent permitted by the AAA rules.

**30-day opt-out.** You may opt out of this arbitration agreement by emailing
[support@mail.ntfy.sh](mailto:support@mail.ntfy.sh) within 30 days of first accepting these Terms (or of these
Terms first including this section), stating your name, your account username or email address (if any), and
that you opt out of arbitration. Opting out does not affect any other part of these Terms.

**Exceptions.** This section does not apply to claims for infringement or misuse of intellectual property,
or to requests for urgent injunctive relief, which may be brought in the courts named above. It also does not
apply to you if you are a consumer residing in the European Union, the United Kingdom, or another
jurisdiction whose laws do not permit pre-dispute arbitration agreements or class action waivers with
consumers. In that case, you may bring claims in the courts available to you under those laws.

If the class action waiver is found unenforceable for a particular claim, that claim (and only that claim)
shall be severed and brought in the courts named above, and the rest of this section remains in effect.

## Changes to these Terms

We reserve the right, at our sole discretion, to modify or replace these Terms at any time. If a revision is 
material, we will try to provide at least 30 days' notice prior to any new terms taking effect.

What constitutes a material change will be determined at our sole discretion. Changes will be posted on this 
page with an updated "Last updated" date. You may also review all changes in the 
[Git history](https://github.com/binwiederhier/ntfy/commits/main/docs/terms.md).

By continuing to access or use our Service after those revisions become effective, you agree to be bound by 
the revised Terms. If you do not agree to the new Terms, please stop using the Service.

## Contact

If you have any questions about these Terms, please see our [contact page](contact.md) or email us at 
[support@mail.ntfy.sh](mailto:support@mail.ntfy.sh).
