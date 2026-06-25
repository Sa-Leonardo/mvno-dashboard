package app

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"mvnodashboard/internal/config"
	"mvnodashboard/internal/domain"
	"mvnodashboard/internal/easy2use"
	"mvnodashboard/internal/storage"

	"github.com/gin-gonic/gin"
)

type Provider interface {
	ListSubscribers(ctx context.Context) (easy2use.ListSubscribersResponse, []byte, int, error)
	ListStock(ctx context.Context) (easy2use.ListStockResponse, []byte, int, error)
	LastRecharge(ctx context.Context, simCard string) (easy2use.LastRechargeResponse, []byte, int, error)
	AddBalance(ctx context.Context, simCard string, quantity int) (easy2use.AddBalanceResponse, []byte, int, error)
}

type Server struct {
	cfg      config.Config
	store    *storage.Store
	provider Provider
	logger   *slog.Logger
}

func NewServer(cfg config.Config, store *storage.Store, provider Provider, logger *slog.Logger) *Server {
	return &Server{cfg: cfg, store: store, provider: provider, logger: logger}
}

func (s *Server) Router() http.Handler {
	router := gin.New()
	router.Use(gin.Recovery())
	router.Static("/assets", "./web/assets")
	router.StaticFile("/", "./web/index.html")
	router.StaticFile("/relatorios", "./web/relatorios.html")
	router.GET("/health", s.health)

	protected := router.Group("/")
	protected.Use(s.adminAuth())
	protected.POST("/sync/assinantes", s.syncSubscribers)
	protected.POST("/sync/estoque", s.syncStock)
	protected.POST("/sync/ultima-recarga", s.syncLastRecharges)
	protected.GET("/iccids", s.listICCIDs)
	protected.GET("/iccids/summary", s.iccidSummary)
	protected.POST("/iccids/:iccid/saldo", s.addBalanceManual)
	protected.POST("/automation/check-recharges", s.checkRecharges)
	protected.GET("/automation/next-run", s.nextRun)
	protected.GET("/recharge-approvals", s.listApprovals)
	protected.POST("/recharge-approvals/:id/approve", s.approveRecharge)
	protected.POST("/recharge-approvals/:id/reject", s.rejectRecharge)
	protected.POST("/dev/iccids/:iccid/force-due", s.forceDueDev)
	protected.GET("/operacoes", s.listOperations)
	protected.POST("/dev/iccids/:iccid/force-status", s.forceStatusDev)

	return router
}

func (s *Server) adminAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		adminKey := c.GetHeader("X-Admin-Key")
		if adminKey == "" {
			adminKey = c.GetHeader("X-API-Key")
		}
		if adminKey != s.cfg.AdminKey {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
			c.Abort()
			return
		}
		c.Next()
	}
}

func (s *Server) health(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"status": "ok",
		"time":   time.Now().Format(time.RFC3339),
	})
}

