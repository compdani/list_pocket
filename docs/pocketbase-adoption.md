# PocketBase adoption audit

This audit is separate from the previous architecture council. The first change
focuses on authentication boundaries; delivery and workflow ownership remain as
documented in architecture.md.

Implemented in the first change:

- PocketBase hidden-field metadata protects `twofa_key`, with a forward migration
  for existing databases and runtime repair for existing/new auth schemas.
- A named `OnRecordAuthRequest` hook enforces enabled human-user eligibility.
  Native password authentication respects `password_login`; native initial auth
  rejects users with an application second factor. Existing authenticated tokens
  may refresh, subject to enabled status. The existing application TOTP UX stays.
- Core user reads preserve explicit `password_login=false` even when a password
  hash exists; legacy defaults are not inferred on reads.
- `/mailapi` requires the `users` collection and checks enabled status on each
  request. Custom token issuance also checks enabled status.
- Versioned API rules lock users and inbox collections to server-side application
  access, blocking public signup, owner role changes and inbox permission bypass.
  Authorized attachment downloads stream through the native filesystem; direct
  protected file URLs remain inaccessible to ordinary unauthenticated clients.
- Native password-reset endpoints are blocked through request hooks so they
  cannot bypass the application reset policy and localized messaging.
- Password reset uses native signed reset tokens and returns to ordinary login,
  preserving the application TOTP requirement. Existing localized mail and URLs
  remain. Previously issued process-local reset links must be requested again.
- Exchange requires nonblank `shared_secret`, HS256, expiration, and a nonblank
  email. Disabled, API, and TOTP users cannot exchange tokens. No issuer/audience
  contract exists in this repository: deployment-specific validation remains a
  follow-up before treating arbitrary upstream issuers as interchangeable.

Follow-up opportunities from the first Sol review (not implemented here): transactional settings
patches with propagated lookup errors; gradual native file-field adoption while
preserving published URLs; native restart with ordered shutdown; incremental dbx
and Record adoption in non-hot paths. Keep custom cron callback joining, durable
queues/outbox, and explicit raw-SQL compatibility boundaries.

Council: Sol 6.1 scored the initial server 6/10 and round 2 at 7/10, identifying
Core password-policy overrides. Both overrides have since been removed, with
regressions covering Core lookup, application login, forgot/reset, and persisted
policy. Native HTTP password/refresh/exchange tests cover enabled, disabled,
password-disabled, TOTP, and API users. The localized email-to-reset integration
covers restart survival, tampering, replay, old-session invalidation, and subsequent
TOTP login. Grok 4.7 round 1 scored 7.5/10; Copilot quick round 1 scored 6.5/10. New reviews must assess the changed implementation; an
initial score is not an approval of these fixes.

Sol round 3 approved the preceding auth scope at 9/10 with no blockers.
Copilot quick round 1 scored 6.5/10 and found native users/inbox API authorization
gaps; the subsequent migration and streaming changes require fresh review.
Its selective native rate-limiting recommendation is deferred to a separate
request-abuse scope so limits and deployment/proxy behavior can be tuned without
changing tracking and webhook traffic.

Sol round 4 scored 8.5/10: security scope satisfied, but browser attachment links
needed authentication and download filenames were missing. Both UIs now fetch
attachment blobs with the PocketBase token; the handler safely encodes the
original filename. Registered-route coverage proves authenticated success and
permission denial. Frontend build and all 116 unit tests pass.

Grok follow-ups: durable/revocable application TOTP challenges with durable
one-time consumption (a signed JWT alone cannot preserve replay prevention
across restart); native RealIP with configured trusted proxies; selective auth
rate limits; reducing redundant vacuum work; named filter parameters; encrypted
application secret settings and directory-listing-free public asset serving.
These are separate adoption slices, not approvals or implemented changes.

Sol round 5: 9/10, satisfied with the final auth, API permissions and authenticated
attachment-download scope; focused regressions independently passed. Nonblocking:
frontend helper coverage for both metadata shapes and failed downloads. Browser
interaction was not exercised. Grok and Copilot rechecks remain pending.

Copilot quick round 2: 9/10, satisfied, no blockers. Attachment responses now
explicitly set private/no-store caching with a regression assertion. Known-email
native reset requests return 403 while unknown emails return 204; this residual
enumeration behavior is documented, with no reset-policy bypass. Grok recheck
remains pending.

The server subsequently migrated to PocketBase v0.41.1 using the canonical
`ganigeorgiev.com/pocketbase/pocketbase` module and companion dbx/validation
modules. Earlier council scores describe the v0.40.1 snapshots, not independent
approval of this dependency upgrade. Grok final recheck was interrupted.
