-- Drop in reverse dependency order.
DROP TABLE IF EXISTS search_item_metadata;
DROP TABLE IF EXISTS search_value;
DROP TABLE IF EXISTS search_value_weight;
DROP TABLE IF EXISTS search_item;
DROP TYPE  IF EXISTS item_type_enum;
DROP EXTENSION IF EXISTS pg_trgm;
