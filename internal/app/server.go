package app

import (
	"context"
	"fmt"
	"io/fs"
	"log/slog"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"mvnodashboard/internal/config"
	"mvnodashboard/internal/domain"
	"mvnodashboard/internal/easy2use"
	webui "mvnodashboard/web"

	"github.com/gin-gonic/gin"
)

type Provider interface {
	ListSubscribers(ctx context.Context) (easy2use.ListSubscribersResponse, []byte, int, error)
	ListStock(ctx context.Context) (easy2use.ListStockResponse, []byte, int, error)
	LastRecharge(ctx context.Context, simCard string) (easy2use.LastRechargeResponse, []byte, int, error)
}

type Server struct {
	cfg      config.Config
	provider Provider
	logger   *slog.Logger

	mu     sync.RWMutex
	iccids map[string]domain.ICCID
	nextID int64
}

func NewServer(cfg config.Config, provider Provider, logger *slog.Logger) *Server {
	return &Server{
		cfg:      cfg,
		provider: provider,
		logger:   logger,
		iccids:   map[string]domain.ICCID{},
	}
}

func (s *Server) Router() http.Handler {
	router := gin.New()
	router.Use(gin.Recovery())
	assets, err := fs.Sub(webui.FS, "assets")
	if err != nil {
		panic(err)
	}
	router.StaticFS("/assets", http.FS(assets))
	router.GET("/", serveEmbeddedFile("relatorios.html"))
	router.GET("/dashboard", serveEmbeddedFile("index.html"))
	router.GET("/relatorios", serveEmbeddedFile("relatorios.html"))
	router.GET("/health", s.health)

	protected := router.Group("/")
	protected.Use(s.adminAuth())
	protected.POST("/sync/assinantes", s.syncSubscribers)
	protected.POST("/sync/estoque", s.syncStock)
	protected.POST("/sync/ultima-recarga", s.syncLastRecharges)
	protected.GET("/iccids", s.listICCIDs)
	protected.GET("/iccids/summary", s.iccidSummary)
	protected.GET("/automation/next-run", s.nextRun)

	protected.POST("/automation/check-recharges", s.checkRechargesReportOnly)
	protected.GET("/recharge-approvals", s.emptyCollection)
	protected.POST("/recharge-approvals/:id/approve", s.reportOnlyAction)
	protected.POST("/recharge-approvals/:id/reject", s.reportOnlyAction)
	protected.GET("/operacoes", s.emptyCollection)
	protected.POST("/iccids/:iccid/saldo", s.reportOnlyAction)
	protected.POST("/dev/iccids/:iccid/force-due", s.reportOnlyAction)
	protected.POST("/dev/iccids/:iccid/force-status", s.reportOnlyAction)

	return router
}

func serveEmbeddedFile(name string) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.FileFromFS(name, http.FS(webui.FS))
	}
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
		"status":  "ok",
		"mode":    "report-only",
		"storage": "memory",
		"time":    time.Now().Format(time.RFC3339),
	})
}

func (s *Server) syncSubscribers(c *gin.Context) {
	resp, raw, statusCode, err := s.provider.ListSubscribers(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusBadGateway, providerError(err, statusCode, raw))
		return
	}
	if !easy2use.StatusCodeTipOK(resp.StatusCodeTip) {
		c.JSON(http.StatusBadGateway, gin.H{"error": "provider returned non-success codigo_status_tip", "codigo_status_tip": resp.StatusCodeTip})
		return
	}
	c.JSON(http.StatusOK, s.mergeSubscribers(resp))
}

func (s *Server) syncStock(c *gin.Context) {
	resp, raw, statusCode, err := s.provider.ListStock(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusBadGateway, providerError(err, statusCode, raw))
		return
	}
	if !easy2use.StatusCodeTipOK(resp.StatusCodeTip) {
		c.JSON(http.StatusBadGateway, gin.H{"error": "provider returned non-success codigo_status_tip", "codigo_status_tip": resp.StatusCodeTip})
		return
	}
	c.JSON(http.StatusOK, s.mergeStock(resp))
}

