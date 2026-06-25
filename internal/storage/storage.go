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

	"mvnodashboard/internal/domain"

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
			stock_status text,
			stock_included_at text,
			esim integer,
			operator text,
			stock_sync_at text,
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
		`create table if not exists recharge_approvals (
			id integer primary key,
			sim_card text not null,
			cnpj text not null,
			subscriber_name text,
			contract_status text,
			quantity integer not null,
			status text not null,
			reason text,
			last_recharge_at text,
			next_recharge_due_at text,
			operation_id integer,
			created_at text not null,
			approved_at text,
			rejected_at text,
			finished_at text
		)`,
		`create unique index if not exists idx_recharge_approvals_open_sim_card
			on recharge_approvals(sim_card)
			where status in ('pending', 'approved', 'processing')`,
	}
	for _, statement := range schema {
		if _, err := s.db.ExecContext(ctx, statement); err != nil {
			return err
		}
	}
	if err := s.ensureICCIDStockColumns(ctx); err != nil {
		return err
	}
	return nil
}

func (s *Store) ensureICCIDStockColumns(ctx context.Context) error {
	columns := map[string]string{
		"stock_status":      "alter table iccids add column stock_status text",
		"stock_included_at": "alter table iccids add column stock_included_at text",
		"esim":              "alter table iccids add column esim integer",
		"operator":          "alter table iccids add column operator text",
		"stock_sync_at":     "alter table iccids add column stock_sync_at text",
	}
	rows, err := s.db.QueryContext(ctx, `pragma table_info(iccids)`)
	if err != nil {
		return err
	}
	defer rows.Close()
	existing := map[string]bool{}
	for rows.Next() {
		var cid int
		var name, columnType string
		var notNull int
		var defaultValue any
		var pk int
		if err := rows.Scan(&cid, &name, &columnType, &notNull, &defaultValue, &pk); err != nil {
			return err
		}
		existing[name] = true
	}
	if err := rows.Err(); err != nil {
		return err
	}
	for column, statement := range columns {
		if !existing[column] {
			if _, err := s.db.ExecContext(ctx, statement); err != nil {
				return err
			}
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

type UpsertStockSIMParams struct {
	SimCard  string
	Status   string
	Date     string
	ESim     *bool
	Operator string
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

func (s *Store) UpsertStockSIM(ctx context.Context, p UpsertStockSIMParams) error {
	now := formatTime(time.Now())
	var esim any
	if p.ESim != nil {
		if *p.ESim {
			esim = 1
		} else {
			esim = 0
		}
	}
	_, err := s.db.ExecContext(ctx, `insert into iccids (
			cnpj, sim_card, stock_status, stock_included_at, esim, operator,
			default_quantity, recharge_interval_months, safety_window_days,
			auto_recharge_enabled, stock_sync_at, last_sync_at, created_at, updated_at
		) values ('', ?, ?, ?, ?, ?, 1, 11, 10, 0, ?, ?, ?, ?)
		on conflict(sim_card) do update set
			stock_status = excluded.stock_status,
			stock_included_at = excluded.stock_included_at,
			esim = excluded.esim,
			operator = excluded.operator,
			stock_sync_at = excluded.stock_sync_at,
			updated_at = excluded.updated_at`,
		p.SimCard, p.Status, strings.TrimSpace(p.Date), esim, strings.TrimSpace(p.Operator), now, now, now, now)
	return err
}

func (s *Store) ListICCIDs(ctx context.Context) ([]domain.ICCID, error) {
	rows, err := s.db.QueryContext(ctx, `select id, cnpj, subscriber_name, sim_card, phone_number, contract_number,
		contract_status, plan_name, stock_status, stock_included_at, esim, operator, stock_sync_at,
		last_recharge_at, next_recharge_due_at, default_quantity,
		recharge_interval_months, safety_window_days, auto_recharge_enabled, last_sync_at, created_at, updated_at
		from iccids order by next_recharge_due_at is null desc, next_recharge_due_at asc, sim_card asc`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanICCIDs(rows)
}

func (s *Store) ICCIDSummary(ctx context.Context) ([]map[string]any, error) {
	rows, err := s.db.QueryContext(ctx, `select cnpj, contract_status, count(*)
		from iccids
		group by cnpj, contract_status
		order by cnpj asc, contract_status asc`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []map[string]any{}
	for rows.Next() {
		var cnpj string
		var status string
		var count int
		if err := rows.Scan(&cnpj, &status, &count); err != nil {
			return nil, err
		}
		items = append(items, map[string]any{
			"cnpj":            cnpj,
			"contract_status": status,
			"count":           count,
		})
	}
	return items, rows.Err()
}

func (s *Store) ListDueICCIDs(ctx context.Context, now time.Time) ([]domain.ICCID, error) {
	rows, err := s.db.QueryContext(ctx, `select id, cnpj, subscriber_name, sim_card, phone_number, contract_number,
		contract_status, plan_name, stock_status, stock_included_at, esim, operator, stock_sync_at,
		last_recharge_at, next_recharge_due_at, default_quantity,
		recharge_interval_months, safety_window_days, auto_recharge_enabled, last_sync_at, created_at, updated_at
		from iccids
		where auto_recharge_enabled = 1
		  and next_recharge_due_at is not null
		  and next_recharge_due_at <= ?
		  and upper(trim(contract_status)) = 'EM USO'
		  and exists (
			select 1 from allowed_cnpjs
			where allowed_cnpjs.cnpj = iccids.cnpj
			  and allowed_cnpjs.active = 1
		  )
		order by next_recharge_due_at asc`, formatDate(now))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanICCIDs(rows)
}

func (s *Store) GetICCID(ctx context.Context, simCard string) (domain.ICCID, error) {
	row := s.db.QueryRowContext(ctx, `select id, cnpj, subscriber_name, sim_card, phone_number, contract_number,
		contract_status, plan_name, stock_status, stock_included_at, esim, operator, stock_sync_at,
		last_recharge_at, next_recharge_due_at, default_quantity,
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

func (s *Store) ForceDueToday(ctx context.Context, simCard string, now time.Time) (domain.ICCID, error) {
	result, err := s.db.ExecContext(ctx, `update iccids set next_recharge_due_at = ?, updated_at = ? where sim_card = ?`,
		formatDate(now), formatTime(time.Now()), simCard)
	if err != nil {
		return domain.ICCID{}, err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return domain.ICCID{}, err
	}
	if affected == 0 {
		return domain.ICCID{}, sql.ErrNoRows
	}
	return s.GetICCID(ctx, simCard)
}

func (s *Store) ForceContractStatus(ctx context.Context, simCard string, status string) (domain.ICCID, error) {
	result, err := s.db.ExecContext(ctx, `update iccids set contract_status = ?, updated_at = ? where sim_card = ?`,
		status, formatTime(time.Now()), simCard)
	if err != nil {
		return domain.ICCID{}, err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return domain.ICCID{}, err
	}
	if affected == 0 {
		return domain.ICCID{}, sql.ErrNoRows
	}
	return s.GetICCID(ctx, simCard)
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

func (s *Store) UpsertPendingApproval(ctx context.Context, item domain.ICCID, reason string) (domain.RechargeApproval, bool, error) {
	now := formatTime(time.Now())
	var lastRecharge any
	if item.LastRechargeAt != nil {
		lastRecharge = formatDate(*item.LastRechargeAt)
	}
	var nextDue any
	if item.NextRechargeDueAt != nil {
		nextDue = formatDate(*item.NextRechargeDueAt)
	}
	result, err := s.db.ExecContext(ctx, `insert or ignore into recharge_approvals (
			sim_card, cnpj, subscriber_name, contract_status, quantity, status, reason,
			last_recharge_at, next_recharge_due_at, created_at
		) values (?, ?, ?, ?, ?, 'pending', ?, ?, ?, ?)`,
		item.SimCard, item.CNPJ, item.SubscriberName, item.ContractStatus, item.DefaultQuantity,
		reason, lastRecharge, nextDue, now)
	if err != nil {
		return domain.RechargeApproval{}, false, err
	}
	created, _ := result.RowsAffected()
	approval, err := s.GetOpenApprovalBySimCard(ctx, item.SimCard)
	return approval, created > 0, err
}

func (s *Store) ListApprovals(ctx context.Context, status string, limit int) ([]domain.RechargeApproval, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	query := `select id, sim_card, cnpj, subscriber_name, contract_status, quantity, status, reason,
		last_recharge_at, next_recharge_due_at, operation_id, created_at, approved_at, rejected_at, finished_at
		from recharge_approvals`
	args := []any{}
	if strings.TrimSpace(status) != "" {
		query += ` where status = ?`
		args = append(args, status)
	}
	query += ` order by created_at desc, id desc limit ?`
	args = append(args, limit)
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanApprovals(rows)
}

func (s *Store) GetApproval(ctx context.Context, id int64) (domain.RechargeApproval, error) {
	row := s.db.QueryRowContext(ctx, `select id, sim_card, cnpj, subscriber_name, contract_status, quantity, status, reason,
		last_recharge_at, next_recharge_due_at, operation_id, created_at, approved_at, rejected_at, finished_at
		from recharge_approvals where id = ?`, id)
	return scanApproval(row)
}

func (s *Store) GetOpenApprovalBySimCard(ctx context.Context, simCard string) (domain.RechargeApproval, error) {
	row := s.db.QueryRowContext(ctx, `select id, sim_card, cnpj, subscriber_name, contract_status, quantity, status, reason,
		last_recharge_at, next_recharge_due_at, operation_id, created_at, approved_at, rejected_at, finished_at
		from recharge_approvals where sim_card = ? and status in ('pending', 'approved', 'processing') order by id desc limit 1`, simCard)
	return scanApproval(row)
}

func (s *Store) MarkApprovalApproved(ctx context.Context, id int64) error {
	_, err := s.db.ExecContext(ctx, `update recharge_approvals set status = 'approved', approved_at = ? where id = ? and status = 'pending'`,
		formatTime(time.Now()), id)
	return err
}

func (s *Store) MarkApprovalProcessing(ctx context.Context, id int64) error {
	_, err := s.db.ExecContext(ctx, `update recharge_approvals set status = 'processing' where id = ? and status = 'approved'`, id)
	return err
}

func (s *Store) FinishApproval(ctx context.Context, id int64, status string, operationID *int64) error {
	_, err := s.db.ExecContext(ctx, `update recharge_approvals set status = ?, operation_id = ?, finished_at = ? where id = ?`,
		status, operationID, formatTime(time.Now()), id)
	return err
}

func (s *Store) RejectApproval(ctx context.Context, id int64) error {
	_, err := s.db.ExecContext(ctx, `update recharge_approvals set status = 'rejected', rejected_at = ?, finished_at = ? where id = ? and status = 'pending'`,
		formatTime(time.Now()), formatTime(time.Now()), id)
	return err
}

func (s *Store) NextRun(ctx context.Context, now time.Time) (*time.Time, int, error) {
	var value sql.NullString
	var count int
	err := s.db.QueryRowContext(ctx, `select min(next_recharge_due_at), count(*) from iccids
		where auto_recharge_enabled = 1
		  and next_recharge_due_at is not null
		  and next_recharge_due_at >= ?
		  and upper(trim(contract_status)) = 'EM USO'
		  and exists (
			select 1 from allowed_cnpjs
			where allowed_cnpjs.cnpj = iccids.cnpj
			  and allowed_cnpjs.active = 1
		  )`, formatDate(now)).Scan(&value, &count)
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

func (s *Store) ListNextRunICCIDs(ctx context.Context, next time.Time) ([]domain.ICCID, error) {
	rows, err := s.db.QueryContext(ctx, `select id, cnpj, subscriber_name, sim_card, phone_number, contract_number,
		contract_status, plan_name, stock_status, stock_included_at, esim, operator, stock_sync_at,
		last_recharge_at, next_recharge_due_at, default_quantity,
		recharge_interval_months, safety_window_days, auto_recharge_enabled, last_sync_at, created_at, updated_at
		from iccids
		where auto_recharge_enabled = 1
		  and next_recharge_due_at = ?
		  and upper(trim(contract_status)) = 'EM USO'
		  and exists (
			select 1 from allowed_cnpjs
			where allowed_cnpjs.cnpj = iccids.cnpj
			  and allowed_cnpjs.active = 1
		  )
		order by cnpj asc, sim_card asc`, formatDate(next))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanICCIDs(rows)
}

type scanner interface {
	Scan(dest ...any) error
}

func scanICCID(row scanner) (domain.ICCID, error) {
	var item domain.ICCID
	var stockIncluded, stockSync, lastRecharge, nextDue, lastSync, created, updated sql.NullString
	var cnpj, subscriberName, phoneNumber, contractNumber, contractStatus, planName, stockStatus, operator sql.NullString
	var esim sql.NullInt64
	var auto int
	err := row.Scan(&item.ID, &cnpj, &subscriberName, &item.SimCard, &phoneNumber, &contractNumber,
		&contractStatus, &planName, &stockStatus, &stockIncluded, &esim, &operator, &stockSync, &lastRecharge, &nextDue, &item.DefaultQuantity,
		&item.RechargeIntervalMonths, &item.SafetyWindowDays, &auto, &lastSync, &created, &updated)
	if err != nil {
		return item, err
	}
	if cnpj.Valid {
		item.CNPJ = cnpj.String
	}
	if subscriberName.Valid {
		item.SubscriberName = subscriberName.String
	}
	if phoneNumber.Valid {
		item.PhoneNumber = phoneNumber.String
	}
	if contractNumber.Valid {
		item.ContractNumber = contractNumber.String
	}
	if contractStatus.Valid {
		item.ContractStatus = contractStatus.String
	}
	if planName.Valid {
		item.PlanName = planName.String
	}
	if stockStatus.Valid {
		item.StockStatus = stockStatus.String
	}
	if operator.Valid {
		item.Operator = operator.String
	}
	if lastRecharge.Valid {
		t, err := parseDate(lastRecharge.String)
		if err == nil {
			item.LastRechargeAt = &t
		}
	}
	if stockIncluded.Valid {
		t, err := parseDate(stockIncluded.String)
		if err == nil {
			item.StockIncludedAt = &t
		}
	}
	if stockSync.Valid {
		t, _ := parseStoredTime(stockSync.String)
		item.StockSyncAt = &t
	}
	if esim.Valid {
		value := esim.Int64 == 1
		item.ESim = &value
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

func scanApproval(row scanner) (domain.RechargeApproval, error) {
	var item domain.RechargeApproval
	var lastRecharge, nextDue, created, approved, rejected, finished sql.NullString
	var operationID sql.NullInt64
	err := row.Scan(&item.ID, &item.SimCard, &item.CNPJ, &item.SubscriberName, &item.ContractStatus,
		&item.Quantity, &item.Status, &item.Reason, &lastRecharge, &nextDue, &operationID,
		&created, &approved, &rejected, &finished)
	if err != nil {
		return item, err
	}
	if lastRecharge.Valid {
		t, _ := parseDate(lastRecharge.String)
		item.LastRechargeAt = &t
	}
	if nextDue.Valid {
		t, _ := parseDate(nextDue.String)
		item.NextRechargeDueAt = &t
	}
	if operationID.Valid {
		id := operationID.Int64
		item.OperationID = &id
	}
	if created.Valid {
		t, _ := parseStoredTime(created.String)
		item.CreatedAt = t
	}
	if approved.Valid {
		t, _ := parseStoredTime(approved.String)
		item.ApprovedAt = &t
	}
	if rejected.Valid {
		t, _ := parseStoredTime(rejected.String)
		item.RejectedAt = &t
	}
	if finished.Valid {
		t, _ := parseStoredTime(finished.String)
		item.FinishedAt = &t
	}
	return item, nil
}

func scanApprovals(rows *sql.Rows) ([]domain.RechargeApproval, error) {
	items := []domain.RechargeApproval{}
	for rows.Next() {
		item, err := scanApproval(rows)
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
