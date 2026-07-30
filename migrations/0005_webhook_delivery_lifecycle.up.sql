BEGIN;

ALTER TABLE webhook_deliveries
    DROP CONSTRAINT IF EXISTS chk_webhook_delivery_status;

UPDATE webhook_deliveries SET status = 'delivering' WHERE status = 'processing';
UPDATE webhook_deliveries SET status = 'delivered' WHERE status = 'succeeded';
UPDATE webhook_deliveries SET status = 'failed' WHERE status = 'dead_letter';

ALTER TABLE webhook_deliveries
    ADD CONSTRAINT chk_webhook_delivery_status CHECK (
        status IN ('pending', 'delivering', 'delivered', 'retrying', 'failed')
    );

COMMIT;
