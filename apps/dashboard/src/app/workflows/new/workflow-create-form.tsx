"use client";

import { FormEvent, useState, useTransition } from "react";
import { useRouter } from "next/navigation";
import { createWorkflowAction } from "../actions";

export function CreateWorkflowForm() {
  const router = useRouter();
  const [pending, startTransition] = useTransition();
  const [name, setName] = useState("");
  const [description, setDescription] = useState("");
  const [error, setError] = useState("");

  function submit(event: FormEvent) {
    event.preventDefault();
    if (!name.trim()) {
      setError("workflow name is required");
      return;
    }
    setError("");
    startTransition(async () => {
      const result = await createWorkflowAction({ name, description });
      if (!result.ok) {
        setError(result.error);
        return;
      }
      router.push(`/workflows/${result.data.id}?created=1`);
    });
  }

  return (
    <form className="panel resource-form" onSubmit={submit}>
      <label>
        <span>name</span>
        <input
          autoComplete="off"
          autoFocus
          maxLength={255}
          onChange={(event) => setName(event.target.value)}
          placeholder="Order lifecycle"
          required
          value={name}
        />
      </label>
      <label>
        <span>description · optional</span>
        <textarea
          maxLength={2000}
          onChange={(event) => setDescription(event.target.value)}
          placeholder="What this workflow coordinates"
          value={description}
        />
      </label>
      {error && (
        <p aria-live="polite" className="form-error">
          {error}
        </p>
      )}
      <div className="form-actions">
        <button className="button button-primary" disabled={pending} type="submit">
          {pending ? "creating…" : "create workflow"}
        </button>
      </div>
    </form>
  );
}
