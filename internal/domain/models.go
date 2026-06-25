package domain

import "time"

type ICCID struct {
	ID                     int64      `json:"id"`
	CNPJ                   string     `json:"cnpj"`
	SubscriberName         string     `json:"subscriber_name"`
	SimCard                string     `json:"sim_card"`
	PhoneNumber            string     `json:"phone_number"`
	ContractNumber         string     `json:"contract_number"`
	ContractStatus         string     `json:"contract_status"`
	PlanName               string     `json:"plan_name"`
	StockStatus            string     `json:"stock_status,omitempty"`
	StockIncludedAt        *time.Time `json:"stock_included_at,omitempty"`
	ESim                   *bool      `json:"esim,omitempty"`
	Operator               string     `json:"operator,omitempty"`
	StockSyncAt            *time.Time `json:"stock_sync_at,omitempty"`
	LastRechargeAt         *time.Time `json:"last_recharge_at,omitempty"`
	NextRechargeDueAt      *time.Time `json:"next_recharge_due_at,omitempty"`
	DefaultQuantity        int        `json:"default_quantity"`
	RechargeIntervalMonths int        `json:"recharge_interval_months"`
	SafetyWindowDays       int        `json:"safety_window_days"`
	AutoRechargeEnabled    bool       `json:"auto_recharge_enabled"`
	LastSyncAt             time.Time  `json:"last_sync_at"`
	CreatedAt              time.Time  `json:"created_at"`
	UpdatedAt              time.Time  `json:"updated_at"`
}

type GBOperation struct {
	ID                  int64      `json:"id"`
	SimCard             string     `json:"sim_card"`
	CNPJ                string     `json:"cnpj"`
	Quantity            int        `json:"quantity"`
	Status              string     `json:"status"`
	TriggerType         string     `json:"trigger_type"`
	Easy2UseStatusCode  *int       `json:"easy2use_status_code,omitempty"`
	Easy2UseUserMessage string     `json:"easy2use_user_message,omitempty"`
	RequestPayload      string     `json:"request_payload,omitempty"`
	ResponsePayload     string     `json:"response_payload,omitempty"`
	ErrorMessage        string     `json:"error_message,omitempty"`
	CreatedAt           time.Time  `json:"created_at"`
	FinishedAt          *time.Time `json:"finished_at,omitempty"`
}

type AutomationRun struct {
	ID             int64      `json:"id"`
	StartedAt      time.Time  `json:"started_at"`
	FinishedAt     *time.Time `json:"finished_at,omitempty"`
	Status         string     `json:"status"`
	CheckedCount   int        `json:"checked_count"`
	RechargedCount int        `json:"recharged_count"`
	SkippedCount   int        `json:"skipped_count"`
	FailedCount    int        `json:"failed_count"`
	Summary        string     `json:"summary,omitempty"`
}

type RechargeApproval struct {
	ID                int64      `json:"id"`
	SimCard           string     `json:"sim_card"`
	CNPJ              string     `json:"cnpj"`
	SubscriberName    string     `json:"subscriber_name"`
	ContractStatus    string     `json:"contract_status"`
	Quantity          int        `json:"quantity"`
	Status            string     `json:"status"`
	Reason            string     `json:"reason"`
	LastRechargeAt    *time.Time `json:"last_recharge_at,omitempty"`
	NextRechargeDueAt *time.Time `json:"next_recharge_due_at,omitempty"`
	OperationID       *int64     `json:"operation_id,omitempty"`
	CreatedAt         time.Time  `json:"created_at"`
	ApprovedAt        *time.Time `json:"approved_at,omitempty"`
	RejectedAt        *time.Time `json:"rejected_at,omitempty"`
	FinishedAt        *time.Time `json:"finished_at,omitempty"`
}

type RechargeDecision struct {
	ICCID  ICCID  `json:"iccid"`
	Reason string `json:"reason"`
}

func ComputeNextRecharge(lastRechargeAt time.Time, intervalMonths int, safetyWindowDays int) time.Time {
	return lastRechargeAt.AddDate(0, intervalMonths, -safetyWindowDays)
}
