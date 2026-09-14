# Self-link URLs as resource IDs

## Scope

Every `asc` JSON read prints Apple's envelope unmodified, so each resource
carries a `links.self` URL such as
`https://api.appstoreconnect.apple.com/v1/builds/2f1a3c4d-...`. Webhook
deliveries (`asc webhooks`) and other tools (for example EAS CLI's
`eas testflight:feedback`) hand the same URLs around. Until now every ID flag
accepted bare IDs only, so callers had to strip the URL by hand before piping a
`links.self` value back into `asc`.

This change lets the highest-traffic ID flags accept either a bare ID or the
resource's API self-link. It adds no command, no flag, and no output change.

## Contract

`shared.ResourceIDFromValue(value, resourceType string) (string, error)`:

- A value that is not an `http`/`https` URL is a bare ID and is returned
  trimmed and otherwise unchanged. Bundle identifiers, product IDs, app names,
  and every other selector shape keep working.
- A URL is accepted only when its host is `api.appstoreconnect.apple.com`
  and its path is exactly `/v<n>/<type>/<id>`. The `<id>` segment is returned.
  Query strings and fragments are ignored.
- When `resourceType` is non-empty and `<type>` differs, the value is rejected
  with a message naming both types, for example
  `expected a builds self-link, got appStoreVersions`.
- A URL on another host, with a different path shape (including
  `/relationships/...` and related-resource paths), or with an empty `<id>` is
  rejected with a message describing the accepted shape.

`shared.BindResourceIDFlag(fs, name, resourceType, usage) *string` registers a
string flag backed by the normalizer. Rejections surface as flag parse
failures, so they print `Error: invalid value "<url>" for flag -<name>: <reason>`
on stderr and exit with code 2 before authentication or any network call.

`shared.ResolveAppID` also normalizes `apps` self-links for the explicit
`--app` value, `ASC_APP_ID`, and the configured app ID, so every command that
resolves its app through that helper accepts an app self-link. Because that
helper cannot return an error, a wrong-type URL there is left untouched and
fails in the app lookup instead of as a usage error; the wired commands below
reject it at parse time.

## Wired flags

| Flag | Commands | Resource type |
| --- | --- | --- |
| `--build-id` | `builds info/update/expire/wait/...` (shared build selector), `builds add-groups`, `versions attach-build`, `validate testflight`, `testflight beta-notifications` | `builds` |
| `--app` | shared build selector, `builds list`, `versions list/view`, `testflight crashes list`, `testflight feedback list`, `subscriptions groups list`, `subscriptions list` | `apps` |
| `--id` | `apps view/update` | `apps` |
| `--version-id` | `versions view/update/release/attach-build/...` | `appStoreVersions` |
| `--version` | `localizations list` | `appStoreVersions` |
| `--submission-id` | `testflight feedback view/delete` | `betaFeedbackScreenshotSubmissions` |
| `--submission-id` | `testflight crashes view/delete/log` | `betaFeedbackCrashSubmissions` |
| `--id`, `--group`, `--group-id` | `testflight groups view/update/delete/add-testers/remove-testers` and related/relationship reads | `betaGroups` |
| `--localization-id` | `localizations search-keywords ...` | `appStoreVersionLocalizations` |
| `--info-id` | `apps info ...` | `appInfos` |
| `--subscription-id`, `--id` | `subscriptions view/update/delete`, offers, pricing | `subscriptions` |
| `--id`, `--bundle` | `bundle-ids view/update/delete`, capabilities, relationships | `bundleIds` |

Remaining `fs.String` ID flags keep bare-ID behavior; they can adopt
`BindResourceIDFlag` one package at a time.

## Alternatives considered

- Normalizing inside every resolver (`ResolveBuild`, `ResolveSubscriptionID`,
  ...): scattered, and the flag definition is the single place each command
  already declares which resource it names.
- Accepting any path and taking the third segment: silently turns
  `/v1/builds/<id>/relationships/betaGroups` into a build ID. Rejecting the
  longer path keeps the operator's intent visible.
- Accepting URLs from any host: a Developer Portal or App Store Connect web
  URL is not an API self-link; passing it through as an ID would produce a
  confusing not-found error from Apple.

## Verification

- Unit tests for the normalizer: bare ID, valid link, wrong type, longer path,
  other host, empty ID, query string, non-URL selectors.
- `cmdtest` coverage through the real command tree with a stub transport
  asserting the request path uses the extracted ID for `builds info`,
  `versions view`, and `testflight crashes view`, plus a wrong-type rejection
  exiting 2 before any request.
