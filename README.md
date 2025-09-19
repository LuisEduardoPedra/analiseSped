# analiseSped Development

This branch introduces configurable CORS settings to make local testing easier.

## Environment variables

Create a `.env` file in the project root with the following contents:

```env
JWT_SECRET=your-dev-secret
ALLOWED_ORIGINS=http://localhost:5173,https://analise-sped-frontend.vercel.app
```

The application loads this file automatically at startup. When `ALLOWED_ORIGINS` is not defined, the application defaults to allowing only `https://analise-sped-frontend.vercel.app`. To allow any origin during ad-hoc testing, set `ALLOWED_ORIGINS` to `*` (not recommended for production).


Values already present in the environment will not be overridden by variables defined in `.env`.

## Running the server

After creating the `.env` file, start the server with:

After creating the `.env` file, start the server with:

```bash
go run ./cmd/web
```
