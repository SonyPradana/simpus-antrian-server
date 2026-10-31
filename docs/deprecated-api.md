# Deprecated API Removal Checklist

The following legacy HTTP endpoints are deprecated. They remain temporarily for backward compatibility and should not receive new features. Responses include `Deprecation: true`; endpoints with a replacement also include `Link: <successor>; rel="successor-version"`.

| Deprecated endpoint | Replacement |
|---|---|
| `GET /api/v1/poli` | `GET /api/v1/apm/poli` |
| `GET /api/v1/dokter` | `GET /api/v1/apm/dokter` |
| `GET /api/v1/patients/lookup` | `POST /api/v1/apm/lookup` |
| `POST /api/v1/registration` | `POST /api/v1/apm/queue` with `mode=registration` |
| `POST /api/v1/regestration` | `POST /api/v1/apm/queue` with `mode=registration` |
| `POST /api/v1/queue/empty` | `POST /api/v1/apm/queue` with `mode=blank` |
| `POST /api/v1/queue/new` | `POST /api/v1/apm/queue` with `mode=blank` |
| `GET /api/v1/registration/options` | `GET /api/v1/apm/options` for petugas/poscheck; other settings via `config.json` or CLI `options` |

## Manual removal

When old callers have migrated, remove:

- route registrations in `internal/httpapi/httpapi.go`
- legacy handlers that are no longer referenced
- README sections for the removed endpoints
- this checklist once the endpoints are gone

Keep `/healthz` and the `/api/v1/apm/*` facade.

## Supported API

Use the APM facade for new integrations:

- `POST /api/v1/apm/lookup`
- `GET /api/v1/apm/options`
- `GET /api/v1/apm/poli`
- `GET /api/v1/apm/dokter`
- `POST /api/v1/apm/queue`

The facade requires `x-api-key` from `APM_API_KEY`.