func (s *Server) syncSubscribers(c *gin.Context) {
	ctx := c.Request.Context()
	resp, raw, statusCode, err := s.provider.ListSubscribers(ctx)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{
			"error":                  err.Error(),
			"status_code":            statusCode,
			"provider_response_body": string(raw),
			"hint":                   "Confira EASY2USE_BASE_URL e EASY2USE_USER_TOKEN no .env",
		})
		return
	}
	if !easy2use.StatusCodeTipOK(resp.StatusCodeTip) {
		c.JSON(http.StatusBadGateway, gin.H{"error": "provider returned non-success codigo_status_tip", "codigo_status_tip": resp.StatusCodeTip})
		return
	}

	totalContracts := 0
	allowedSubscribers := 0
	allowedContracts := 0
	saved := 0
	skipped := 0
	savedByCNPJ := map[string]int{}
	savedByStatus := map[string]int{}
	allowedContractsByCNPJ := map[string]int{}
	savedAllowed := 0
	savedNonAllowed := 0
	for _, subscriber := range resp.Results {
		cnpj := config.OnlyDigits(subscriber.Document)
		allowed, err := s.store.IsAllowedCNPJ(ctx, cnpj)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		if allowed {
			allowedSubscribers++
		}
		for _, contract := range subscriber.Contracts {
			totalContracts++
			if strings.TrimSpace(contract.SimCard) == "" {
				skipped++
				continue
			}
			if allowed {
				allowedContracts++
				allowedContractsByCNPJ[cnpj]++
				savedAllowed++
			} else {
				savedNonAllowed++
			}
			if err := s.store.UpsertICCID(ctx, storage.UpsertICCIDParams{
				CNPJ:                   cnpj,
				SubscriberName:         subscriber.Name,
				SimCard:                strings.TrimSpace(contract.SimCard),
				PhoneNumber:            strings.TrimSpace(contract.PhoneLine),
				ContractNumber:         strings.TrimSpace(contract.ContractNumber),
				ContractStatus:         strings.TrimSpace(contract.Status),
				PlanName:               strings.TrimSpace(contract.Plan),
				DefaultQuantity:        s.cfg.DefaultRechargeQuantity,
				RechargeIntervalMonths: s.cfg.RechargeIntervalMonths,
				SafetyWindowDays:       s.cfg.RechargeSafetyWindowDays,
			}); err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
				return
			}
			saved++
			savedByCNPJ[cnpj]++
			status := strings.TrimSpace(contract.Status)
			if status == "" {
				status = "(vazio)"
			}
			savedByStatus[status]++
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"total_subscribers":         len(resp.Results),
		"total_contracts":           totalContracts,
		"allowed_subscribers":       allowedSubscribers,
		"allowed_contracts":         allowedContracts,
		"allowed_contracts_by_cnpj": allowedContractsByCNPJ,
		"saved":                     saved,
		"saved_allowed":             savedAllowed,
		"saved_non_allowed":         savedNonAllowed,
		"saved_by_cnpj":             savedByCNPJ,
		"saved_by_status":           savedByStatus,
		"skipped":                   skipped,
	})
}

func (s *Server) syncStock(c *gin.Context) {
	ctx := c.Request.Context()
	resp, raw, statusCode, err := s.provider.ListStock(ctx)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{
			"error":                  err.Error(),
			"status_code":            statusCode,
			"provider_response_body": string(raw),
			"hint":                   "Confira EASY2USE_BASE_URL e EASY2USE_USER_TOKEN no .env",
		})
		return
	}
	if !easy2use.StatusCodeTipOK(resp.StatusCodeTip) {
		c.JSON(http.StatusBadGateway, gin.H{"error": "provider returned non-success codigo_status_tip", "codigo_status_tip": resp.StatusCodeTip})
		return
	}

	saved := 0
	skipped := 0
	savedByStatus := map[string]int{}
	savedByOperator := map[string]int{}
	esimCount := 0
	for _, item := range resp.Results {
		simCard := strings.TrimSpace(item.SimCard)
		if simCard == "" {
			skipped++
			continue
		}
		if err := s.store.UpsertStockSIM(ctx, storage.UpsertStockSIMParams{
			SimCard:  simCard,
			Status:   strings.TrimSpace(item.Status),
			Date:     item.Date,
			ESim:     item.ESim,
			Operator: item.Operator,
		}); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		saved++
		status := strings.TrimSpace(item.Status)
		if status == "" {
			status = "(vazio)"
		}
		operator := strings.TrimSpace(item.Operator)
		if operator == "" {
			operator = "(vazio)"
		}
		savedByStatus[status]++
		savedByOperator[operator]++
		if item.ESim != nil && *item.ESim {
			esimCount++
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"total_stock_items": len(resp.Results),
		"saved":             saved,
		"skipped":           skipped,
		"saved_by_status":   savedByStatus,
		"saved_by_operator": savedByOperator,
		"esim_count":        esimCount,
	})
}

