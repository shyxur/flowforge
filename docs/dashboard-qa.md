# Dashboard visual QA

Use this checklist before a release that changes the dashboard or brand assets.
Test with representative task and webhook data as well as empty collections.

- [ ] The windylane logo is visible in the sidebar or header.
- [ ] The windylane wordmark is visible and uses lowercase styling.
- [ ] No legacy product-name text or visual asset is visible.
- [ ] The monochrome ink, graphite, slate, mist, and paper palette is preserved.
- [ ] Task list pages load and remain usable with empty and populated results.
- [ ] Task detail pages load and format payload, result, status, and timestamps.
- [ ] Webhook endpoint list, create, and detail pages load.
- [ ] Webhook delivery list and detail pages load.
- [ ] Signing secrets appear only after creation or rotation and are not exposed
      after the user leaves the one-time display.
- [ ] `QUEUEFLOW_API_KEY` remains server-only and is absent from browser bundles,
      requests, rendered markup, and client-side logs.
- [ ] Empty, loading, and error states use consistent spacing, borders, and copy.
- [ ] Navigation, tables, forms, and actions work at mobile, tablet, and desktop
      widths.

Current visual references are documented in
[the windylane brand system](brand/brand-system.md). Dashboard screenshots, when
captured from a real local environment, belong in [`docs/screenshots/`](screenshots/).
