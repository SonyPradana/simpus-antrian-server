# Antrian API

A Go API and CLI for issuing queue numbers for patients in the SIMPUS database, including optional BPJS/PCare participant verification and blank/new-patient queue numbers.

This project recreates the APM Touch registration flow. It supports:

- existing-patient registration: create a `visits` row, create four `finger_unit` queue rows, and return the queue number plus patient/BPJS data
- blank/new-patient queue numbers (`empty`/`new`): create four blank `finger_unit` rows and one `finger_laporan` row, without creating a patient or visit

## Status

Implemented:

- Patient lookup by NIK, BPJS card number, or medical record number.
- Active polyclinic list.
- Today's doctor schedule lookup from MJKN data.
- Automatic doctor selection when a polyclinic has exactly one doctor.
- Duplicate registration check for the same patient, date, and unit.
- Global and per-polyclinic queue number calculation.
- Dry-run mode that does not write to the database.
- Calls to `SPapmsimpus` and `SPinsertFingerUnit`.
- Blank/new-patient queue numbers without patient or visit records.
- JSON registration settings at `<exe-dir>/config.json`.
- Registration option lookup endpoint and CLI command.
- BPJS/PCare participant verification.
- REST API and CLI.
- APM facade for other applications under `/api/v1/apm/*`, protected by `x-api-key`.

Not implemented yet:

- SIAP online queue server integration.
- MJKN/BPJS Antrean Online integration (separate project).
- JWT authentication; the APM facade uses a static API key instead.
- Kiosk or web UI.
- Batch/scheduled queue ordering beyond the database advisory lock.

## Registration Flow

1. Resolve the patient by `nik`, `no_bpjs`, or `no_rm`.
2. Resolve the polyclinic from `bpjs_polyclinics_puskesmas`.
3. Resolve the doctor from `finger_mjkn_dokter` for the polyclinic and today's date.
4. Check whether the patient already has a `visits` row for the same polyclinic today.
5. Verify the patient with BPJS/PCare when the patient is a BPJS participant. A successful verification also supplies the BPJS provider code and name used for "Nama Faskes".
6. Calculate the queue number:
   - global number: maximum `finger_unit.NoUrut` for unit `Pendaftaran` today + 1
   - polyclinic number: maximum `finger_unit.NoUrut` for that polyclinic today + 1
   - queue code: polyclinic prefix plus the three-digit polyclinic number, for example `A001`
7. Call `SPapmsimpus`.
8. Call `SPinsertFingerUnit`.
9. Insert a `finger_laporan` row.

The queue calculation is intentionally based on the legacy tables, not on an application counter or cache.

## Blank and New-Patient Queue Numbers

`empty` and `new` are queue-only flows for patients who do not have (or do not want to provide) a SIMPUS identity yet. They produce the same shape as the legacy `A001`/`A002` rows:

- no patient lookup
- no `visits` row
- no `SPapmsimpus` call
- no BPJS/PCare call
- four `finger_unit` rows with blank patient fields
- one `finger_laporan` row: `Daftar`, `idpasien = NULL`, `poli_bpjs = '-'`, `test = 'OnSite'`

Both endpoints require `poli_id`; `kode_dokter` is required when more than one doctor is available. Capacity rules are the same as normal registration.

The advisory lock `GET_LOCK('apm:queue:<date>')` is acquired on a dedicated database connection and held through commit, so normal registration and queue-only requests cannot allocate the same number concurrently.

## Requirements

- Go 1.27 or newer.
- Access to the SIMPUS MariaDB/MySQL database.
- A local `.env` in the working directory (recommended) or equivalent environment variables. The legacy `koneksi.jjk` file is optional and only read when `KONEKSI_FILE` is set.
- PCare credentials are optional. Without them, BPJS verification can be disabled globally or skipped per request.

## Configuration

Settings are resolved in this order:

1. environment variables
2. `config.json`
3. legacy `koneksi.jjk` (only when `KONEKSI_FILE` is set)
4. built-in defaults for generic settings

Registration IDs are site-specific and have no built-in defaults. `WORKER_ID`, `POSCHECK_ID`, `ACTION_ID`, `CONSIGNOR_ID`, and `GROUPVISIT_ID` must come from the environment or `config.json`; otherwise the server refuses to start.

The application looks for `config.json` next to `antrian.exe`. Use `CONFIG_FILE` to point to another path. When running with `go run`, it also falls back to `config.json` in the current directory. A local `.env` in the working directory is loaded for credentials and other settings; keep it uncommitted.

