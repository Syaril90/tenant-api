# API

Standalone Go API for the `v2` property operations platform.

## Stack

- Go
- PostgreSQL
- GORM
- Modular monolith

## Current slices

- `property`
- `billing`
- `announcements`
- `complaints`
- `feedback`
- `visitors`
- `documents`
- `identity`
- `adminaccess`
- `approvals`

## Run

```bash
cd api
docker compose up --build
```

The API auto-migrates the schema and seeds sample property, billing, and announcement data on startup.
Announcement images uploaded from the admin dashboard are stored on local disk under `storage/` and served through `/media/...`.

## Environment

- `HTTP_ADDRESS`
- `DATABASE_URL`
- `STORAGE_ROOT`
- `PUBLIC_BASE_URL`
- `BILLPLZ_API_BASE_URL`
- `BILLPLZ_API_KEY`
- `BILLPLZ_X_SIGNATURE_KEY`
- `BILLPLZ_COLLECTION_ID`
- `BILLPLZ_CALLBACK_BASE_URL`

Copy `.env.example` if you want local overrides outside Docker.

## Endpoints

- `GET /healthz`
- `GET /api/v1/property/tree`
- `GET /api/v1/property/units`
- `GET /api/v1/property/resident-accounts?email=...`
- `GET /api/v1/property/owner-tenants?ownerAccountCodes=...`
- `POST /api/v1/property/owner-tenants`
- `GET /api/v1/admin/billing/tree`
- `POST /api/v1/admin/billing/charges`
- `POST /api/v1/admin/billing/payments`
- `POST /api/v1/billing/billplz/checkout`
- `POST /api/v1/billing/billplz/confirm`
- `POST /api/v1/billing/payments/billplz/callback`
- `GET /api/v1/resident/billing/{unitCode}`
- `GET /api/v1/announcements`
- `GET /api/v1/announcements/{announcementId}`
- `POST /api/v1/admin/announcements`
- `GET /api/v1/admin/complaints`
- `GET /api/v1/admin/complaints/{complaintId}`
- `PATCH /api/v1/admin/complaints/{complaintId}/status`
- `GET /api/v1/resident-complaints`
- `GET /api/v1/resident-complaints/{complaintId}`
- `POST /api/v1/resident-complaints`
- `GET /api/v1/admin/feedback`
- `GET /api/v1/admin/feedback/{feedbackId}`
- `GET /api/v1/resident-feedback`
- `GET /api/v1/resident-feedback/{feedbackId}`
- `POST /api/v1/resident-feedback`
- `GET /api/v1/visitor-requests`
- `POST /api/v1/visitor-requests`
- `GET /api/v1/admin/visitor-requests`
- `PUT /api/v1/admin/visitor-requests`
- `GET /api/v1/admin/visitor-parking-configs`
- `PUT /api/v1/admin/visitor-parking-configs`

## Compatibility aliases

The API is being migrated toward clearer resource-oriented routes while preserving current clients.
These legacy aliases still work for now:

- `/api/v1/billing/admin/tree`
- `/api/v1/billing/admin/charges`
- `/api/v1/billing/admin/payments`
- `/api/v1/billing/resident/{unitCode}`
- `/api/v1/announcements/admin`
- `/api/v1/complaints`
- `/api/v1/complaints/{complaintId}`
- `/api/v1/complaints/{complaintId}/status`
- `/api/v1/feedback`
- `/api/v1/feedback/{feedbackId}`
- `/api/v1/resident-feedback` remains the canonical resident route while `/api/v1/feedback` is legacy-compatible
- `/api/v1/visitors`
- `/api/v1/visitors/admin`
- `/api/v1/admin/visitor-requests/workspace`

## Error responses

The canonical error response shape is:

```json
{
  "code": "bad_request",
  "message": "email query parameter is required",
  "error": "email query parameter is required"
}
```

`code` is the stable machine-readable field and currently maps to:

- `bad_request`
- `not_found`
- `conflict`
- `method_not_allowed`
- `internal_error`

`error` is still returned as a compatibility field for older clients; new clients should prefer `code` and `message`.

## Response shapes

Collection endpoints return:

```json
{
  "items": []
}
```

Single-resource endpoints return:

```json
{
  "item": {}
}
```

During the current migration, some older clients may still accept raw single-resource payloads. New and updated clients in this repo now prefer the canonical envelope while remaining backward-compatible.
