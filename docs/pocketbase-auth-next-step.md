# PocketBase migration analysis: recommended next step

## What the codebase currently looks like

The project is in a **hybrid state**:

- Routing and API protection are now handled by PocketBase (`/mailapi` is bound with `apis.RequireAuth()`).
- Frontend requests are made through the PocketBase JS SDK and login already calls `pb.collection('users').authWithPassword(...)`.
- At the same time, the backend still contains legacy listmonk auth logic: Echo handlers for login/setup/forgot/reset/TOTP, custom OIDC flow, and cookie/session management via `simplesessions`.
- User lifecycle and credential operations are still implemented in the legacy core SQL/query path (`CreateUser`, `UpdateUser`, `LoginUser`, etc.), not native PocketBase auth-record management.

In short: **PocketBase tokens gate API access, but identity and account workflows are still largely listmonk-era code**.

---

## Recommended immediate next step

### Build a dedicated `PocketBaseAuthService` and switch one end-to-end auth flow first

The highest-leverage next move is to make PocketBase the **single source of truth for authentication** by introducing a backend auth service that wraps PocketBase auth APIs and migrating one complete vertical flow first.

#### Start with this vertical slice

1. **Password login + profile bootstrap path**
   - Replace legacy `doLogin` / session creation dependency path with PocketBase auth-record validation and token issuance only.
   - Keep current permission checks, but derive current user identity exclusively from the PocketBase auth record in request context.

2. **Logout semantics cleanup**
   - Remove dependence on legacy session destruction for API auth and standardize on token invalidation/clear behavior (frontend + backend).

3. **User create/update sync to PocketBase auth records**
   - Ensure admin user management endpoints write credentials/status to PocketBase auth records first, then mirror any listmonk-specific role metadata.

This gives you a complete “login-to-API-to-user-admin” loop backed by PocketBase identity without trying to migrate every auth feature (OIDC, 2FA, reset flows) in one risky batch.

---

## Why this is the best next step

- It attacks the largest inconsistency: dual auth stacks running simultaneously.
- It unblocks all later auth work (OIDC, 2FA, reset/password policies) because those can then be implemented against a single identity model.
- It reduces accidental auth regressions where frontend auth state and backend session state diverge.
- It makes future migration tasks measurable and testable by flow.

---

## Suggested implementation plan (small, safe phases)

### Phase 0: Introduce abstraction (no behavior change)
- Add `internal/auth/pocketbase_service.go` with operations like:
  - `AuthenticatePassword(username, password)`
  - `FindAuthUser(id|email|username)`
  - `CreateAuthUser(...)`
  - `UpdateAuthUser(...)`
  - `DisableAuthUser(...)`
- Wire current handlers/core code through interface calls while still retaining existing behavior.

### Phase 1: Migrate password login flow
- Replace legacy password verification + session setup path with PocketBase-native authentication.
- Continue returning current response structure so frontend compatibility is preserved.
- Add integration tests for:
  - valid login
  - invalid login
  - disabled user
  - role/permission hydration from auth context

### Phase 2: Migrate user CRUD credentials
- Update admin user create/update endpoints to manage password/status in PocketBase auth records.
- Keep list roles/user roles in existing app domain storage until role migration is done.
- Add consistency checks in code path to prevent “app user exists but auth record missing” drift.

### Phase 3: Remove legacy session stack for admin auth
- Delete `simplesessions`-based session dependencies for admin/API paths.
- Remove server-rendered admin login/forgot/reset pages when frontend auth pages are ready.
- Keep public subscriber endpoints unchanged (they are not admin auth).

---

## Acceptance criteria for the next milestone

You can consider the next auth milestone complete when:

- Admin password login authenticates only through PocketBase auth records.
- `/mailapi` authorization is based on PocketBase auth record context end-to-end.
- User create/update keeps auth credentials/status in PocketBase as primary source.
- Legacy session auth is no longer required for admin/API login/logout path.
- Existing role/permission checks continue to behave unchanged.

---

## Risk controls

- Keep migration behind a feature flag (e.g. `auth.mode = "hybrid"|"pocketbase"`) for one release.
- Add startup audit that reports identity drift (legacy user row without PB auth record and vice versa).
- Instrument login failures by reason to detect rollout issues quickly.

