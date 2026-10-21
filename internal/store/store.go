// Package store provides short, atomic, service-owned persistence transactions.
package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"sync"

	_ "github.com/jackc/pgx/v5/stdlib"
)

var namespaces = map[string]map[string]bool{
	"udm": {"subscribers": true},
	"amf": {"equipments": true, "registrations": true, "session_events": true},
	"smf": {"sessions": true},
	"upf": {"routes": true, "settings": true},
}

type Store struct {
	mu     sync.Mutex
	db     *sql.DB
	schema string
	data   map[string]map[string][]byte
}
type Tx struct {
	ctx    context.Context
	db     *sql.Tx
	owner  *Store
	writes map[string]map[string][]byte
}

func NewMemory(schema string) *Store {
	return &Store{schema: schema, data: make(map[string]map[string][]byte)}
}
func NewPostgres(ctx context.Context, url, schema string) (*Store, error) {
	if namespaces[schema] == nil {
		return nil, fmt.Errorf("unknown storage owner %q", schema)
	}
	db, err := sql.Open("pgx", url)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(12)
	db.SetMaxIdleConns(4)
	if err = db.PingContext(ctx); err != nil {
		db.Close()
		return nil, err
	}
	return &Store{schema: schema, db: db}, nil
}
func (s *Store) Close() error {
	if s.db != nil {
		return s.db.Close()
	}
	return nil
}
func (s *Store) Ping(ctx context.Context) error {
	if s.db != nil {
		return s.db.PingContext(ctx)
	}
	return ctx.Err()
}

// Run never contains network calls. All mutations commit together or roll back.
func (s *Store) Run(ctx context.Context, fn func(*Tx) error) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	tx := &Tx{ctx: ctx, owner: s, writes: make(map[string]map[string][]byte)}
	if s.db != nil {
		var err error
		tx.db, err = s.db.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		defer tx.db.Rollback()
		if _, err = tx.db.ExecContext(ctx, "SELECT pg_advisory_xact_lock(hashtext($1))", "core5g:"+s.schema); err != nil {
			return err
		}
	}
	if err := fn(tx); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if tx.db != nil {
		return tx.db.Commit()
	}
	for table, values := range tx.writes {
		if s.data[table] == nil {
			s.data[table] = make(map[string][]byte)
		}
		for id, value := range values {
			s.data[table][id] = value
		}
	}
	return nil
}
func (tx *Tx) table(name string) (string, error) {
	if !namespaces[tx.owner.schema][name] {
		return "", fmt.Errorf("table %q is not owned by %s", name, tx.owner.schema)
	}
	return tx.owner.schema + "." + name, nil
}
func (tx *Tx) Get(table, id string, target any) (bool, error) {
	qualified, err := tx.table(table)
	if err != nil {
		return false, err
	}
	var value []byte
	if tx.db != nil {
		err = tx.db.QueryRowContext(tx.ctx, "SELECT data FROM "+qualified+" WHERE id=$1", id).Scan(&value)
		if errors.Is(err, sql.ErrNoRows) {
			return false, nil
		}
		if err != nil {
			return false, err
		}
	} else {
		var exists bool
		value, exists = tx.writes[table][id]
		if !exists {
			value, exists = tx.owner.data[table][id]
		}
		if !exists {
			return false, nil
		}
	}
	return true, json.Unmarshal(value, target)
}
func (tx *Tx) Put(table, id string, value any) error {
	qualified, err := tx.table(table)
	if err != nil {
		return err
	}
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	if tx.db != nil {
		_, err = tx.db.ExecContext(tx.ctx, "INSERT INTO "+qualified+" (id,data) VALUES ($1,$2) ON CONFLICT(id) DO UPDATE SET data=EXCLUDED.data", id, data)
		return err
	}
	if tx.writes[table] == nil {
		tx.writes[table] = make(map[string][]byte)
	}
	tx.writes[table][id] = data
	return nil
}
func All[T any](tx *Tx, table string) ([]*T, error) {
	qualified, err := tx.table(table)
	if err != nil {
		return nil, err
	}
	values := make([][]byte, 0)
	if tx.db != nil {
		rows, err := tx.db.QueryContext(tx.ctx, "SELECT data FROM "+qualified+" ORDER BY id")
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		for rows.Next() {
			var data []byte
			if err = rows.Scan(&data); err != nil {
				return nil, err
			}
			values = append(values, data)
		}
		if err = rows.Err(); err != nil {
			return nil, err
		}
	} else {
		merged := make(map[string][]byte)
		for id, data := range tx.owner.data[table] {
			merged[id] = data
		}
		for id, data := range tx.writes[table] {
			merged[id] = data
		}
		ids := make([]string, 0, len(merged))
		for id := range merged {
			ids = append(ids, id)
		}
		sort.Strings(ids)
		for _, id := range ids {
			values = append(values, merged[id])
		}
	}
	result := make([]*T, 0, len(values))
	for _, value := range values {
		item := new(T)
		if err := json.Unmarshal(value, item); err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, nil
}
