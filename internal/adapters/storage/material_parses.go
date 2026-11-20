package storage

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/alex/assistent/internal/adapters/mappers"
	"github.com/alex/assistent/internal/domain/submission"
)

// Preserve an earlier unscoped outcome before replacing its snapshot.
func archiveLegacyParse(ctx context.Context, tx *sql.Tx, id int64, newPath string) error {
	var source submission.DocumentSource
	var data string
	err := tx.QueryRowContext(ctx, `SELECT COALESCE(external_material_id,''), name, COALESCE(mime_type,''),
COALESCE(source_uri,''), COALESCE(local_path,''), role, appendix_number, parse_result FROM materials
WHERE id=? AND COALESCE(local_path,'')<>? AND parse_result IS NOT NULL`, id, newPath).
		Scan(&source.ExternalID, &source.Name, &source.MIMEType, &source.URI, &source.LocalPath,
			&source.Role, &source.AppendixNumber, &data)
	if err == sql.ErrNoRows {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read legacy outcome: %w", err)
	}
	result, err := mappers.DecodeMaterialParsing(&data)
	if err != nil {
		return fmt.Errorf("decode legacy outcome: %w", err)
	}
	if result.Document != nil {
		source = result.Document.Source
	}
	return storeMaterialParses(ctx, tx, id, []submission.MaterialParse{{Source: source, Result: *result}})
}

func storeMaterialParses(ctx context.Context, tx *sql.Tx, materialID int64, outcomes []submission.MaterialParse) error {
	for _, outcome := range outcomes {
		data, err := mappers.EncodeMaterialParse(outcome)
		if err != nil {
			return fmt.Errorf("encode outcome for material %d: %w", materialID, err)
		}
		_, err = tx.ExecContext(ctx, `INSERT INTO material_parses (material_id, scenario_id, snapshot_path, outcome)
VALUES (?, ?, ?, ?) ON CONFLICT (material_id, scenario_id, snapshot_path) DO UPDATE SET outcome=excluded.outcome`,
			materialID, outcome.Source.ScenarioID, outcome.Source.LocalPath, data)
		if err != nil {
			return fmt.Errorf("store outcome for material %d: %w", materialID, err)
		}
	}
	return nil
}

func (r *SQLiteSubmissionRepository) materialParses(ctx context.Context, submissionID int64) (map[int64][]submission.MaterialParse, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT p.material_id, p.outcome FROM material_parses p
JOIN materials m ON m.id=p.material_id WHERE m.submission_id=? ORDER BY p.material_id, p.scenario_id, p.snapshot_path`, submissionID)
	if err != nil {
		return nil, fmt.Errorf("list material outcomes: %w", err)
	}
	defer rows.Close()
	outcomes := make(map[int64][]submission.MaterialParse)
	for rows.Next() {
		var id int64
		var data string
		if err := rows.Scan(&id, &data); err != nil {
			return nil, fmt.Errorf("scan material outcome: %w", err)
		}
		outcome, err := mappers.DecodeMaterialParse(data)
		if err != nil {
			return nil, fmt.Errorf("decode outcome for material %d: %w", id, err)
		}
		outcomes[id] = append(outcomes[id], outcome)
	}
	return outcomes, rows.Err()
}
