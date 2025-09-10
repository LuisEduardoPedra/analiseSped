# analiseSped Development

This branch introduces configurable CORS settings to make local testing easier.

## CORS configuration
## Environment variables

Specify allowed origins through the `ALLOWED_ORIGINS` environment variable. Use a comma-separated list for multiple domains.
Create a `.env` file in the project root with the following contents:

```bash
# allow local frontend and production site
export ALLOWED_ORIGINS=http://localhost:5173,https://analise-sped-frontend.vercel.app

# start the server
go run ./cmd/web
```env
JWT_SECRET=your-dev-secret
ALLOWED_ORIGINS=http://localhost:5173,https://analise-sped-frontend.vercel.app
```

When `ALLOWED_ORIGINS` is not defined, the application defaults to allowing only `https://analise-sped-frontend.vercel.app`.

To allow any origin during ad-hoc testing, set `ALLOWED_ORIGINS` to `*` (not recommended for production).
The application loads this file automatically at startup. When `ALLOWED_ORIGINS` is not defined, the application defaults to allowing only `https://analise-sped-frontend.vercel.app`. To allow any origin during ad-hoc testing, set `ALLOWED_ORIGINS` to `*` (not recommended for production).

## Running the server

Ensure required environment variables such as `JWT_SECRET` are configured. Then run:
After creating the `.env` file, start the server with:

```bash
go run ./cmd/web
```
