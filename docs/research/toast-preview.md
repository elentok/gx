# What a phone toast shows: Telegram and Slack notification previews

Research for ticket gx:better-notifications/01. Read on 2026-11-06. No device was tested. Every
claim names its source. Where the docs say nothing, this file says so.

## Summary

- No primary source gives a line or character count for a Telegram or Slack toast. Both vendors are
  silent. The only numbers come from the OS layer and from Android design advice (below).
- Telegram: a bot message to a private chat is pushed as the template `{1}: {2}` with the bot name
  first and the message body second. The body is a plain string, so MarkdownV2 markers are not
  shown and bold is not rendered.
- Slack: the toast text comes from the top-level `text` field, not from blocks, on mobile. Desktop
  tries blocks first. The webhook name and icon cannot be overridden.
- The headline should be short and stand alone, because the visible length is set by each OS and
  is not documented. Nothing in the docs shows that a batch can show more than its first message.

## 1. Telegram

### Where the bot name goes

- Telegram's push format lists `MESSAGE_TEXT` (private chat) with the template `{1}: {2}`.
  Arguments: message author, then message body. Group chats use `CHAT_MESSAGE_TEXT`:
  `{1} @ {2}: {3}` (author, chat name, body). Channels use `CHANNEL_MESSAGE_TEXT`: `{1}: {2}`.
  Source: https://core.telegram.org/api/push-updates (section on notification types).
- So for a bot DM the bot name is part of the line, in front of the body, and costs width in that
  line. Whether a client puts the name in a separate title row is a client choice. The template is
  only the default text. Telegram for Android builds its notification with
  `setContentTitle(name)` and the message as content text, so the name is a title row there.
  Source: `NotificationsController.java` in https://github.com/DrKLO/Telegram (around line 5643,
  `setContentTitle(name)`, `setContentText(text.toString())`).
- Telegram for iOS fills `content.title`, `content.subtitle` and `content.body` from the push
  `aps.alert` dictionary (`title`, `subtitle`, `body`), or uses the alert string as the body.
  Source: `Telegram/NotificationService/Sources/NotificationService.swift` in
  https://github.com/TelegramMessenger/Telegram-iOS (lines near 1235 and 1404).
- Docs are silent on the macOS desktop client toast layout. Not checked in source.

### Lines and characters

