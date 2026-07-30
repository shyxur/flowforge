# windylane dashboard

Next.js operations dashboard for QueueFlow tasks and EventForge webhooks.

Start the backend stack from the repository root with `make up`, then:

```bash
cp .env.example .env.local
npm install
npm run dev
```

Open [`http://localhost:3000`](http://localhost:3000). The local seed uses:

```text
Org ID:  00000000-0000-4000-8000-000000000001
API key: queueflow-dev-key
```

Environment variables:

```text
QUEUEFLOW_API_BASE_URL=http://localhost:8080
QUEUEFLOW_API_KEY=queueflow-dev-key
```

The dashboard reads `QUEUEFLOW_API_BASE_URL` and `QUEUEFLOW_API_KEY` only on the
server. Never expose the API key through a `NEXT_PUBLIC_` variable, client
component, browser request, or rendered markup.

Run `npm run lint` and `npm run build` before committing dashboard changes. See
the [dashboard visual QA checklist](../../docs/dashboard-qa.md) for release
review.
