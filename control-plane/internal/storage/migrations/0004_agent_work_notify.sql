CREATE OR REPLACE FUNCTION skquad_notify_agent_work()
RETURNS trigger AS $$
DECLARE
    target_agent uuid;
    should_notify boolean := false;
BEGIN
    IF TG_TABLE_NAME = 'tasks' THEN
        target_agent := NEW.assignee_agent_id;
        should_notify := target_agent IS NOT NULL
            AND NEW.status IN ('todo', 'in-progress')
            AND (
                TG_OP = 'INSERT'
                OR OLD.assignee_agent_id IS DISTINCT FROM NEW.assignee_agent_id
                OR OLD.status IS DISTINCT FROM NEW.status
            );
    ELSIF TG_TABLE_NAME = 'messages' THEN
        target_agent := NEW.to_agent_id;
        should_notify := NEW.status = 'pending'
            AND NEW.next_retry_at <= now()
            AND NEW.expires_at > now()
            AND (
                TG_OP = 'INSERT'
                OR OLD.status IS DISTINCT FROM NEW.status
                OR OLD.next_retry_at IS DISTINCT FROM NEW.next_retry_at
            );
    END IF;

    IF should_notify THEN
        PERFORM pg_notify('skquad_agent_work', target_agent::text);
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS trg_tasks_agent_work_notify ON tasks;
CREATE TRIGGER trg_tasks_agent_work_notify
AFTER INSERT OR UPDATE OF assignee_agent_id, status ON tasks
FOR EACH ROW
EXECUTE FUNCTION skquad_notify_agent_work();

DROP TRIGGER IF EXISTS trg_messages_agent_work_notify ON messages;
CREATE TRIGGER trg_messages_agent_work_notify
AFTER INSERT OR UPDATE OF status, next_retry_at ON messages
FOR EACH ROW
EXECUTE FUNCTION skquad_notify_agent_work();
