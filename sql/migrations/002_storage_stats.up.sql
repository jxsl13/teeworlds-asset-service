-- Single-row table that always reflects the current sum of all item sizes.
-- The boolean PK with CHECK guarantees at most one row.
CREATE TABLE storage_stats (
    id         BOOLEAN PRIMARY KEY DEFAULT TRUE CHECK (id),
    total_size BIGINT  NOT NULL    DEFAULT 0
);

INSERT INTO storage_stats DEFAULT VALUES;

-- Maintain total_size whenever search_item rows are inserted, updated or deleted.
CREATE OR REPLACE FUNCTION storage_stats_update()
RETURNS TRIGGER AS $$
BEGIN
    IF TG_OP = 'INSERT' THEN
        UPDATE storage_stats SET total_size = total_size + NEW.size;
    ELSIF TG_OP = 'DELETE' THEN
        UPDATE storage_stats SET total_size = total_size - OLD.size;
    ELSIF TG_OP = 'UPDATE' THEN
        UPDATE storage_stats SET total_size = total_size - OLD.size + NEW.size;
    END IF;
    RETURN NULL;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER search_item_storage_stats
    AFTER INSERT OR UPDATE OF size OR DELETE ON search_item
    FOR EACH ROW EXECUTE FUNCTION storage_stats_update();