Example `config.json`:

```json
{
  "registrasi": {
    "petugas_id": "1",
    "jenis_pasien_id": "1",
    "jenis_puskesmas_id": "1",
    "jenis_tindakan_id": "1",
    "jenis_pengirim_id": "1",
    "jenis_kunjungan_id": "1"
  }
}
```

| JSON field | Used as | Meaning |
|---|---|---|
| `petugas_id` | `WORKER_ID` | Registration worker |
| `jenis_pasien_id` | `GROUPVISIT_ID` | `groupvisits.id` |
| `jenis_puskesmas_id` | `POSCHECK_ID` | `poschecks.id`; also determines `lokasi` |
| `jenis_tindakan_id` | `ACTION_ID` | `actions.id` |
| `jenis_pengirim_id` | `CONSIGNOR_ID` | `consignors.id` |
| `jenis_kunjungan_id` | `JENIS_KUNJUNGAN_ID` | `0` = Sehat, `1` = Sakit |

`jenis_kunjungan_id` is the global `sKunjSakit` value used by normal registration. The per-polyclinic `poliSakit` column is no longer used for `visits.kunj_sakit`.

The legacy `koneksi.jjk` file is only read when `KONEKSI_FILE` points to it. The server also loads `.env` from the working directory when present; that file is the preferred local source and must never be committed. See `.env.example` and `config.example.json` for the full list.

Supported environment variables:

| Variable | Default | Description |
|---|---|---|
| `APP_ADDR` | `:8080` | HTTP listen address |
| `APP_TIMEZONE` | `Asia/Jakarta` | Application timezone |
| `APP_USER_AGENT` | `simpus-antrian` | User-Agent sent to PCare |
| `KONEKSI_FILE` | empty | Path to the optional legacy connection file |
| `CONFIG_FILE` | `<exe-dir>/config.json` | Path to the JSON registration settings |
| `DB_DSN` | empty | MySQL DSN; overrides individual DB settings |
| `DB_HOST` | from `KONEKSI_FILE` | Database host |
| `DB_PORT` | `3306` | Database port |
| `DB_USER` | from `KONEKSI_FILE` | Database user |
| `DB_PASSWORD` | from `KONEKSI_FILE` | Database password |
| `DB_NAME` | `simpus` | Database name |
| `PCARE_BASE_URL` | `https://new-apijkn.bpjs-kesehatan.go.id/pcare-rest` | PCare base URL |
| `PCARE_MODE` | `warn` | `off`, `warn`, or `strict` |
| `PCARE_KODE_APLIKASI` | empty | PCare `kdAplikasi` used in `X-Authorization`; required when PCare is active |
| `PCARE_CACHE_TTL` | `10m` | PCare response cache duration |
| `APM_API_KEY` | empty | API key for `/api/v1/apm/*`; the facade is disabled when empty |
| `CORS_ALLOWED_ORIGIN` | empty | Allowed browser origin; when empty no CORS header is sent |
| `ALLOW_OVERQUOTA` | `false` | Allow queue numbers beyond doctor capacity |
| `HTTP_TIMEOUT` | `15s` | PCare request timeout |
| `LOCATION` | from `poschecks` | Examination location; overrides `jenis_puskesmas_id` lookup |
| `WORKER_ID` | **required** | Registration worker ID |
| `POSCHECK_ID` | **required** | Poscheck ID |
| `ACTION_ID` | **required** | Action ID |
| `CONSIGNOR_ID` | **required** | Consignor ID |
| `GROUPVISIT_ID` | **required** | Group visit ID |
| `JENIS_KUNJUNGAN_ID` | `1` | Global visit type: `0` Sehat, `1` Sakit |

PCare notes:

- Every PCare environment variable has a default except `PCARE_KODE_APLIKASI`, which must be set when PCare verification is enabled.
- Credentials are read from the SIMPUS database: `healthcenters` for consumer/PCare credentials and `finger_userkey` for the user key. When `finger_userkey` is empty, `finger_mjkn_userkey.userkey` is used as a fallback.
- `PCARE_MODE=warn` still creates the queue when PCare is unreachable, but rejects inactive participants and date-of-birth mismatches.
- `PCARE_MODE=strict` rejects the registration when verification fails.
- The request-level `pcare` option defaults to `true`. Set `"pcare": false` in the JSON body or `?pcare=false` in the query string to skip PCare for that request only.
- When PCare is skipped, the response uses local BPJS data and sets `"sumber": "local"` with `"pesan": "verifikasi BPJS dilewati"`.