- The Bot API caps a sent message at 1-4096 characters after entity parsing
  (https://core.telegram.org/bots/api#sendmessage). That is the send limit, not a preview limit.
- Bot API and push-updates docs say nothing about how much of the body a toast shows.
- Android: the notification carries the full message string. Truncation is done by the Android
  system (see section 3). Source: `NotificationsController.java` passes the whole message to
  `MessagingStyle.Message` and `setContentText`. The only cut in that file is
  `substring(0, 100)` for the legacy status-bar ticker (`setTicker`), which also replaces `\n` with
  a space. That is the ticker, not the toast body.
- iOS: the body comes from the push payload. The docs do not say whether Telegram's server trims it.
  Unknown.
- Line breaks: no Telegram source states how `\n` in a bot message shows in a toast. The Android
  `MessagingStyle` path keeps the string as is. Unknown for iOS and macOS.

### Markdown

- MarkdownV2 is parsed into message entities (bold, italic and so on) by the server. The message
  text field holds the plain text and the formatting travels as separate entities.
  Source: https://core.telegram.org/bots/api#formatting-options and the `Message.text` /
  `Message.entities` fields.
- Android builds the toast text from `messageObject.messageOwner.message`, the plain text field,
  with no entity spans applied. Markers like `*` are therefore gone (they were consumed by the
  parser) and bold is not rendered. Source: `NotificationsController.java`,
  `getStringForMessage`.
- Docs do not state this outright for iOS or macOS. The iOS code path takes a plain alert string
  and, only when custom emoji are present, uses `message.text` for the body.
- One documented exception: a custom emoji entity needs a fallback emoji, which is shown "in
  places where a custom emoji cannot be displayed (e.g., system notifications)". Source: Bot API
  `MessageEntity`.

### Emoji at the start

- A normal Unicode emoji at the start of the body is part of the plain string and shows. Nothing in
  the docs says it is stripped.

## 2. Slack

### Which text becomes the toast

- `chat.postMessage` docs: the top-level `text` field is "used as a fallback string to display in
  notifications" when `blocks` are used. It is not required when blocks or attachments are present.
  Source: https://docs.slack.dev/reference/methods/chat.postMessage.
- Formatting docs: desktop notifications "extract text from the top-level `text` field of `blocks`
  first", and fall back to `message.text`. Mobile notifications use only `message.text`.
  Source: https://docs.slack.dev/messaging/formatting-message-text.
- Incoming webhooks: the payload is a `text` key, with optional blocks and attachments. The docs
  do not mention notification text on that page.
  Source: https://docs.slack.dev/messaging/sending-messages-using-incoming-webhooks.
- Practical result: for gx, put the headline in the top-level `text`. Do not rely on blocks for the
  mobile toast.

### Lines and characters

- Slack docs give no preview length. They advise keeping `text` to 4,000 characters, and cut
  messages over 40,000 characters (chat.postMessage). Those are message limits, not toast limits.
- Slack help only says the desktop preview can be turned on or off ("Include a preview of the
  message in each notification"). It does not say how much shows.
  Source: https://slack.com/help/articles/201355156-Configure-your-Slack-notifications.
- Line break handling: unknown. No Slack doc states it.

### Markdown

- `mrkdwn` is the default for `text` (`mrkdwn` param, default true); `*bold*` produces bold in the
  message.
- Whether a toast shows `*` markers, renders bold or strips them: Slack docs are silent. Do not
  assume. A real-device check is needed.

### App name

- A webhook cannot override the username or icon; they "always inherit from the associated Slack
  app configuration". Source: incoming webhooks page above.
- Docs say nothing on whether the app name or channel sits in the title row or in the body row of a
  toast, per client. Slack help says desktop banners identify the sender and conversation, with no
  detail.

### Emoji at the start

- Docs are silent. Standard emoji and `:shortcode:` handling in toasts is not documented.

## 3. OS limits that apply to both apps

- Android design guide: the content title is cut to one line, "even when expanded". Advice is
  titles of at most 30 characters and content text under 40 characters. This is advice, not a hard
  limit. Source: https://developer.android.com/design/ui/mobile/guides/home-screen/notifications
  (section "Style").
- Android: collapsed notifications show title, one line of content text and the app name in the
  header. `BigTextStyle` shows more text only after the user expands. Source:
  https://developer.android.com/develop/ui/views/notifications/expanded.
- Android `MessagingStyle` ignores `setContentTitle` and `setContentText` (same expanded page).
- Android privacy: on a secure lock screen a "private" notification hides content. Source: Android
  design guide above.
- iOS: Apple's page for `UNNotificationContent` did not return readable text through the fetch tool,
  and the Human Interface Guidelines page for notifications also returned only a title. Not
  verified. Apple is not known to publish a character or line count for banners.
- macOS: not checked in any primary source.

## 4. What this means for the design

- Headline length: the docs set no number. Treat about 30 characters (Android's title advice) as
  the stand-alone budget and 40 characters as the one-line content budget. Anything longer may be
  cut on a phone. This is a heuristic from Android design advice, not a measured limit.
- Put the most useful words first. Telegram prefixes the bot name in the template, so the first
  words of the body are not at the very start of the line.
- Plain text in the first line: assume no bold. Telegram drops the markers server side. Slack's
  behaviour is unknown.
- Batches: no source says a toast shows more than the first lines of a message. A grouped batch
  would depend on `\n` handling, which no doc states. Needs a device test before relying on it.

## 5. Open questions (need a device test)

1. Telegram iOS and macOS: how many lines of the body show, and are `\n` kept?
2. Slack iOS, Android, macOS: lines and characters, `\n` handling, and whether `*bold*` shows as
   markers, bold or plain.
3. Slack: where the app name sits in each client's toast.
4. Both apps: does a leading emoji show on every client? Expected yes, unverified for Slack.
