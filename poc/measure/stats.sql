-- Zeilenzahlen und Crosswalk-Kennzahlen der ingestierten Datenbank
-- (M1/M2 des Reality-Checks). Aufruf:
--   sqlite3 poc/measure/out/m2.sqlite < poc/measure/stats.sql
.mode list
.headers off
SELECT 'name_rows',                COUNT(*) FROM name;
SELECT 'concept_rows',             COUNT(*) FROM taxon_concept;
SELECT 'concept_name_accepted',    COUNT(*) FROM concept_name WHERE role='accepted';
SELECT 'concept_name_synonym',     COUNT(*) FROM concept_name WHERE role='synonym';
SELECT 'distribution_rows',        COUNT(*) FROM distribution;
SELECT 'distinct_area_codes',      COUNT(DISTINCT area_code) FROM distribution;
SELECT 'xref_rows',                COUNT(*) FROM xref;
SELECT 'fts_name_map_rows',        COUNT(*) FROM fts_name_map;
SELECT 'trait_value_rows',         COUNT(*) FROM trait_value;
SELECT 'concepts_with_any_trait',  COUNT(DISTINCT concept_id) FROM trait_value;
SELECT 'concepts_with_eive',       COUNT(DISTINCT concept_id) FROM trait_value WHERE vocab='eive';
SELECT 'concepts_with_tichy',      COUNT(DISTINCT concept_id) FROM trait_value WHERE vocab='tichy2023';
SELECT 'concepts_with_midolo',     COUNT(DISTINCT concept_id) FROM trait_value WHERE vocab='midolo2023';
SELECT 'concepts_with_eive_and_tichy', COUNT(*) FROM (
  SELECT concept_id FROM trait_value WHERE vocab='eive'
  INTERSECT
  SELECT concept_id FROM trait_value WHERE vocab='tichy2023');
SELECT 'concepts_with_all_three', COUNT(*) FROM (
  SELECT concept_id FROM trait_value WHERE vocab='eive'
  INTERSECT
  SELECT concept_id FROM trait_value WHERE vocab='tichy2023'
  INTERSECT
  SELECT concept_id FROM trait_value WHERE vocab='midolo2023');
SELECT 'concepts_rank_'||rank, COUNT(*) FROM taxon_concept GROUP BY rank;
SELECT 'names_rank_'||rank, COUNT(*) FROM name GROUP BY rank;
SELECT 'concepts_in_GER', COUNT(DISTINCT concept_id) FROM distribution WHERE area_code='GER';