### BPJS provider and "Nama Faskes"

During normal registration, the server passes `bpjs.kode_provider` and `bpjs.nama_provider` to `SPinsertFingerUnit`. The procedure writes them to `finger_unit.kdProviderPeserta` and `finger_unit.namaprovider`. SIMPUS displays `namaprovider` as **Nama Faskes**.

PCare supplies these values from the participant's `kdProviderPst` object. When registration runs with `"pcare": false`, the provider fields stay empty, so the queue row can appear with an empty "Nama Faskes" even though the registration itself succeeded. Keep `pcare=true` for production registration unless skipping verification is intentional.

The provider name returned by PCare may contain a leading space (for example `" Puskesmas Contoh"`). The server stores it unchanged to match existing SIMPUS rows.

## Build and Run

From the project directory:

```bash
go mod tidy
go build -o antrian ./cmd/antrian
```

Start the HTTP server:

```bash
./antrian serve -addr :8080
```

Or run it without building:

```bash
go run ./cmd/antrian serve -addr :8080
```

Run the health check:

```bash
curl -sS http://localhost:8080/healthz
```

## CLI

### List polyclinics

```bash
./antrian poli
```

### List doctors

```bash
./antrian dokter -poli 01
```

When only one doctor is available, the response contains `"otomatis": true` and a `dipilih` object.

### Registration options

```bash
./antrian options
```

Returns the available workers, patient types, poschecks, actions, consignors, and visit types, plus the values currently selected in `config.json` and the resolved `lokasi`.

### Blank or new-patient queue number

Dry-run first:

```bash
./antrian empty -poli 01 -dokter 00001 -dry-run
./antrian new -poli 01 -dokter 00001 -dry-run
```

Create the queue number:

```bash
./antrian empty -poli 01 -dokter 00001
./antrian new -poli 01 -dokter 00001
```

`empty` and `new` have the same database behavior. `new` is intended for a new patient but still only creates a blank queue number; it does not create a patient record.

### Look up a patient

Send exactly one identifier: `-nik`, `-no-bpjs`, or `-no-rm`.

```bash
./antrian lookup -no-rm 000001
./antrian lookup -nik 0000000000000001
./antrian lookup -no-bpjs 0000000000001
./antrian lookup -no-rm 000001 -pcare=false
```

### Registration dry-run

Dry-run does not write to the database:

```bash
./antrian register -no-rm 000001 -poli 01 -dry-run
./antrian register -no-rm 000001 -poli 01 -dry-run -pcare=false
```

Select a doctor explicitly:

```bash
./antrian register -no-rm 000001 -poli 01 -dokter 00001 -dry-run
```

### Real registration

This command **writes to the production database**:

```bash
./antrian register -no-rm 000001 -poli 01
```

Do not run this without permission, a backup, and an understanding that the main tables use MyISAM, so transactional rollback is unavailable.

## REST API

Base URL: `http://localhost:8080`

Successful responses use this envelope:

```json
{ "data": {} }
```

Error responses use this envelope:

```json
{
  "error": {
    "code": "PATIENT_NOT_FOUND",
    "message": "pasien tidak ditemukan"
  }
}
```

All examples below use Bash-compatible `curl`. On Windows, run them in Git Bash or WSL, or use `curl.exe` with a JSON file and `--data-binary @file.json`.

### APM facade (`/api/v1/apm/*`)

This facade is intended for another application that needs to reproduce the APM Touch manual flow: lookup, choose a polyclinic, choose a doctor, then take a queue number. All operations use today's date; there is no tomorrow booking.

Every request must include:

```http
X-API-Key: <APM_API_KEY>
```

If the key is missing or wrong, the response is `401 UNAUTHORIZED`. If `APM_API_KEY` is not configured, the facade returns `503 APM_NOT_CONFIGURED`; the legacy endpoints remain available.

#### `GET /api/v1/apm/options`

Returns active registration workers and available poscheck/location options:

```bash
curl -sS http://localhost:8080/api/v1/apm/options \
  -H "X-API-Key: $APM_API_KEY"
```

```json
{
  "data": {
    "petugas": [{ "id": "1", "nama": "..." }],
    "poscheck": [{ "id": "1", "nama": "Puskesmas Contoh" }],
    "terpilih": {
      "petugas_id": "1",
      "poscheck_id": "1",
      "lokasi": "Puskesmas Contoh"
    }
  }
}
```

