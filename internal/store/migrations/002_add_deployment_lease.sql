-- Add last_status_at column for deployment lease tracking
-- This column tracks when the agent last reported status, used to detect stale deployments

-- SQLite ALTER TABLE only supports adding columns
ALTER TABLE deployment_instances ADD COLUMN last_status_at DATETIME;

-- Create index for efficient queries on stale deployments
CREATE INDEX IF NOT EXISTS idx_deployment_instances_last_status ON deployment_instances(last_status_at);
