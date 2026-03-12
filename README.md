# gradlog

Self-hosted ML experiment tracker.

Gradlog runs as a single backend service (Go + Gin + Postgres) and can serve the website directly from the same container. You do not need Cloudflare Pages when deploying this way.

## Architecture

- Backend API: Go/Gin
- Database: PostgreSQL 17
- Auth: opaque API/session tokens (no JWT)
- UI hosting: static frontend files served by backend

## Run With Docker Compose

1. Copy env template:

```bash
cp backend/.env.example backend/.env
```

2. Edit `backend/.env` with your values.

3. Build and start:

```bash
docker compose up --build
```

4. Open:

- API health: `http://localhost:8080/health`
- Website: `http://localhost:8080/`

## Frontend Served By Backend

The backend binary embeds static files under `backend/internal/ui/dist` and serves them for non-API routes.

In the current Docker setup, `frontend/` is copied into `backend/internal/ui/dist/` during image build. This means a single backend container serves both API and website.

## Google OAuth (Optional)

If you enable Google OAuth, set these values in `backend/.env`:

- `GOOGLE_CLIENT_ID`
- `GOOGLE_CLIENT_SECRET`
- `GOOGLE_REDIRECT_URL`

For domain `gradlog.efesirin.com` use:

- Authorized JavaScript origins: `https://gradlog.efesirin.com`
- Authorized redirect URI: `https://gradlog.efesirin.com/api/v1/auth/google/callback`

And set:

```env
GOOGLE_REDIRECT_URL=https://gradlog.efesirin.com/api/v1/auth/google/callback
FRONTEND_URL=https://gradlog.efesirin.com
```

## Notes

- `JWT_SECRET` is not used.
- If OAuth is not configured, token-based API key auth still works.