`terpilih` shows the server defaults. Use `petugas_id` and `poscheck_id` from this response in the queue request.

#### `POST /api/v1/apm/lookup`

```bash
curl -sS -X POST http://localhost:8080/api/v1/apm/lookup \
  -H 'Content-Type: application/json' \
  -H "X-API-Key: $APM_API_KEY" \
  -d '{"no_rm":"000001"}'
```

Send exactly one of `nik`, `no_bpjs`/`bpjs`, or `no_rm`. The response is always `200` when the request is valid, including when the patient is not found.

```json
{
  "data": {
    "mode": "existing_active",
    "can_register": true,
    "patient": { "...": "..." },
    "bpjs": { "...": "..." }
  }
}
```

Modes:

- `existing_active`: non-BPJS or BPJS active; use `mode: "registration"`.
- `existing_inactive`: BPJS inactive; use `mode: "blank"`.
- `existing_bpjs_unknown`: BPJS status cannot be verified; `can_register` follows `PCARE_MODE`.
- `new`: patient not found; use `mode: "blank"`.

#### `GET /api/v1/apm/poli` and `GET /api/v1/apm/dokter?poli_id=05`

`poli` returns active polyclinics. `dokter` returns today's schedule for the selected polyclinic and sets `otomatis: true` plus `dipilih` when only one doctor is available.

#### `POST /api/v1/apm/queue`

For an active patient, use full registration:

```bash
curl -sS -X POST http://localhost:8080/api/v1/apm/queue \
  -H 'Content-Type: application/json' \
  -H "X-API-Key: $APM_API_KEY" \
  -d '{"mode":"registration","no_rm":"000001","poli_id":"01","kode_dokter":"00001","petugas_id":"1","poscheck_id":"1","dry_run":true}'
```

For a new or inactive patient, use a blank queue:

```bash
curl -sS -X POST http://localhost:8080/api/v1/apm/queue \
  -H 'Content-Type: application/json' \
  -H "X-API-Key: $APM_API_KEY" \
  -d '{"mode":"blank","poli_id":"01","kode_dokter":"00001","poscheck_id":"1","dry_run":true}'
```

Fields:

| Field | Required | Description |
|---|---|---|
| `mode` | yes | `registration` or `blank` |
| `nik` / `no_bpjs` / `no_rm` | for registration | Exactly one identifier |
| `poli_id` | yes | Polyclinic ID |
| `kode_dokter` | no | Required when more than one doctor is available |
| `petugas_id` | no | Active registration worker; defaults to server config. Rejected for `blank` |
| `poscheck_id` | no | Location ID from `/api/v1/apm/options`; defaults to server config |
| `dry_run` | no | Preview only |
| `pcare` | no | BPJS verification; defaults to `true`. When enabled, also fills `bpjs.kode_provider`/`bpjs.nama_provider` -> "Nama Faskes" |

`mode: "blank"` rejects patient identifiers and `petugas_id`. It creates only blank `finger_unit` rows and one `finger_laporan` row; it does not create a patient, visit, or PCare call. The ticket data contains only the queue code, `tanggal`, `waktu`, polyclinic, doctor, and location. Patient identity is captured later at the registration desk.

Request overrides take precedence over server config. The location is always derived from `poscheck_id`; the API does not accept free-text location.

Response status is `201` for a real queue and `200` for a dry-run. Other statuses and error codes match the registration/queue endpoints. The response includes `petugas_id`, `poscheck_id`, and `lokasi` when available.

### Deprecated legacy endpoints

The endpoints below are deprecated and remain temporarily for backward compatibility. Responses include `Deprecation: true`; when a replacement exists, the response also includes a `Link` header with `rel="successor-version"`. New integrations must use the APM facade.

| Deprecated endpoint | Replacement |
|---|---|
| `GET /api/v1/poli` | `GET /api/v1/apm/poli` |
| `GET /api/v1/dokter` | `GET /api/v1/apm/dokter` |
| `GET /api/v1/patients/lookup` | `POST /api/v1/apm/lookup` |
| `POST /api/v1/registration` | `POST /api/v1/apm/queue` with `"mode": "registration"` |
| `POST /api/v1/regestration` | `POST /api/v1/apm/queue` with `"mode": "registration"` |
| `POST /api/v1/queue/empty` | `POST /api/v1/apm/queue` with `"mode": "blank"` |
| `POST /api/v1/queue/new` | `POST /api/v1/apm/queue` with `"mode": "blank"` |
| `GET /api/v1/registration/options` | `GET /api/v1/apm/options` for petugas/poscheck; other settings via `config.json` or `./antrian options` |