func (s *Server) syncLastRecharges(c *gin.Context) {
	if err := s.ensureLoaded(c.Request.Context()); err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}

	items := s.snapshot()
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
			case <-c.Request.Context().Done():
				c.JSON(http.StatusRequestTimeout, gin.H{"error": c.Request.Context().Err().Error()})
				return
			case <-time.After(s.cfg.ProviderRequestDelay):
			}
		}
		resp, _, statusCode, err := s.provider.LastRecharge(c.Request.Context(), item.SimCard)
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
		lastRecharge, err := time.ParseInLocation("2006-01-02", strings.TrimSpace(resp.LastRecharge), time.Local)
		if err != nil {
			failed++
			failures = append(failures, gin.H{"sim_card": item.SimCard, "error": "invalid ultima_recarga: " + resp.LastRecharge})
			continue
		}
		s.updateLastRecharge(item.SimCard, lastRecharge)
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
	if err := s.ensureLoaded(c.Request.Context()); err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": s.snapshot()})
}

func (s *Server) iccidSummary(c *gin.Context) {
	if err := s.ensureLoaded(c.Request.Context()); err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}
	type key struct {
		cnpj   string
		status string
	}
	counts := map[key]int{}
	for _, item := range s.snapshot() {
		counts[key{cnpj: item.CNPJ, status: item.ContractStatus}]++
	}
	keys := make([]key, 0, len(counts))
	for k := range counts {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].cnpj == keys[j].cnpj {
			return keys[i].status < keys[j].status
		}
		return keys[i].cnpj < keys[j].cnpj
	})
	items := []gin.H{}
	for _, k := range keys {
		items = append(items, gin.H{
			"cnpj":            k.cnpj,
			"contract_status": k.status,
			"count":           counts[k],
		})
	}
	c.JSON(http.StatusOK, gin.H{"items": items})
}

func (s *Server) nextRun(c *gin.Context) {
	if err := s.ensureLoaded(c.Request.Context()); err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}
	now := startOfDay(time.Now())
	items := s.snapshot()
	dueCount := 0
	actionableCount := 0
	var next *time.Time

	for _, item := range items {
		if !isActionable(item, s.cfg.AllowedCNPJs) || item.NextRechargeDueAt == nil {
			continue
		}
		actionableCount++
		due := startOfDay(*item.NextRechargeDueAt)
		if !due.After(now) {
			dueCount++
			continue
		}
		if next == nil || due.Before(*next) {
			value := due
			next = &value
		}
	}

	nextICCIDs := []domain.ICCID{}
	if next != nil {
		for _, item := range items {
			if item.NextRechargeDueAt == nil || !isActionable(item, s.cfg.AllowedCNPJs) {
				continue
			}
			if startOfDay(*item.NextRechargeDueAt).Equal(*next) {
				nextICCIDs = append(nextICCIDs, item)
			}
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"today":                   now.Format("2006-01-02"),
		"next_recharge_due_at":    next,
		"iccids_due_count":        dueCount,
		"actionable_iccids_count": actionableCount,
		"next_recharge_iccids":    nextICCIDs,
	})
}

func (s *Server) checkRechargesReportOnly(c *gin.Context) {
	if err := s.ensureLoaded(c.Request.Context()); err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}
	now := startOfDay(time.Now())
	results := []gin.H{}
	for _, item := range s.snapshot() {
		if !isActionable(item, s.cfg.AllowedCNPJs) || item.NextRechargeDueAt == nil {
			continue
		}
		if !startOfDay(*item.NextRechargeDueAt).After(now) {
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
		}
	}
	c.JSON(http.StatusOK, gin.H{
		"dry_run":          true,
		"checked":          len(results),
		"recharged":        0,
		"skipped":          len(results),
		"failed":           0,
		"results":          results,
		"automation_state": "report_only",
	})
}

