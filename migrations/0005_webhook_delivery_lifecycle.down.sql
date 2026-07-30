BEGIN;

ALTER TABLE webhook_deliveries
    DROP CONSTRAINT IF EXISTS chk_webhook_delivery_status;

UPDATE webhook_deliveries SET status = 'processing' WHERE status = 'delivering';
UPDATE webhook_deliveries SET status = 'succeeded' WHERE status = 'delivered';
UPDATE webhook_deliveries SET status = 'pending' WHERE status = 'retrying';
UPDATE webhook_deliveries SET status = 'dead_letter' WHERE status = 'failed';

ALTER TABLE webhook_deliveries
    ADD CONSTRAINT chk_webhook_delivery_status CHECK (
        status IN ('pending', 'processing', 'succeeded', 'failed', 'dead_letter')
    );

COMMIT;
