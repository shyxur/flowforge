import type { Metadata } from "next";
import Link from "next/link";
import { CreateWebhookForm } from "./create-form";

export const metadata: Metadata = { title: "new webhook" };

export default function NewWebhookPage() {
  return (
    <div className="narrow-page">
      <Link className="back-link" href="/webhooks">
        ← back to webhooks
      </Link>
      <p className="eyebrow">eventforge</p>
      <h1 className="page-title">new webhook endpoint</h1>
      <p className="page-copy">
        subscribe an HTTPS endpoint to selected task lifecycle events. a signing
        secret is generated after creation and displayed once.
      </p>
      <CreateWebhookForm />
    </div>
  );
}
