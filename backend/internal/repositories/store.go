package repositories

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"github.com/urestic/urestic/backend/internal/ids"
)

var (
	ErrDuplicateName = errors.New("repository name already exists")
	ErrNotFound      = errors.New("repository not found")
)

type Repository struct {
	ID                 string            `json:"id"`
	Name               string            `json:"name"`
	Backend            string            `json:"backend"`
	RepoURL            string            `json:"repoUrl"`
	PasswordCiphertext string            `json:"-"`
	Variables          map[string]string `json:"variables"`
	SecretFields       []string          `json:"secretFields"`
	Description        string            `json:"description"`
	CreatedAt          time.Time         `json:"createdAt"`
	UpdatedAt          time.Time         `json:"updatedAt"`
}

type Params struct {
	Name               string
	Backend            string
	RepoURL            string
	PasswordCiphertext string
	Variables          map[string]string
	SecretFields       []string
	Description        string
}

type Store struct {
	db *sql.DB
}

func NewStore(db *sql.DB) *Store {
	return &Store{db: db}
}

func (s *Store) List(ctx context.Context) ([]Repository, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, name, backend, repo_url, password_ciphertext, variables_json, secret_fields_json, description, created_at, updated_at FROM backup_repositories ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := []Repository{}
	for rows.Next() {
		item, err := scanRepository(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item.Public())
	}

	return items, rows.Err()
}

func (s *Store) Get(ctx context.Context, id string) (Repository, error) {
	item, err := s.GetPrivate(ctx, id)
	if err != nil {
		return Repository{}, err
	}
	return item.Public(), nil
}

func (s *Store) GetPrivate(ctx context.Context, id string) (Repository, error) {
	row := s.db.QueryRowContext(ctx, `SELECT id, name, backend, repo_url, password_ciphertext, variables_json, secret_fields_json, description, created_at, updated_at FROM backup_repositories WHERE id = ?`, id)
	item, err := scanRepository(row)
	if errors.Is(err, sql.ErrNoRows) {
		return Repository{}, ErrNotFound
	}
	return item, err
}

func (s *Store) Create(ctx context.Context, params Params) (Repository, error) {
	exists, err := s.nameExists(ctx, params.Name, "")
	if err != nil {
		return Repository{}, err
	}
	if exists {
		return Repository{}, ErrDuplicateName
	}

	id, err := ids.New()
	if err != nil {
		return Repository{}, err
	}
	now := time.Now().UTC()
	variablesJSON, err := json.Marshal(params.Variables)
	if err != nil {
		return Repository{}, err
	}
	secretFieldsJSON, err := json.Marshal(params.SecretFields)
	if err != nil {
		return Repository{}, err
	}

	_, err = s.db.ExecContext(ctx, `INSERT INTO backup_repositories (id, name, backend, repo_url, password_ciphertext, variables_json, secret_fields_json, description, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		id, params.Name, params.Backend, params.RepoURL, params.PasswordCiphertext, string(variablesJSON), string(secretFieldsJSON), params.Description, now.Format(time.RFC3339Nano), now.Format(time.RFC3339Nano))
	if err != nil {
		return Repository{}, err
	}

	return s.Get(ctx, id)
}

func (s *Store) Update(ctx context.Context, id string, params Params) (Repository, error) {
	if _, err := s.GetPrivate(ctx, id); err != nil {
		return Repository{}, err
	}
	exists, err := s.nameExists(ctx, params.Name, id)
	if err != nil {
		return Repository{}, err
	}
	if exists {
		return Repository{}, ErrDuplicateName
	}

	variablesJSON, err := json.Marshal(params.Variables)
	if err != nil {
		return Repository{}, err
	}
	secretFieldsJSON, err := json.Marshal(params.SecretFields)
	if err != nil {
		return Repository{}, err
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)

	_, err = s.db.ExecContext(ctx, `UPDATE backup_repositories SET name = ?, backend = ?, repo_url = ?, password_ciphertext = ?, variables_json = ?, secret_fields_json = ?, description = ?, updated_at = ? WHERE id = ?`,
		params.Name, params.Backend, params.RepoURL, params.PasswordCiphertext, string(variablesJSON), string(secretFieldsJSON), params.Description, now, id)
	if err != nil {
		return Repository{}, err
	}

	return s.Get(ctx, id)
}

func (s *Store) Delete(ctx context.Context, id string) error {
	result, err := s.db.ExecContext(ctx, `DELETE FROM backup_repositories WHERE id = ?`, id)
	if err != nil {
		return err
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

func (r Repository) Public() Repository {
	public := r
	for _, field := range public.SecretFields {
		if _, ok := public.Variables[field]; ok {
			public.Variables[field] = "********"
		}
	}
	return public
}

func (s *Store) nameExists(ctx context.Context, name string, excludeID string) (bool, error) {
	var count int
	var err error
	if excludeID == "" {
		err = s.db.QueryRowContext(ctx, `SELECT COUNT(1) FROM backup_repositories WHERE name = ?`, name).Scan(&count)
	} else {
		err = s.db.QueryRowContext(ctx, `SELECT COUNT(1) FROM backup_repositories WHERE name = ? AND id <> ?`, name, excludeID).Scan(&count)
	}
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

type scanner interface {
	Scan(dest ...any) error
}

func scanRepository(scanner scanner) (Repository, error) {
	var item Repository
	var variablesJSON string
	var secretFieldsJSON string
	var createdAt string
	var updatedAt string

	err := scanner.Scan(&item.ID, &item.Name, &item.Backend, &item.RepoURL, &item.PasswordCiphertext, &variablesJSON, &secretFieldsJSON, &item.Description, &createdAt, &updatedAt)
	if err != nil {
		return Repository{}, err
	}
	if err := json.Unmarshal([]byte(variablesJSON), &item.Variables); err != nil {
		return Repository{}, err
	}
	if err := json.Unmarshal([]byte(secretFieldsJSON), &item.SecretFields); err != nil {
		return Repository{}, err
	}
	item.CreatedAt, err = time.Parse(time.RFC3339Nano, createdAt)
	if err != nil {
		return Repository{}, err
	}
	item.UpdatedAt, err = time.Parse(time.RFC3339Nano, updatedAt)
	if err != nil {
		return Repository{}, err
	}
	if item.Variables == nil {
		item.Variables = map[string]string{}
	}
	return item, nil
}
