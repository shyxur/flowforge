# QueueFlow Dashboard

Next.js operations dashboard for windylane QueueFlow.

```bash
cp .env.example .env.local
npm install
npm run dev
```

The dashboard reads `QUEUEFLOW_API_BASE_URL` and `QUEUEFLOW_API_KEY` only on the
server. Never rename the API key variable with a `NEXT_PUBLIC_` prefix.