Do not add new features to these endpoints. Remove them when the old callers have migrated. See [`docs/deprecated-api.md`](docs/deprecated-api.md) for the removal checklist.

### `GET /healthz`

```bash
curl -sS http://localhost:8080/healthz
```

Response:

```json
{ "status": "ok" }
```

### `GET /api/v1/poli` (deprecated)

Use `GET /api/v1/apm/poli` instead.

Returns active polyclinics.

```bash
curl -sS http://localhost:8080/api/v1/poli
```

### `GET /api/v1/dokter?poli_id=89` (deprecated)

Use `GET /api/v1/apm/dokter?poli_id=89` instead.

Returns doctors for the polyclinic and today's date.

```bash
curl -sS 'http://localhost:8080/api/v1/dokter?poli_id=89'
```

The query alias `poli` is also accepted.

### `GET /api/v1/registration/options` (deprecated)

Use `GET /api/v1/apm/options` for active petugas and poscheck/location choices. Other registration settings remain in `config.json` or `./antrian options`.

Returns the six registration settings from the database and the values selected in `config.json`.

```bash
curl -sS http://localhost:8080/api/v1/registration/options
```

### `POST /api/v1/queue/empty` and `POST /api/v1/queue/new` (deprecated)

Use `POST /api/v1/apm/queue` with `"mode": "blank"` instead.

Queue-only endpoints. They do not accept patient identifiers and do not create patient or visit rows.

Request body:

```json
{
  "poli_id": "01",
  "kode_dokter": "00001",
  "dry_run": true
}
```

```bash
curl -sS -X POST http://localhost:8080/api/v1/queue/empty \
  -H 'Content-Type: application/json' \
  -d '{"poli_id":"01","kode_dokter":"00001","dry_run":true}'

curl -sS -X POST http://localhost:8080/api/v1/queue/new \
  -H 'Content-Type: application/json' \
  -d '{"poli_id":"01","kode_dokter":"00001","dry_run":true}'
```

Response:

```json
{
  "data": {
    "jenis": "empty",
    "poli": {
      "id": "01",
      "nama": "Poli Contoh",
      "prefix": "A"
    },
    "dokter": {
      "kode_dokter": "00001",
      "nama_dokter": "dr. Contoh",
      "jam_praktek": "07:30-12:00",
      "kapasitas": 18
    },
    "antrian": {
      "kode": "A001",
      "no_urut_poli": 1,
      "no_urut": 80,
      "tanggal": "2026-10-22",
      "poli": "Poli Contoh",
      "dokter": "dr. Contoh",
      "kode_dokter": "00001",
      "menunggu_di_depan": 0,
      "sisa_kuota": 17
    },
    "dry_run": true
  }
}
```

`dry_run` can also be set with `?dry_run=true`. HTTP status codes are the same as registration, plus `503 QUEUE_BUSY` when the queue lock cannot be acquired within five seconds.

### `GET /api/v1/patients/lookup` (deprecated)

Use `POST /api/v1/apm/lookup` instead.

Query parameters:

- `nik`
- `no_bpjs` or `bpjs`
- `no_rm`
- `pcare` (optional, default `true`)

Send exactly one identifier. `no_bpjs` and `bpjs` are aliases, so do not send both.

```bash
curl -sS 'http://localhost:8080/api/v1/patients/lookup?no_rm=000001'
curl -sS 'http://localhost:8080/api/v1/patients/lookup?no_rm=000001&pcare=false'
```

### `POST /api/v1/registration` (deprecated)

Use `POST /api/v1/apm/queue` with `"mode": "registration"` instead.

Legacy alias: `POST /api/v1/regestration` (also deprecated).

Request body:

```json
{
  "nik": "0000000000000001",
  "no_bpjs": "0000000000001",
  "no_rm": "000001",
  "poli_id": "01",
  "kode_dokter": "00001",
  "dry_run": true
}
```

Fields:

| Field | Required | Description |
|---|---|---|
| `nik` | exactly one identifier | Patient NIK |
| `no_bpjs` / `bpjs` | exactly one identifier | BPJS card number; the two names are aliases |
| `no_rm` | exactly one identifier | Medical record number |
| `poli_id` | yes | Polyclinic ID, for example `01` or `89` |
| `kode_dokter` | no | Doctor code; required when more than one doctor is available |
| `petugas_id` | no | Active registration worker override; defaults to `WORKER_ID` |
| `poscheck_id` | no | Location override; defaults to `POSCHECK_ID`. The `lokasi` is always resolved from this ID |
| `dry_run` | no | Preview only; no database writes |
| `pcare` | no | Verify BPJS through PCare; defaults to `true` |

