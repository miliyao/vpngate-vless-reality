# Changelog

## v0.9.0-rc.2 - 2026-06-23

- Added `/healthz` and authenticated `/api/system/status` operational endpoints.
- Added Docker Compose health checks for the web panel and worker.
- Added backup and restore scripts for `.env` and persistent data.
- Updated backend dependencies to clear production npm audit findings.

## v0.9.0-rc.1 - 2026-06-23

- Added SQLite persistence and job history for egress operations.
- Split long-running create, rebuild, and delete work into the worker process.
- Added task queue visibility and task detail viewing in the web panel.
- Added subscription output for all active egress nodes.
- Added HTTP Basic Auth for the web panel and API.
- Added `.env.example` and release-oriented deployment defaults.
- Updated default Reality SNI to `www.amd.com`.
