BEGIN;

ALTER TABLE webhook_endpoints
    ADD COLUMN secret_ciphertext TEXT;

COMMIT;
