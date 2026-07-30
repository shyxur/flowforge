DROP TRIGGER IF EXISTS metric_events_append_only ON metric_events;
DROP FUNCTION IF EXISTS reject_metric_event_mutation();
DROP TABLE IF EXISTS metric_events;
