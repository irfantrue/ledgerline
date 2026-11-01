# API

## Response envelope

Sukses — `response.WriteSuccess`:

```json
{ "data": { "...": "..." } }
```

`Data` **tidak** memakai `omitempty`, supaya nilai nol yang sah (0, false, "") tidak hilang. Status HTTP diambil dari `Response.Status`; field itu tidak ikut di body.

## Error — RFC 7807

`response.Problem`:

```json
{
  "title": "Bad Request",
  "status": 400,
  "detail": "request validation failed",
  "invalid_params": [
    { "name": "amount", "reason": "Amount must be greater than 0" }
  ]
}
```

`title` = kategori error, `status` = kode HTTP, `detail` = penjelasan, `invalid_params` = detail per field (hanya untuk validasi body).

Helper yang tersedia di `internal/shared/httpx/response`:

| Helper | Status |
|---|---|
| `WriteSuccess(c, &response.Response[T]{...})` | sesuai `Response.Status` |
| `WriteProblem(c, &response.Problem{...})` | sesuai `Problem.Status` |
| `BadRequestResponse(c, title, detail, invalidParams)` | 400 |
| `UnauthorizedResponse(c, detail)` | 401 |
| `UnprocessableEntityResponse(c, title, detail)` | 422 |

`Problem` juga mengimplementasikan `error`, jadi bisa dikembalikan langsung dari handler. `response.IsProblem(err)` / `response.From(err)` mendeteksinya di error handler.

## Validasi body

```go
if invalid := validator.ValidateStruct(&req); invalid != nil {
    return response.BadRequestResponse(c, "Bad Request", "request validation failed", invalid)
}
```

`validator.ValidateStruct` mengembalikan `[]InvalidParam` dengan `Name` = nama field JSON dan `Reason` = pesan dari `go-playground/validator`.

## Endpoint terdaftar

| Method | Path | Status | Body |
|---|---|---|---|
| GET | `/healthz` | 200 | `{"data":{"status":"active"}}` |
| GET | `/readyz` | 200 | `{"data":{"database":"ready"}}` |
| GET | `/readyz` | 503 | Problem 503 — database tidak bisa di-ping |

`/healthz` = liveness (proses hidup). `/readyz` = readiness (dependency terjangkau); ping DB punya timeout 2 detik, dan dilewati kalau `httpx.Config.DB` nil.

Route lain untuk modul ledger belum ada — modul itu diekspos lewat package `repository`, bukan lewat HTTP.

## Header

| Header | Arah | Fungsi |
|---|---|---|
| `X-Request-ID` | request/response | Diisi middleware requestid, dipakai di log |
| `X-Timezone` | request | Nama zona waktu (mis. `Asia/Jakarta`); default dari `APP_TIMEZONE` |
| `Authorization` | request | Ada di daftar header CORS yang diizinkan |
| `Origin` | request | Divalidasi CORS |

## CORS

`APP_CORS_ALLOWED_ORIGINS` = daftar origin dipisah koma, spasi diabaikan.

| Kondisi | Perilaku |
|---|---|
| Daftar kosong, `APP_ENV != production` | `*`, credentials dimatikan |
| Daftar kosong, `APP_ENV == production` | semua origin ditolak |
| Daftar berisi `*` | wildcard; credentials dimatikan (cors middleware panic kalau wildcard + credentials) |

Di production daftar origin **wajib** diisi dan tidak boleh wildcard — `config.validateConfig` menolaknya saat load.

## Middleware helmet

| Header | Nilai |
|---|---|
| `X-XSS-Protection` | `0` (XSS auditor lama deprecated; andalkan CSP) |
| `X-Content-Type-Options` | `nosniff` |
| `X-Frame-Options` | `DENY` |
| `Strict-Transport-Security` | `max-age=31536000; includeSubDomains`, preload aktif |
| `Referrer-Policy` | `strict-origin-when-cross-origin` |

## Compress

`APP_COMPRESS_LEVEL`: `0` = mati, `1` = best speed (default), `2` = best compression.

## Error handler

`middleware.NewErrorHandler` (dipasang sebagai `fiber.Config.ErrorHandler`):

1. `*response.Problem` → ditulis apa adanya.
2. `*fiber.Error` → status + `http.StatusText(fe.Code)` + `fe.Message` (mis. 404 "Not Found").
3. Lainnya → 500 dengan detail generik "An unexpected error occurred.". Error asli hanya di-log, tidak pernah bocor ke client (isi SQL, path, dll).

## Menambah endpoint baru

1. Tulis handler di package modul (mis. `internal/<module>/infrastructure/http/`) atau handler dedicated.
2. Validasi body dengan `validator.ValidateStruct` → `[]InvalidParam` → `response.BadRequestResponse`.
3. Balas dengan `response.WriteSuccess(c, &response.Response[T]{...})`.
4. Daftarkan route di `httpx.RegisterRoutes` **setelah** middleware global.
5. Error dari repository dipetakan ke status yang tepat; error tak dikenal dibiarkan apa adanya agar error handler mengubahnya jadi 500 generik.