func (s *Server) emptyCollection(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"items": []any{}})
}

func (s *Server) reportOnlyAction(c *gin.Context) {
	c.JSON(http.StatusGone, gin.H{
		"error": "acao desativada: este backend esta em modo relatorio e nao executa recargas, aprovacoes ou alteracoes locais",
	})
}

func (s *Server) ensureLoaded(ctx context.Context) error {
	s.mu.RLock()
	loaded := len(s.iccids) > 0
	s.mu.RUnlock()
	if loaded {
		return nil
	}

	s.logger.Info("memory cache is empty; loading subscribers and stock from provider")
	subscribers, raw, statusCode, err := s.provider.ListSubscribers(ctx)
	if err != nil {
		return fmt.Errorf("listar assinantes: %w (status %d, body %s)", err, statusCode, string(raw))
	}
	if !easy2use.StatusCodeTipOK(subscribers.StatusCodeTip) {
		return fmt.Errorf("listar assinantes retornou codigo_status_tip=%v", subscribers.StatusCodeTip)
	}
	s.mergeSubscribers(subscribers)

	stock, raw, statusCode, err := s.provider.ListStock(ctx)
	if err != nil {
		s.logger.Warn("stock load failed; continuing with subscriber data", "error", err, "status", statusCode, "body", string(raw))
		return nil
	}
	if !easy2use.StatusCodeTipOK(stock.StatusCodeTip) {
		s.logger.Warn("stock load returned non-success codigo_status_tip", "codigo_status_tip", stock.StatusCodeTip)
		return nil
	}
	s.mergeStock(stock)
	return nil
}