func (s *Server) syncLastRecharges(c *gin.Context) {
	ctx := c.Request.Context()
	items, err := s.store.ListICCIDs(ctx)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	updated := 0
	failed := 0
	failures := []gin.H{}
	rateLimited := false
	for index, item := range items {
		if strings.TrimSpace(item.SimCard) == "" {
			continue
		}
		if index > 0 && s.cfg.ProviderRequestDelay > 0 {
			select {
			case <-ctx.Done():
				c.JSON(http.StatusRequestTimeout, gin.H{"error": ctx.Err().Error()})
				return
			case <-time.After(s.cfg.ProviderRequestDelay):
			}
		}
		resp, _, statusCode, err := s.provider.LastRecharge(ctx, item.SimCard)
		if err != nil {
			failed++
			failures = append(failures, gin.H{"sim_card": item.SimCard, "error": err.Error(), "status_code": statusCode})
			if statusCode == http.StatusTooManyRequests {
				rateLimited = true
				break
			}
			continue
		}
		if !easy2use.StatusCodeTipOK(resp.StatusCodeTip) {
			failed++
			failures = append(failures, gin.H{"sim_card": item.SimCard, "codigo_status_tip": resp.StatusCodeTip})
			continue
		}
		lastRecharge, err := time.ParseInLocation("2006-01-02", resp.LastRecharge, time.Local)
		if err != nil {
			failed++
			failures = append(failures, gin.H{"sim_card": item.SimCard, "error": "invalid ultima_recarga: " + resp.LastRecharge})
			continue
		}
		if err := s.store.UpdateLastRecharge(ctx, item.SimCard, lastRecharge, item.RechargeIntervalMonths, item.SafetyWindowDays); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		updated++
	}

	c.JSON(http.StatusOK, gin.H{
		"checked":      updated + failed,
		"total_iccids": len(items),
		"updated":      updated,
		"failed":       failed,
		"rate_limited": rateLimited,
		"failures":     failures,
	})
}

