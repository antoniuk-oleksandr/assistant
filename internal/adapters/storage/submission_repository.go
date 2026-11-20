package storage

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/alex/assistent/internal/adapters/mappers"
	"github.com/alex/assistent/internal/adapters/models"
	"github.com/alex/assistent/internal/domain/ports"
	"github.com/alex/assistent/internal/domain/submission"
)

type SQLiteSubmissionRepository struct{ db *sql.DB }

func NewSubmissionRepository(db *sql.DB) ports.SubmissionRepository {
	return &SQLiteSubmissionRepository{db: db}
}

func (r *SQLiteSubmissionRepository) Upsert(ctx context.Context, item submission.Submission) error {
	if item.State == "" {
		item.State = submission.Unknown
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin submission upsert: %w", err)
	}
	defer tx.Rollback()
	if item.TaskID == 0 {
		if err := tx.QueryRowContext(ctx, `SELECT id FROM tasks WHERE external_task_id=? ORDER BY id LIMIT 1`, item.TaskExternalID).Scan(&item.TaskID); err != nil {
			return fmt.Errorf("find task for submission %q: %w", item.ExternalID, err)
		}
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO submissions (task_id, external_submission_id, submitter_id, submitter_email, submitter_name, state, submitted_at, created_at_external, updated_at_external, late, first_seen_at, last_seen_at)
VALUES (?, ?, ?, ?, ?, ?, NULLIF(?, ''), NULLIF(?, ''), NULLIF(?, ''), ?, `+sqliteNowUTC+`, `+sqliteNowUTC+`)
ON CONFLICT(task_id, external_submission_id) DO UPDATE SET submitter_id=excluded.submitter_id, submitter_email=excluded.submitter_email, submitter_name=excluded.submitter_name, state=excluded.state, submitted_at=excluded.submitted_at, created_at_external=excluded.created_at_external, updated_at_external=excluded.updated_at_external, late=excluded.late, last_seen_at=excluded.last_seen_at`, item.TaskID, item.ExternalID, item.SubmitterID, item.SubmitterEmail, item.SubmitterName, item.State, item.SubmittedAt, item.CreatedAt, item.UpdatedAt, item.Late)
	if err != nil {
		return fmt.Errorf("upsert submission %q: %w", item.ExternalID, err)
	}
	var submissionID int64
	if err := tx.QueryRowContext(ctx, `SELECT id FROM submissions WHERE task_id=? AND external_submission_id=?`, item.TaskID, item.ExternalID).Scan(&submissionID); err != nil {
		return fmt.Errorf("find submission %q: %w", item.ExternalID, err)
	}
	for _, material := range item.Materials {
		if err := upsertMaterial(ctx, tx, submissionID, material); err != nil {
			return err
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit submission %q: %w", item.ExternalID, err)
	}
	return nil
}

func upsertMaterial(ctx context.Context, tx *sql.Tx, submissionID int64, item submission.Material) error {
	if item.Role == "" {
		item.Role = "supporting_evidence"
	}
	if item.Status == "" {
		item.Status = submission.Pending
	}
	parsed, err := mappers.EncodeMaterialParsing(item.Parsing)
	if err != nil {
		return fmt.Errorf("encode material parse result: %w", err)
	}
	var id int64
	query := `SELECT id FROM materials WHERE submission_id=? AND external_material_id=?`
	args := []any{submissionID, item.ExternalID}
	if item.ExternalID == "" {
		query = `SELECT id FROM materials WHERE submission_id=? AND external_material_id IS NULL AND name=? AND COALESCE(source_uri, '')=?`
		args = []any{submissionID, item.Name, item.SourceURI}
	}
	err = tx.QueryRowContext(ctx, query, args...).Scan(&id)
	if err == sql.ErrNoRows {
		err = tx.QueryRowContext(ctx, `INSERT INTO materials (submission_id, external_material_id, name, mime_type, size_bytes, source_uri, local_path, appendix_number, role, status, parse_result, created_at)
VALUES (?, NULLIF(?, ''), ?, NULLIF(?, ''), ?, NULLIF(?, ''), NULLIF(?, ''), ?, ?, ?, ?, `+sqliteNowUTC+`) RETURNING id`, submissionID, item.ExternalID, item.Name, item.MIMEType, item.SizeBytes, item.SourceURI, item.LocalPath, item.AppendixNumber, item.Role, item.Status, parsed).Scan(&id)
	} else if err == nil {
		err = updateMaterial(ctx, tx, id, item, parsed)
	}
	if err != nil {
		return fmt.Errorf("upsert material %q: %w", item.Name, err)
	}
	return storeMaterialParses(ctx, tx, id, item.ParseOutcomes)
}

func updateMaterial(ctx context.Context, tx *sql.Tx, id int64, item submission.Material, parsed *string) error {
	if item.LocalPath != "" {
		if err := archiveLegacyParse(ctx, tx, id, item.LocalPath); err != nil {
			return err
		}
		// A successful read replaces the whole snapshot, including an absent size
		// (for example, when the file is now a native Workspace document).
		_, err := tx.ExecContext(ctx, `UPDATE materials SET name=?, mime_type=NULLIF(?, ''),
size_bytes=?, source_uri=NULLIF(?, ''), local_path=?, appendix_number=?, role=?, status=?,
parse_result=CASE WHEN ? IS NULL AND local_path=? THEN parse_result ELSE ? END WHERE id=?`,
			item.Name, item.MIMEType, item.SizeBytes, item.SourceURI, item.LocalPath, item.AppendixNumber, item.Role, item.Status, parsed, item.LocalPath, parsed, id)
		return err
	}

	// Record a failed attempt without changing the last good snapshot. Rows
	// without a snapshot may still receive new provider metadata. A Classroom
	// reference without an attempt can fill gaps but cannot reset the outcome.
	attempted := item.Status != submission.Pending
	metadataObserved := attempted || parsed != nil || len(item.ParseOutcomes) != 0
	_, err := tx.ExecContext(ctx, `UPDATE materials SET
name=CASE WHEN local_path IS NULL AND (? OR name='') THEN COALESCE(NULLIF(?, ''), name) ELSE name END,
mime_type=CASE WHEN local_path IS NULL AND (? OR mime_type IS NULL) THEN COALESCE(NULLIF(?, ''), mime_type) ELSE mime_type END,
size_bytes=CASE WHEN local_path IS NULL AND (? OR size_bytes IS NULL) THEN COALESCE(?, size_bytes) ELSE size_bytes END,
source_uri=CASE WHEN local_path IS NULL AND (? OR source_uri IS NULL) THEN COALESCE(NULLIF(?, ''), source_uri) ELSE source_uri END,
status=CASE WHEN ? OR (? AND local_path IS NULL) THEN ? ELSE status END,
parse_result=CASE WHEN local_path IS NULL THEN COALESCE(?, parse_result) ELSE parse_result END WHERE id=?`,
		metadataObserved, item.Name, metadataObserved, item.MIMEType, metadataObserved, item.SizeBytes, metadataObserved, item.SourceURI,
		attempted, metadataObserved, item.Status, parsed, id)
	return err
}

func (r *SQLiteSubmissionRepository) ListByTask(ctx context.Context, taskID int64) ([]submission.Submission, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT id, task_id, external_submission_id, submitter_id, submitter_email, submitter_name, state, COALESCE(submitted_at,''), COALESCE(created_at_external,''), COALESCE(updated_at_external,''), late FROM submissions WHERE task_id=? ORDER BY external_submission_id`, taskID)
	if err != nil {
		return nil, fmt.Errorf("list submissions: %w", err)
	}
	rowsResult := make([]models.SubmissionRow, 0)
	for rows.Next() {
		var row models.SubmissionRow
		if err := rows.Scan(&row.ID, &row.TaskID, &row.ExternalID, &row.SubmitterID, &row.SubmitterEmail, &row.SubmitterName, &row.State, &row.SubmittedAt, &row.CreatedAt, &row.UpdatedAt, &row.Late); err != nil {
			rows.Close()
			return nil, fmt.Errorf("scan submission: %w", err)
		}
		rowsResult = append(rowsResult, row)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, fmt.Errorf("read submissions: %w", err)
	}
	if err := rows.Close(); err != nil {
		return nil, fmt.Errorf("close submissions: %w", err)
	}

	result := make([]submission.Submission, 0, len(rowsResult))
	for _, row := range rowsResult {
		materials, err := r.materials(ctx, row.ID)
		if err != nil {
			return nil, err
		}
		current := mappers.ToSubmission(row, materials)
		outcomes, err := r.materialParses(ctx, row.ID)
		if err != nil {
			return nil, err
		}
		for i, material := range materials {
			parsed, err := mappers.DecodeMaterialParsing(material.ParseResult)
			if err != nil {
				return nil, fmt.Errorf("decode parse result for material %d: %w", material.ID, err)
			}
			current.Materials[i].Parsing = parsed
			current.Materials[i].ParseOutcomes = outcomes[material.ID]
		}
		result = append(result, current)
	}
	return result, nil
}

func (r *SQLiteSubmissionRepository) materials(ctx context.Context, submissionID int64) ([]models.MaterialRow, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT id, submission_id, COALESCE(external_material_id,''), name, COALESCE(mime_type,''), size_bytes, COALESCE(source_uri,''), COALESCE(local_path,''), appendix_number, role, status, parse_result FROM materials WHERE submission_id=? ORDER BY id`, submissionID)
	if err != nil {
		return nil, fmt.Errorf("list materials: %w", err)
	}
	defer rows.Close()
	result := make([]models.MaterialRow, 0)
	for rows.Next() {
		var row models.MaterialRow
		if err := rows.Scan(&row.ID, &row.SubmissionID, &row.ExternalID, &row.Name, &row.MIMEType, &row.SizeBytes, &row.SourceURI, &row.LocalPath, &row.AppendixNumber, &row.Role, &row.Status, &row.ParseResult); err != nil {
			return nil, fmt.Errorf("scan material: %w", err)
		}
		result = append(result, row)
	}
	return result, rows.Err()
}