func (s *Server) mergeSubscribers(resp easy2use.ListSubscribersResponse) gin.H {
	now := time.Now()
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

	s.mu.Lock()
	defer s.mu.Unlock()

	for _, subscriber := range resp.Results {
		cnpj := config.OnlyDigits(subscriber.Document)
		allowed := cnpjAllowed(cnpj, s.cfg.AllowedCNPJs)
		if allowed {
			allowedSubscribers++
		}
		for _, contract := range subscriber.Contracts {
			totalContracts++
			simCard := strings.TrimSpace(contract.SimCard)
			if simCard == "" {
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
			item := s.iccids[simCard]
			if item.ID == 0 {
				s.nextID++
				item.ID = s.nextID
				item.CreatedAt = now
			}
			item.CNPJ = cnpj
			item.SubscriberName = strings.TrimSpace(subscriber.Name)
			item.SimCard = simCard
			item.PhoneNumber = strings.TrimSpace(contract.PhoneLine)
			item.ContractNumber = strings.TrimSpace(contract.ContractNumber)
			item.ContractStatus = strings.TrimSpace(contract.Status)
			item.PlanName = strings.TrimSpace(contract.Plan)
			item.DefaultQuantity = s.cfg.DefaultRechargeQuantity
			item.RechargeIntervalDays = s.cfg.RechargeIntervalDays
			item.SafetyWindowDays = s.cfg.RechargeSafetyWindowDays
			item.AutoRechargeEnabled = allowed
			item.LastSyncAt = now
			item.UpdatedAt = now
			s.iccids[simCard] = item
			saved++
			savedByCNPJ[cnpj]++
			status := item.ContractStatus
			if status == "" {
				status = "(vazio)"
			}
			savedByStatus[status]++
		}
	}

	return gin.H{
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
		"storage":                   "memory",
	}
}

func (s *Server) mergeStock(resp easy2use.ListStockResponse) gin.H {
	now := time.Now()
	saved := 0
	skipped := 0
	savedByStatus := map[string]int{}
	savedByOperator := map[string]int{}
	esimCount := 0

	s.mu.Lock()
	defer s.mu.Unlock()

	for _, stock := range resp.Results {
		simCard := strings.TrimSpace(stock.SimCard)
		if simCard == "" {
			skipped++
			continue
		}
		item := s.iccids[simCard]
		if item.ID == 0 {
			s.nextID++
			item.ID = s.nextID
			item.SimCard = simCard
			item.DefaultQuantity = s.cfg.DefaultRechargeQuantity
			item.RechargeIntervalDays = s.cfg.RechargeIntervalDays
			item.SafetyWindowDays = s.cfg.RechargeSafetyWindowDays
			item.CreatedAt = now
			item.LastSyncAt = now
		}
		item.StockStatus = strings.TrimSpace(stock.Status)
		item.StockIncludedAt = parseProviderDate(stock.Date)
		item.ESim = stock.ESim
		item.Operator = strings.TrimSpace(stock.Operator)
		item.StockSyncAt = &now
		item.UpdatedAt = now
		s.iccids[simCard] = item
		saved++

		status := item.StockStatus
		if status == "" {
			status = "(vazio)"
		}
		operator := item.Operator
		if operator == "" {
			operator = "(vazio)"
		}
		savedByStatus[status]++
		savedByOperator[operator]++
		if item.ESim != nil && *item.ESim {
			esimCount++
		}
	}

	return gin.H{
		"total_stock_items": len(resp.Results),
		"saved":             saved,
		"skipped":           skipped,
		"saved_by_status":   savedByStatus,
		"saved_by_operator": savedByOperator,
		"esim_count":        esimCount,
		"storage":           "memory",
	}
}

func (s *Server) updateLastRecharge(simCard string, lastRecharge time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()

	item := s.iccids[simCard]
	nextRecharge := domain.ComputeNextRecharge(lastRecharge, item.RechargeIntervalDays, item.SafetyWindowDays)
	item.LastRechargeAt = &lastRecharge
	item.NextRechargeDueAt = &nextRecharge
	item.UpdatedAt = time.Now()
	s.iccids[simCard] = item
}

func (s *Server) snapshot() []domain.ICCID {
	s.mu.RLock()
	defer s.mu.RUnlock()

	items := make([]domain.ICCID, 0, len(s.iccids))
	for _, item := range s.iccids {
		items = append(items, item)
	}
	sort.Slice(items, func(i, j int) bool {
		left := items[i]
		right := items[j]
		if left.NextRechargeDueAt == nil && right.NextRechargeDueAt != nil {
			return true
		}
		if left.NextRechargeDueAt != nil && right.NextRechargeDueAt == nil {
			return false
		}
		if left.NextRechargeDueAt != nil && right.NextRechargeDueAt != nil && !left.NextRechargeDueAt.Equal(*right.NextRechargeDueAt) {
			return left.NextRechargeDueAt.Before(*right.NextRechargeDueAt)
		}
		return left.SimCard < right.SimCard
	})
	return items
}

func providerError(err error, statusCode int, raw []byte) gin.H {
	return gin.H{
		"error":                  err.Error(),
		"status_code":            statusCode,
		"provider_response_body": string(raw),
		"hint":                   "Confira EASY2USE_BASE_URL e EASY2USE_USER_TOKEN no .env",
	}
}

func cnpjAllowed(cnpj string, allowed []string) bool {
	for _, value := range allowed {
		if cnpj == value {
			return true
		}
	}
	return false
}

func isActionable(item domain.ICCID, allowedCNPJs []string) bool {
	return item.AutoRechargeEnabled &&
		cnpjAllowed(item.CNPJ, allowedCNPJs) &&
		strings.EqualFold(strings.TrimSpace(item.ContractStatus), "EM USO")
}

func parseProviderDate(value string) *time.Time {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	t, err := time.ParseInLocation("2006-01-02", value, time.Local)
	if err != nil {
		return nil
	}
	return &t
}

func startOfDay(t time.Time) time.Time {
	year, month, day := t.Date()
	return time.Date(year, month, day, 0, 0, 0, 0, t.Location())
}
