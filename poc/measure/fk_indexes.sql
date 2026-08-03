-- Indizes auf den KIND-Spalten aller Fremdschluessel, die das Serienschema
-- (internal/adapters/sqlite/schema.sql) nicht anlegt. Ohne sie muss SQLite
-- bei jedem `INSERT OR REPLACE` (= DELETE + INSERT) die referenzierenden
-- Tabellen vollstaendig scannen, um die FK-Constraints zu pruefen — das
-- macht den Ingest quadratisch (M1-Befund).
--
-- Diese Datei ist ein MESS-Werkzeug, kein Patch: sie wird vor dem Ingest
-- auf eine bereits mit dem Serienschema angelegte, leere DB angewendet
-- (`CREATE TABLE IF NOT EXISTS` laesst sie danach unangetastet).
CREATE INDEX IF NOT EXISTS m_idx_name_basionym       ON name(basionym_id);
CREATE INDEX IF NOT EXISTS m_idx_tc_parent           ON taxon_concept(parent_id);
CREATE INDEX IF NOT EXISTS m_idx_tc_accepted_name    ON taxon_concept(accepted_name);
CREATE INDEX IF NOT EXISTS m_idx_tc_backbone         ON taxon_concept(backbone_id);
CREATE INDEX IF NOT EXISTS m_idx_cn_name             ON concept_name(name_id);
CREATE INDEX IF NOT EXISTS m_idx_xref_concept        ON xref(concept_id);
CREATE INDEX IF NOT EXISTS m_idx_ftsmap_concept      ON fts_name_map(concept_id);
CREATE INDEX IF NOT EXISTS m_idx_cr_to               ON concept_relation(to_concept);
