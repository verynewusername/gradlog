# GRADLOG

Gradlog is a self-hosted ML experiment tracker built for teams that want authentication and access control out of the box.

It was created from a simple idea: MLflow-style tracking is great, but many teams still need stronger built-in auth and multi-user access patterns for real deployments. Gradlog keeps the workflow simple while adding production-friendly auth and project access management.

Gradlog runs as a single service (Go + Gin + Postgres) and serves the web UI directly from the same container.

## Architecture

- API + Web server: Go/Gin
- Database: PostgreSQL 
- Auth: opaque API/session tokens (no JWT)
- UI hosting: static frontend files served by backend

## Gradlog vs MLflow

Gradlog is intentionally built as a lightweight, self-hosted alternative for teams that want a simpler operational footprint.

- Language/runtime: Gradlog is written in Go and runs as a single compiled service.
- Deployment model: one service + Postgres, with API and UI served together.
- Auth focus: built-in user auth, project membership, and API key flows for multi-user teams.
- Operational overhead: no large Python web stack for the tracking server itself.

MLflow remains a strong choice for broad ecosystem integrations and established workflows. Gradlog focuses on teams that prefer a smaller, auth-first tracker with straightforward self-hosting.

## Run With Docker Compose

1. Copy env template:

```bash
cp gradlog/.env.example gradlog/.env
```

2. Edit `gradlog/.env` with your values.

3. Build and start:

```bash
docker compose up --build
```

4. Open:

- API health: `http://localhost:8080/health`
- Website: `http://localhost:8080/`

## Frontend Served By Backend

The gradlog binary embeds static files under `gradlog/internal/ui/dist` and serves them for non-API routes.

In the current setup, UI files are committed under `gradlog/internal/ui/dist` and embedded at build time. This means a single container serves both API and website.

## Google OAuth (Optional)

If you enable Google OAuth, set these values in `gradlog/.env`:

- `GOOGLE_CLIENT_ID`
- `GOOGLE_CLIENT_SECRET`
- `GOOGLE_REDIRECT_URL`

For your domain (e.g. `your-domain.com`) use:

- Authorized JavaScript origins: `https://your-domain.com`
- Authorized redirect URI: `https://your-domain.com/api/v1/auth/google/callback`

And set:

```env
GOOGLE_REDIRECT_URL=https://your-domain.com/api/v1/auth/google/callback
FRONTEND_URL=https://your-domain.com
```

## Notes

- `JWT_SECRET` is not used.
- If OAuth is not configured, token-based API key auth still works.

## Dual Licensing

Gradlog is dual-licensed.

- Open source use: GNU GPL v3 (see [LICENSE](LICENSE)).
- Commercial use: commercial license terms (see [LICENSE-COMMERCIAL](LICENSE-COMMERCIAL)).

### GPL v3 path

You can use Gradlog under GPL v3 for scenarios such as:

- Personal and non-commercial usage
- Academic and research usage
- GPL v3-compatible open source projects

### Commercial license path

A commercial license is required for uses such as:

- Closed-source or proprietary distribution
- Hosted/managed SaaS offerings
- Internal commercial platforms that do not comply with GPL v3 obligations

### Commercial licensing contact

For commercial licensing, contact: gradlog@efesirin.com

## Contributing

Pull requests are welcome.
