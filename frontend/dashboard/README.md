# Dashboard

The Angular dashboard and browser trackers live here. Use the repository workflow from the root:

```bash
./hk dev --seed
./hk fmt --scope frontend
./hk qa changed --gate frontend-unit
./hk qa changed --gate frontend-e2e
./hk screenshot /dashboard
./hk build binary
```

See [CONTRIBUTING.md](../../CONTRIBUTING.md) for setup, workspace lifecycle, and QA profiles.

- `src/app/`: dashboard pages, components, and services.
- `src/tracker/`: browser tracking code.
- `public/i18n/`: dashboard translations.

The production build embeds the dashboard, tracker bundles, and the installed Scalar API reference in the Go application.