func (s *Server) listICCIDs(c *gin.Context) {
	items, err := s.store.ListICCIDs(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": items})
}

func (s *Server) iccidSummary(c *gin.Context) {
	items, err := s.store.ICCIDSummary(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": items})
}

type addBalanceRequest struct {
	Quantity int  `json:"quantity"`
	DryRun   bool `json:"dry_run"`
}

func (s *Server) addBalanceManual(c *gin.Context) {
	var req addBalanceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid json body"})
		return
	}
	result, status, err := s.addBalance(c.Request.Context(), c.Param("iccid"), req.Quantity, "manual", req.DryRun)
	if err != nil {
		c.JSON(status, gin.H{"error": err.Error(), "operation": result})
		return
	}
	c.JSON(http.StatusOK, result)
}

type checkRechargesRequest struct {
	DryRun          bool `json:"dry_run"`
	CreateApprovals bool `json:"create_approvals"`
}

func (s *Server) checkRecharges(c *gin.Context) {
	var req checkRechargesRequest
	if c.Request.Body != nil && c.Request.ContentLength != 0 {
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid json body"})
			return
		}
	}

	ctx := c.Request.Context()
	runID, err := s.store.CreateAutomationRun(ctx)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	due, err := s.store.ListDueICCIDs(ctx, time.Now())
	if err != nil {
		_ = s.store.FinishAutomationRun(ctx, runID, "failed", 0, 0, 0, 1, err.Error())
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if req.CreateApprovals {
		created := 0
		existing := 0
		results := []gin.H{}
		for _, item := range due {
			approval, wasCreated, err := s.store.UpsertPendingApproval(ctx, item, "ICCID dentro da janela de recarga preventiva")
			if err != nil {
				_ = s.store.FinishAutomationRun(ctx, runID, "failed", len(due), 0, 0, 1, err.Error())
				c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
				return
			}
			if wasCreated {
				created++
			} else {
				existing++
			}
			results = append(results, gin.H{
				"approval": approval,
				"created":  wasCreated,
			})
		}
		summaryBytes, _ := json.Marshal(results)
		_ = s.store.FinishAutomationRun(ctx, runID, "approval_pending", len(due), 0, existing, 0, string(summaryBytes))
		c.JSON(http.StatusOK, gin.H{
			"run_id":             runID,
			"checked":            len(due),
			"created_approvals":  created,
			"existing_approvals": existing,
			"results":            results,
			"automation_state":   "approval_pending",
		})
		return
	}
	if !req.DryRun && !s.cfg.EnableRealRecharge {
		_ = s.store.FinishAutomationRun(ctx, runID, "blocked", len(due), 0, 0, 0, "real recharge is disabled")
		c.JSON(http.StatusForbidden, gin.H{
			"error":   "real recharge is disabled",
			"hint":    "Use dry_run=true para testar ou configure ENABLE_REAL_RECHARGE=true no .env para permitir recarga real.",
			"checked": len(due),
		})
		return
	}

	recharged := 0
	failed := 0
	skipped := 0
	results := []gin.H{}

	for _, item := range due {
		if req.DryRun {
			skipped++
			results = append(results, gin.H{
				"sim_card":             item.SimCard,
				"cnpj":                 item.CNPJ,
				"subscriber_name":      item.SubscriberName,
				"contract_status":      item.ContractStatus,
				"last_recharge_at":     item.LastRechargeAt,
				"quantity":             item.DefaultQuantity,
				"next_recharge_due_at": item.NextRechargeDueAt,
				"dry_run":              true,
			})
			continue
		}
		result, _, err := s.addBalance(ctx, item.SimCard, item.DefaultQuantity, "automation", false)
		if err != nil {
			failed++
			results = append(results, gin.H{
				"sim_card":        item.SimCard,
				"cnpj":            item.CNPJ,
				"subscriber_name": item.SubscriberName,
				"error":           err.Error(),
				"operation":       result,
			})
			continue
		}
		recharged++
		results = append(results, gin.H{
			"sim_card":        item.SimCard,
			"cnpj":            item.CNPJ,
			"subscriber_name": item.SubscriberName,
			"operation":       result,
		})
	}

	status := "success"
	if failed > 0 && recharged > 0 {
		status = "partial"
	} else if failed > 0 {
		status = "failed"
	}
	summaryBytes, _ := json.Marshal(results)
	_ = s.store.FinishAutomationRun(ctx, runID, status, len(due), recharged, skipped, failed, string(summaryBytes))

	c.JSON(http.StatusOK, gin.H{
		"run_id":           runID,
		"dry_run":          req.DryRun,
		"checked":          len(due),
		"recharged":        recharged,
		"skipped":          skipped,
		"failed":           failed,
		"results":          results,
		"automation_state": status,
	})
}

func (s *Server) nextRun(c *gin.Context) {
	now := time.Now()
	next, actionable, err := s.store.NextRun(c.Request.Context(), now)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	due, err := s.store.ListDueICCIDs(c.Request.Context(), now)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	nextICCIDs := []domain.ICCID{}
	if next != nil {
		nextICCIDs, err = s.store.ListNextRunICCIDs(c.Request.Context(), *next)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
	}
	c.JSON(http.StatusOK, gin.H{
		"today":                   now.Format("2006-01-02"),
		"next_recharge_due_at":    next,
		"iccids_due_count":        len(due),
		"actionable_iccids_count": actionable,
		"next_recharge_iccids":    nextICCIDs,
	})
}

func (s *Server) listApprovals(c *gin.Context) {
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "100"))
	items, err := s.store.ListApprovals(c.Request.Context(), c.Query("status"), limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": items})
}

