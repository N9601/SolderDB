---
title: Authentication
order: 3
---

# Authentication

SolderDB has built-in auth — users, roles, signed session tokens. No external identity provider required.

## Users

Users live in the internal `_users` collection. Each has:

```ts
{
  id: string,             // ULID-style
  data: {
    email: string,        // lowercase, validated
    password_hash: string,// bcrypt cost 12
    role: "admin" | "user"
  }
}
```

You don't edit this collection directly — it's gated behind admin-only middleware. Use the auth endpoints / SDK methods instead.

## First user becomes admin

When you `POST /api/auth/register` and the `_users` collection is empty, the new user is assigned `role: "admin"`. Subsequent registrations get `role: "user"`.

This is intentional: it means the first person to install SolderDB on a machine bootstraps the admin without needing a separate setup flow.

## Sessions

Login returns a `Session`:

```json
{
  "token": "eyJzdWI...XYZ.HMACSIG",
  "user": { "id": "...", "email": "...", "role": "admin", ... },
  "expires": "2026-06-03T12:34:56Z"
}
```

The token is **not** a JWT — it's a custom HMAC-SHA256 signed payload:

```
base64url(payload-json) "." base64url(hmac-sha256(secret, payload-json))
```

Where payload is `{ sub: userID, iat: <unix>, exp: <unix> }`.

The signing secret is generated on first run (32 random bytes from `crypto/rand`) and persisted to `<dataDir>/.secret`. **Don't share that file** — possessing it lets you forge tokens.

Tokens are valid for 7 days. There's no refresh flow in v1; you log in again when one expires.

## Sending the token

Standard: `Authorization: Bearer <token>` header.

For browser-initiated requests that can't set headers (`EventSource`, `<img src>`, `<a href download>`), the server also accepts `?token=<token>` on these specific routes:

- `GET /api/realtime?topic=...&token=...`
- `GET /api/files/:id?token=...`

The JS SDK exposes `db.files.url(id)` and the realtime subscribe helper which automatically appends the query token.

## Password change

```http
POST /api/auth/password
Authorization: Bearer <token>
Content-Type: application/json

{ "current": "supersecret", "next": "newpassword" }
```

Existing tokens stay valid (they were HMAC-signed against the server secret, not the password). To force logout on password change, rotate `<dataDir>/.secret` — every token becomes invalid immediately.

## Roles & policies

The HTTP middleware enforces three policy levels:

| Policy   | Effect |
|----------|--------|
| `public` | No token required |
| `authed` | Any valid token |
| `admin`  | Token belongs to a `role: "admin"` user |

Defaults fail closed — unknown routes require `authed`. Collection record endpoints are special: the policy comes from the collection's per-action rule (`listRule` / `createRule` / etc.).

## Threat model

SolderDB binds to `127.0.0.1` only. It's designed for **local trust** — anyone with a shell on the machine can read the data directory directly, regardless of auth. Auth exists to:

- Gate the admin GUI against shoulder-surfers / shared workstations
- Provide per-user audit trails (the activity log records the user behind each request)
- Enforce per-collection rules when you expose the API to other apps on the same machine

If you want to expose SolderDB to a network (`0.0.0.0`), you should put it behind a reverse proxy with TLS. v1 doesn't ship with that.
