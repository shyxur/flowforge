BEGIN;

ALTER TABLE webhook_endpoints
    DROP COLUMN IF EXISTS secret_ciphertext;

COMMIT;
