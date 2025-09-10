# analiseSped Development

This branch introduces configurable CORS settings to make local testing easier.

## CORS configuration

Specify allowed origins through the `ALLOWED_ORIGINS` environment variable. Use a comma-separated list for multiple domains.

```bash
# allow local frontend and production site
export ALLOWED_ORIGINS=http://localhost:5173,https://analise-sped-frontend.vercel.app

# start the server
go run ./cmd/web
```

When `ALLOWED_ORIGINS` is not defined, the application defaults to allowing only `https://analise-sped-frontend.vercel.app`.

To allow any origin during ad-hoc testing, set `ALLOWED_ORIGINS` to `*` (not recommended for production).

## Running the server

Ensure required environment variables such as `JWT_SECRET` are configured. Then run:

```bash
go run ./cmd/web
```

