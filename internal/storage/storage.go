package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"chipmov/internal/domain"

	_ "modernc.org/sqlite"
)

type Store struct {
	db *sql.DB
}

func Open(path string) (*Store, error) {
	if dir := filepath.Dir(path); dir != "." && dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, err
		}
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	return &Store{db: db}, nil
}

func (s *Store) Close() error {
	return s.db.Close()
}

func (s *Store) Migrate(ctx context.Context) error {
	schema := []string{
		`create table if not exists allowed_cnpjs (
			id integer primary key,
			cnpj text not null unique,
			name text,
			active integer not null default 1,
			created_at text not null
		)`,
		`create table if not exists iccids (
			id integer primary key,
			cnpj text not null,
			subscriber_name text,
			sim_card text not null unique,
			phone_number text,
			contract_number text,
			contract_status text,
			plan_name text,
			last_recharge_at text,
			next_recharge_due_at text,
			default_quantity integer not null default 1,
			recharge_interval_months integer not null default 11,
			safety_window_days integer not null default 10,
			auto_recharge_enabled integer not null default 1,
			last_sync_at text not null,
			created_at text not null,
			updated_at text not null
		)`,
		`create table if not exists gb_operations (
			id integer primary key,
			sim_card text not null,
			cnpj text,
			quantity integer not null,
			status text not null,
			trigger_type text not null,
			easy2use_status_code integer,
			easy2use_user_message text,
			request_payload text,
			response_payload text,
			error_message text,
			created_at text not null,
			finished_at text
		)`,
		`create table if not exists automation_runs (
			id integer primary key,
			started_at text not null,
			finished_at text,
			status text not null,
			checked_count integer not null default 0,
			recharged_count integer not null default 0,
			skipped_count integer not null default 0,
			failed_count integer not null default 0,
			summary text
		)`,
		`create table if not exists last_recharge_syncs (
			id integer primary key,
			started_at text not null,
			finished_at text,
			status text not null,
			items_found integer not null default 0,
			items_updated integer not null default 0,
			error_message text
		)`,
	}
	for _, statement := range schema {
		if _, err := s.db.ExecContext(ctx, statement); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) UpsertAllowedCNPJs(ctx context.Context, cnpjs []string) error {
	now := formatTime(time.Now())
	for _, cnpj := range cnpjs {
		if _, err := s.db.ExecContext(ctx, `insert into allowed_cnpjs (cnpj, active, created_at) values (?, 1, ?)
			on conflict(cnpj) do update set active = 1`, cnpj, now); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) IsAllowedCNPJ(ctx context.Context, cnpj string) (bool, error) {
	var active int
	err := s.db.QueryRowContext(ctx, `select active from allowed_cnpjs where cnpj = ?`, cnpj).Scan(&active)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return active == 1, nil
}

type UpsertICCIDParams struct {
	CNPJ                   string
	SubscriberName         string
	SimCard                string
	PhoneNumber            string
	ContractNumber         string
	ContractStatus         string
	PlanName               string
	DefaultQuantity        int
	RechargeIntervalMonths int
	SafetyWindowDays       int
}

func (s *Store) UpsertICCID(ctx context.Context, p UpsertICCIDParams) error {
	now := formatTime(time.Now())
	_, err := s.db.ExecContext(ctx, `insert into iccids (
			cnpj, subscriber_name, sim_card, phone_number, contract_number, contract_status,
			plan_name, default_quantity, recharge_interval_months, safety_window_days,
			auto_recharge_enabled, last_sync_at, created_at, updated_at
		) values (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 1, ?, ?, ?)
		on conflict(sim_card) do update set
			cnpj = excluded.cnpj,
			subscriber_name = excluded.subscriber_name,
			phone_number = excluded.phone_number,
			contract_number = excluded.contract_number,
			contract_status = excluded.contract_status,
			plan_name = excluded.plan_name,
			default_quantity = excluded.default_quantity,
			recharge_interval_months = excluded.recharge_interval_months,
			safety_window_days = excluded.safety_window_days,
			last_sync_at = excluded.last_sync_at,
			updated_at = excluded.updated_at`,
		p.CNPJ, p.SubscriberName, p.SimCard, p.PhoneNumber, p.ContractNumber, p.ContractStatus,
		p.PlanName, p.DefaultQuantity, p.RechargeIntervalMonths, p.SafetyWindowDays, now, now, now)
	return err
}

func (s *Store) ListICCIDs(ctx context.Context) ([]domain.ICCID, error) {
	rows, err := s.db.QueryContext(ctx, `select id, cnpj, subscriber_name, sim_card, phone_number, contract_number,
		contract_status, plan_name, last_recharge_at, next_recharge_due_at, default_quantity,
		recharge_interval_months, safety_window_days, auto_recharge_enabled, last_sync_at, created_at, updated_at
		from iccids order by next_recharge_due_at is null desc, next_recharge_due_at asc, sim_card asc`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanICCIDs(rows)
}

func (s *Store) ListDueICCIDs(ctx context.Context, now time.Time) ([]domain.ICCID, error) {
	rows, err := s.db.QueryContext(ctx, `select id, cnpj, subscriber_name, sim_card, phone_number, contract_number,
		contract_status, plan_name, last_recharge_at, next_recharge_due_at, default_quantity,
		recharge_interval_months, safety_window_days, auto_recharge_enabled, last_sync_at, created_at, updated_at
		from iccids
		where auto_recharge_enabled = 1
		  and next_recharge_due_at is not null
		  and next_recharge_due_at <= ?
		order by next_recharge_due_at asc`, formatDate(now))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanICCIDs(rows)
}

func (s *Store) GetICCID(ctx context.Context, simCard string) (domain.ICCID, error) {
	row := s.db.QueryRowContext(ctx, `select id, cnpj, subscriber_name, sim_card, phone_number, contract_number,
		contract_status, plan_name, last_recharge_at, next_recharge_due_at, default_quantity,
		recharge_interval_months, safety_window_days, auto_recharge_enabled, last_sync_at, created_at, updated_at
		from iccids where sim_card = ?`, simCard)
	return scanICCID(row)
}

func (s *Store) UpdateLastRecharge(ctx context.Context, simCard string, lastRecharge time.Time, intervalMonths int, safetyWindowDays int) error {
	next := domain.ComputeNextRecharge(lastRecharge, intervalMonths, safetyWindowDays)
	_, err := s.db.ExecContext(ctx, `update iccids set last_recharge_at = ?, next_recharge_due_at = ?, updated_at = ? where sim_card = ?`,
		formatDate(lastRecharge), formatDate(next), formatTime(time.Now()), simCard)
	return err
}

func (s *Store) CreateOperation(ctx context.Context, op domain.GBOperation) (int64, error) {
	result, err := s.db.ExecContext(ctx, `insert into gb_operations (
		sim_card, cnpj, quantity, status, trigger_type, request_payload, created_at
	) values (?, ?, ?, ?, ?, ?, ?)`, op.SimCard, op.CNPJ, op.Quantity, op.Status, op.TriggerType, op.RequestPayload, formatTime(time.Now()))
	if err != nil {
		return 0, err
	}
	return result.LastInsertId()
}

func (s *Store) FinishOperation(ctx context.Context, id int64, status string, statusCode *int, userMessage string, responsePayload string, errorMessage string) error {
	_, err := s.db.ExecContext(ctx, `update gb_operations set status = ?, easy2use_status_code = ?, easy2use_user_message = ?,
		response_payload = ?, error_message = ?, finished_at = ? where id = ?`,
		status, statusCode, userMessage, responsePayload, errorMessage, formatTime(time.Now()), id)
	return err
}

func (s *Store) ListOperations(ctx context.Context, limit int) ([]domain.GBOperation, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	rows, err := s.db.QueryContext(ctx, `select id, sim_card, cnpj, quantity, status, trigger_type, easy2use_status_code,
		easy2use_user_message, request_payload, response_payload, error_message, created_at, finished_at
		from gb_operations order by id desc limit ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	ops := []domain.GBOperation{}
	for rows.Next() {
		var op domain.GBOperation
		var code sql.NullInt64
		var created, finished sql.NullString
		if err := rows.Scan(&op.ID, &op.SimCard, &op.CNPJ, &op.Quantity, &op.Status, &op.TriggerType, &code,
			&op.Easy2UseUserMessage, &op.RequestPayload, &op.ResponsePayload, &op.ErrorMessage, &created, &finished); err != nil {
			return nil, err
		}
		if code.Valid {
			c := int(code.Int64)
			op.Easy2UseStatusCode = &c
		}
		if created.Valid {
			t, _ := parseStoredTime(created.String)
			op.CreatedAt = t
		}
		if finished.Valid {
			t, _ := parseStoredTime(finished.String)
			op.FinishedAt = &t
		}
		ops = append(ops, op)
	}
	return ops, rows.Err()
}

func (s *Store) CreateAutomationRun(ctx context.Context) (int64, error) {
	result, err := s.db.ExecContext(ctx, `insert into automation_runs (started_at, status) values (?, 'running')`, formatTime(time.Now()))
	if err != nil {
		return 0, err
	}
	return result.LastInsertId()
}

func (s *Store) FinishAutomationRun(ctx context.Context, id int64, status string, checked int, recharged int, skipped int, failed int, summary string) error {
	_, err := s.db.ExecContext(ctx, `update automation_runs set finished_at = ?, status = ?, checked_count = ?, recharged_count = ?,
		skipped_count = ?, failed_count = ?, summary = ? where id = ?`,
		formatTime(time.Now()), status, checked, recharged, skipped, failed, summary, id)
	return err
}

func (s *Store) NextRun(ctx context.Context) (*time.Time, int, error) {
	var value sql.NullString
	var count int
	err := s.db.QueryRowContext(ctx, `select min(next_recharge_due_at), count(*) from iccids
		where auto_recharge_enabled = 1 and next_recharge_due_at is not null`).Scan(&value, &count)
	if err != nil {
		return nil, 0, err
	}
	if !value.Valid {
		return nil, count, nil
	}
	t, err := parseDate(value.String)
	if err != nil {
		return nil, count, err
	}
	return &t, count, nil
}

type scanner interface {
	Scan(dest ...any) error
}

func scanICCID(row scanner) (domain.ICCID, error) {
	var item domain.ICCID
	var lastRecharge, nextDue, lastSync, created, updated sql.NullString
	var auto int
	err := row.Scan(&item.ID, &item.CNPJ, &item.SubscriberName, &item.SimCard, &item.PhoneNumber, &item.ContractNumber,
		&item.ContractStatus, &item.PlanName, &lastRecharge, &nextDue, &item.DefaultQuantity,
		&item.RechargeIntervalMonths, &item.SafetyWindowDays, &auto, &lastSync, &created, &updated)
	if err != nil {
		return item, err
	}
	if lastRecharge.Valid {
		t, err := parseDate(lastRecharge.String)
		if err == nil {
			item.LastRechargeAt = &t
		}
	}
	if nextDue.Valid {
		t, err := parseDate(nextDue.String)
		if err == nil {
			item.NextRechargeDueAt = &t
		}
	}
	item.AutoRechargeEnabled = auto == 1
	if lastSync.Valid {
		t, _ := parseStoredTime(lastSync.String)
		item.LastSyncAt = t
	}
	if created.Valid {
		t, _ := parseStoredTime(created.String)
		item.CreatedAt = t
	}
	if updated.Valid {
		t, _ := parseStoredTime(updated.String)
		item.UpdatedAt = t
	}
	return item, nil
}

func scanICCIDs(rows *sql.Rows) ([]domain.ICCID, error) {
	items := []domain.ICCID{}
	for rows.Next() {
		item, err := scanICCID(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func formatTime(t time.Time) string {
	return t.UTC().Format(time.RFC3339)
}

func formatDate(t time.Time) string {
	return t.Format("2006-01-02")
}

func parseDate(value string) (time.Time, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return time.Time{}, fmt.Errorf("empty date")
	}
	return time.ParseInLocation("2006-01-02", value, time.Local)
}

func parseStoredTime(value string) (time.Time, error) {
	if t, err := time.Parse(time.RFC3339, value); err == nil {
		return t, nil
	}
	return parseDate(value)
}