The legacy endpoints do not accept a free-text `lokasi` field. Send `poscheck_id`; the server reads `poschecks.nama` and writes that name to `finger_unit.Lokasi`.

Send exactly one identifier. Sending more than one or both BPJS aliases returns `INVALID_INPUT`.

Dry-run example:

```bash
curl -sS -X POST http://localhost:8080/api/v1/registration \
  -H 'Content-Type: application/json' \
  -d '{"no_rm":"000001","poli_id":"01","dry_run":true}'
```

Dry-run without PCare:

```bash
curl -sS -X POST http://localhost:8080/api/v1/registration \
  -H 'Content-Type: application/json' \
  -d '{"no_rm":"000001","poli_id":"01","dry_run":true,"pcare":false}'
```

Registration example:

```bash
curl -sS -X POST http://localhost:8080/api/v1/registration \
  -H 'Content-Type: application/json' \
  -d '{"no_rm":"000001","poli_id":"01","kode_dokter":"00001"}'
```

Response fields:

- `patient`
- `bpjs`
- `poli`
- `dokter`
- `antrian`
- `petugas_id`, `poscheck_id`, and `lokasi` when resolved
- `visit_id` except in dry-run
- `dry_run`
- `sudah_terdaftar` when a queue already exists

HTTP status codes:

- `201` registration created
- `200` dry-run succeeded
- `400` invalid input or doctor not selected
- `404` patient, polyclinic, doctor, petugas, or poscheck not found
- `409` patient already registered today
- `422` quota exceeded or invalid BPJS data
- `502` PCare failed in strict mode
- `503` queue lock busy; retry later
- `500` internal error

Error codes:

`INVALID_INPUT`, `IDENTIFIER_MISMATCH`, `PATIENT_NOT_FOUND`, `POLI_NOT_FOUND`, `DOCTOR_NOT_FOUND`, `DOCTOR_REQUIRED`, `PETUGAS_NOT_FOUND`, `POSCHECK_NOT_FOUND`, `QUOTA_EXCEEDED`, `BPJS_INCOMPLETE`, `BPJS_POLI_MISSING`, `BPJS_INACTIVE`, `BPJS_DOB_MISMATCH`, `PCARE_UNAVAILABLE`, `QUEUE_BUSY`, `ALREADY_REGISTERED`, `INTERNAL_ERROR`.

## Safe Testing

The following commands do not write data:

```bash
./antrian poli
./antrian dokter -poli 01
./antrian options
./antrian lookup -no-rm 000001
./antrian register -no-rm 000001 -poli 01 -dry-run
./antrian empty -poli 01 -dokter 00001 -dry-run
./antrian new -poli 01 -dokter 00001 -dry-run
```

For HTTP testing, start the server in one terminal and run the read-only endpoints plus a dry-run registration in another.

If a polyclinic is already full, dry-run returns `QUOTA_EXCEEDED` before any write. Use a polyclinic that still has remaining capacity.

## Project Layout

```text
cmd/antrian/          CLI and HTTP entrypoint
internal/config/      configuration, config.json, and koneksi.jjk parsing
internal/db/          database connection
internal/model/       response models
internal/repository/  patient, polyclinic, doctor, registration options, queue, and stored procedure queries
internal/pcare/       PCare client, signature, AES, LZString
internal/service/     registration and queue-only flows
internal/httpapi/     REST handlers
config.json            registration settings next to the executable
```

## Warnings

- The production database uses MyISAM for the main tables. Transactions cannot be rolled back.
- A real registration changes visit and queue data. Only run it in a test environment or after explicit approval.
- Real `empty`/`new` calls write four `finger_unit` rows and one `finger_laporan` row even though no patient or visit is created.
- Compensation logic is best effort and does not replace a backup.
- Patient data is sensitive. Do not expose this API on an open network without authentication. The `/api/v1/apm/*` facade requires `x-api-key`, but the legacy endpoints remain unauthenticated for backward compatibility.

## License

Copyright 2026 Sony Pradana.

Licensed under the Apache License, Version 2.0. See [LICENSE](LICENSE) for the full text and [NOTICE](NOTICE) for attribution.