func (s *Server) approveRecharge(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid approval id"})
		return
	}
	ctx := c.Request.Context()
	approval, err := s.store.GetApproval(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		c.JSON(http.StatusNotFound, gin.H{"error": "approval not found"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if approval.Status != "pending" {
		c.JSON(http.StatusConflict, gin.H{"error": "approval is not pending", "approval": approval})
		return
	}
	if !s.cfg.EnableRealRecharge {
		c.JSON(http.StatusForbidden, gin.H{
			"error":    "real recharge is disabled",
			"hint":     "Configure ENABLE_REAL_RECHARGE=true no .env para aprovar e executar recarga real.",
			"approval": approval,
		})
		return
	}
	if err := s.store.MarkApprovalApproved(ctx, id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if err := s.store.MarkApprovalProcessing(ctx, id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	result, status, err := s.addBalance(ctx, approval.SimCard, approval.Quantity, "approval", false)
	operationID := operationIDFromResult(result)
	if err != nil {
		_ = s.store.FinishApproval(ctx, id, "failed", operationID)
		c.JSON(status, gin.H{"error": err.Error(), "approval_id": id, "operation": result})
		return
	}
	_ = s.store.FinishApproval(ctx, id, "success", operationID)
	c.JSON(http.StatusOK, gin.H{"approval_id": id, "operation": result, "status": "success"})
}

func (s *Server) rejectRecharge(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid approval id"})
		return
	}
	if err := s.store.RejectApproval(c.Request.Context(), id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"approval_id": id, "status": "rejected"})
}

func (s *Server) forceDueDev(c *gin.Context) {
	if !s.cfg.EnableDevRoutes {
		c.JSON(http.StatusNotFound, gin.H{"error": "dev routes are disabled"})
		return
	}
	item, err := s.store.ForceDueToday(c.Request.Context(), strings.TrimSpace(c.Param("iccid")), time.Now())
	if errors.Is(err, sql.ErrNoRows) {
		c.JSON(http.StatusNotFound, gin.H{"error": "iccid not found"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"message": "ICCID marcado como elegivel para recarga hoje apenas no banco local",
		"iccid":   item,
	})
}

type forceStatusRequest struct {
	Status string `json:"status"`
}

func (s *Server) forceStatusDev(c *gin.Context) {
	if !s.cfg.EnableDevRoutes {
		c.JSON(http.StatusNotFound, gin.H{"error": "dev routes are disabled"})
		return
	}
	var req forceStatusRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid json body"})
		return
	}
	status := strings.TrimSpace(req.Status)
	if status == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "status is required"})
		return
	}
	item, err := s.store.ForceContractStatus(c.Request.Context(), strings.TrimSpace(c.Param("iccid")), status)
	if errors.Is(err, sql.ErrNoRows) {
		c.JSON(http.StatusNotFound, gin.H{"error": "iccid not found"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"message": "Status do ICCID alterado apenas no banco local para teste",
		"iccid":   item,
	})
}

func (s *Server) listOperations(c *gin.Context) {
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "100"))
	ops, err := s.store.ListOperations(c.Request.Context(), limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": ops})
}

