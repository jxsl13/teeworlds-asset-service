-- Drop in reverse dependency order.
DROP TABLE IF EXISTS asset_item_metadata;
DROP TABLE IF EXISTS search_value;
DROP TABLE IF EXISTS search_value_weight;
DROP TABLE IF EXISTS asset_item;
DROP TABLE IF EXISTS asset_group;
DROP TYPE  IF EXISTS asset_type_enum;
DROP EXTENSION IF EXISTS pg_trgm;
