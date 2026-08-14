# Runtime settings boundary

The CLI treats settings as either `live` or `restart` so operators know whether
a save can take effect immediately.

## Live settings

These belong in the server SQLite configuration and should be exposed through
an authenticated local management API:

- Display name and description
- Discoverability
- Join policy (`invite`, `approval`, or the existing supported values)
- LAN-open mode
- New-connection gate
- Upload, avatar, and emoji limits
- Profanity mode and censor style
- System and user-created channel configuration

The new-connection gate should be a dedicated boolean such as
`accepting_new_connections`, defaulting to true. It should reject only new
join/authentication attempts; established sessions and owner/admin access must
remain usable so the setting can be reversed without a restart.

Every update should be transactional, audited, validated against a shared
schema, and broadcast to connected administrators. The API should support an
optimistic revision number so two administrators cannot silently overwrite
each other.

## Restart-bound settings

These define process, trust, or infrastructure boundaries and should remain
environment/deployment settings:

- Bind host and port
- Data directory and instance identity
- JWT secret and OIDC/certificate trust roots
- SFU address and shared secret
- Storage backend and credentials
- Trusted proxy topology
- Worker concurrency and sweep intervals

Some rate limits could become live later, but only after their in-memory
counters can consume updated policies safely.

## Proposed local API

```text
GET   /api/admin/runtime-settings
PATCH /api/admin/runtime-settings
GET   /api/admin/status
```

Authentication should use a local operator token stored with owner-only file
permissions, not a regular member bearer token. Remote administration can then
be an explicit opt-in with TLS and normal owner authentication.