func (s *Server) addBalance(ctx context.Context, simCard string, quantity int, triggerType string, dryRun bool) (gin.H, int, error) {
	simCard = strings.TrimSpace(simCard)
	if simCard == "" {
		return nil, http.StatusBadRequest, errors.New("iccid is required")
	}
	if quantity < 1 {
		return nil, http.StatusBadRequest, errors.New("quantity must be at least 1")
	}

	item, err := s.store.GetICCID(ctx, simCard)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, http.StatusForbidden, errors.New("iccid not found in local database; run /sync/assinantes first")
	}
	if err != nil {
		return nil, http.StatusInternalServerError, err
	}

	allowed, err := s.store.IsAllowedCNPJ(ctx, item.CNPJ)
	if err != nil {
		return nil, http.StatusInternalServerError, err
	}
	if !allowed {
		return nil, http.StatusForbidden, errors.New("iccid belongs to a non-allowed cnpj")
	}
	if strings.EqualFold(strings.TrimSpace(item.ContractStatus), "CANCELADO") {
		return nil, http.StatusForbidden, errors.New("contract is cancelled")
	}
	if triggerType == "automation" && !item.AutoRechargeEnabled {
		return nil, http.StatusForbidden, errors.New("auto recharge is disabled for this iccid")
	}
	if !dryRun && !s.cfg.EnableRealRecharge {
		return gin.H{
			"sim_card":        item.SimCard,
			"cnpj":            item.CNPJ,
			"subscriber_name": item.SubscriberName,
			"dry_run_hint":    "envie {\"quantity\":1,\"dry_run\":true} para simular sem chamar o provedor",
		}, http.StatusForbidden, errors.New("real recharge is disabled; set ENABLE_REAL_RECHARGE=true to allow provider calls")
	}

	if dryRun {
		return gin.H{
			"dry_run":         true,
			"sim_card":        item.SimCard,
			"cnpj":            item.CNPJ,
			"subscriber_name": item.SubscriberName,
			"contract_status": item.ContractStatus,
			"quantity":        quantity,
			"status":          "dry_run",
			"message":         "Simulacao concluida. Nenhuma chamada foi enviada ao provedor.",
		}, http.StatusOK, nil
	}

	requestPayload := fmt.Sprintf(`{"quantity":%d}`, quantity)
	opID, err := s.store.CreateOperation(ctx, domain.GBOperation{
		SimCard:        item.SimCard,
		CNPJ:           item.CNPJ,
		Quantity:       quantity,
		Status:         "pending",
		TriggerType:    triggerType,
		RequestPayload: requestPayload,
	})
	if err != nil {
		return nil, http.StatusInternalServerError, err
	}

	resp, raw, statusCode, err := s.provider.AddBalance(ctx, item.SimCard, quantity)
	code := statusCode
	responsePayload := string(raw)
	if err != nil {
		_ = s.store.FinishOperation(ctx, opID, "failed", &code, "", responsePayload, err.Error())
		return gin.H{
			"operation_id":            opID,
			"sim_card":                item.SimCard,
			"cnpj":                    item.CNPJ,
			"subscriber_name":         item.SubscriberName,
			"contract_status":         item.ContractStatus,
			"provider_status_code":    statusCode,
			"provider_response_body":  responsePayload,
			"provider_error_message":  err.Error(),
			"provider_request_target": "saldo/adicionar",
		}, http.StatusBadGateway, err
	}
	if !easy2use.StatusCodeTipOK(resp.StatusCodeTip) {
		_ = s.store.FinishOperation(ctx, opID, "failed", &code, resp.UserMessage, responsePayload, "provider returned non-success codigo_status_tip")
		return gin.H{
			"operation_id":           opID,
			"sim_card":               item.SimCard,
			"cnpj":                   item.CNPJ,
			"subscriber_name":        item.SubscriberName,
			"contract_status":        item.ContractStatus,
			"provider_status_code":   statusCode,
			"provider_response":      resp,
			"provider_response_body": responsePayload,
		}, http.StatusBadGateway, errors.New("provider returned non-success codigo_status_tip")
	}

	now := time.Now()
	nextRecharge := domain.ComputeNextRecharge(now, item.RechargeIntervalMonths, item.SafetyWindowDays)
	if err := s.store.UpdateLastRecharge(ctx, item.SimCard, now, item.RechargeIntervalMonths, item.SafetyWindowDays); err != nil {
		_ = s.store.FinishOperation(ctx, opID, "failed", &code, resp.UserMessage, responsePayload, err.Error())
		return gin.H{"operation_id": opID}, http.StatusInternalServerError, err
	}
	if err := s.store.FinishOperation(ctx, opID, "success", &code, resp.UserMessage, responsePayload, ""); err != nil {
		return gin.H{"operation_id": opID}, http.StatusInternalServerError, err
	}

	return gin.H{
		"operation_id":         opID,
		"sim_card":             item.SimCard,
		"cnpj":                 item.CNPJ,
		"subscriber_name":      item.SubscriberName,
		"contract_status":      item.ContractStatus,
		"quantity":             quantity,
		"status":               "success",
		"last_recharge_at":     now,
		"next_recharge_due_at": nextRecharge,
		"provider_response":    resp,
	}, http.StatusOK, nil
}

func operationIDFromResult(result gin.H) *int64 {
	if result == nil {
		return nil
	}
	switch value := result["operation_id"].(type) {
	case int64:
		return &value
	case int:
		id := int64(value)
		return &id
	case float64:
		id := int64(value)
		return &id
	default:
		return nil
	}
}
