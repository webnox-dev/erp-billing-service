package validation

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// RestrictionKeys for use across billing service
const (
	RestrictionKeyMaxInvoices       = "max_invoices"
	RestrictionKeyMaxSalesOrders    = "max_sales_orders"
	RestrictionKeyMaxSalesReturns   = "max_sales_returns"
	RestrictionKeyMaxBills          = "max_bills"
	RestrictionKeyMaxVendorCredits = "max_vendor_credits"
	RestrictionKeyMaxPayments       = "max_payments"
)

// imsRestrictionKeys is the set of restriction keys that belong to IMS/Inventory module.
var imsRestrictionKeys = map[string]bool{
	RestrictionKeyMaxInvoices:       true,
	RestrictionKeyMaxSalesOrders:    true,
	RestrictionKeyMaxSalesReturns:   true,
	RestrictionKeyMaxBills:          true,
	RestrictionKeyMaxVendorCredits:  true,
	RestrictionKeyMaxPayments:       true,
}

// SubscriptionClient is a local validation client that queries local *_readonly replica tables
type SubscriptionClient struct {
	db *gorm.DB
}

var globalDB *gorm.DB

// InitDB initializes the global GORM db for subscription validations
func InitDB(db *gorm.DB) {
	globalDB = db
}

// NewSubscriptionClient creates a new SubscriptionClient
func NewSubscriptionClient() *SubscriptionClient {
	return &SubscriptionClient{
		db: globalDB,
	}
}

// ValidateRestriction checks if the org can create a new resource locally
func (c *SubscriptionClient) ValidateRestriction(orgID, restrictionKey string, optionalModules ...string) (bool, string, error) {
	allowed, msg, _, _, err := c.ValidateRestrictionDetailed(orgID, restrictionKey, optionalModules...)
	return allowed, msg, err
}

func normalizeBillingModule(optionalModules []string) string {
	for _, m := range optionalModules {
		switch strings.ToLower(strings.TrimSpace(m)) {
		case "crm":
			return "crm"
		case "ims", "inventory":
			return "ims"
		case "efs", "fsm", "billing":
			return "efs"
		}
	}
	return ""
}

func formatBillingModuleLabel(moduleSlug string) string {
	switch strings.ToLower(strings.TrimSpace(moduleSlug)) {
	case "crm":
		return "CRM"
	case "ims":
		return "IMS"
	case "efs":
		return "EFS"
	default:
		return ""
	}
}

func resolveDefaultBillingPlanIDs(db *gorm.DB, restrictionKey, targetModule string) []string {
	var planIDs []string
	discriminator := "max_vendors"
	if targetModule == "crm" {
		discriminator = "max_leads"
	} else if targetModule == "efs" {
		discriminator = "max_work_orders"
	}
	db.Table("plans_readonly AS p").
		Joins("JOIN plan_restrictions_readonly AS pr ON pr.plan_id = p.id AND pr.deleted_at IS NULL").
		Where("p.is_default = TRUE AND p.is_active = TRUE AND pr.restriction_key = ?", discriminator).
		Order("p.updated_at DESC").
		Pluck("p.id", &planIDs)
	if len(planIDs) == 0 {
		db.Table("plans_readonly AS p").
			Joins("JOIN plan_restrictions_readonly AS pr ON pr.plan_id = p.id AND pr.deleted_at IS NULL").
			Where("p.is_default = TRUE AND p.is_active = TRUE AND pr.restriction_key = ?", restrictionKey).
			Order("pr.restriction_value ASC").
			Pluck("p.id", &planIDs)
	}
	return planIDs
}

func resolveActiveBillingPlanIDs(db *gorm.DB, subID, subPlanID, restrictionKey, targetModule string) ([]string, string) {
	if targetModule != "" {
		var modItem struct{ PlanID string }
		if errMod := db.Table("organization_subscription_items_readonly").
			Where("subscription_id = ? AND LOWER(module_slug) = ?", subID, targetModule).
			Select("plan_id").
			Scan(&modItem).Error; errMod == nil && modItem.PlanID != "" {
			return []string{modItem.PlanID}, targetModule
		}

		// Multi-module subscription without an item for targetModule must use that module's default plan
		// instead of inheriting another module's plan limits.
		var itemCount int64
		db.Table("organization_subscription_items_readonly").
			Where("subscription_id = ?", subID).
			Count(&itemCount)
		if itemCount > 0 {
			return resolveDefaultBillingPlanIDs(db, restrictionKey, targetModule), targetModule
		}
		if subPlanID != "" {
			return []string{subPlanID}, targetModule
		}
		return nil, targetModule
	}

	effectiveModule := ""
	var planIDs []string
	if imsRestrictionKeys[restrictionKey] {
		var imsItem struct{ PlanID string }
		if errIMS := db.Table("organization_subscription_items_readonly").
			Where("subscription_id = ? AND LOWER(module_slug) = ?", subID, "ims").
			Select("plan_id").
			Scan(&imsItem).Error; errIMS == nil && imsItem.PlanID != "" {
			planIDs = append(planIDs, imsItem.PlanID)
			effectiveModule = "ims"
		}
	}

	if len(planIDs) == 0 {
		if subPlanID != "" {
			planIDs = append(planIDs, subPlanID)
		}
		var itemPlanIDs []string
		db.Table("organization_subscription_items_readonly").
			Where("subscription_id = ?", subID).
			Pluck("plan_id", &itemPlanIDs)

		for _, pid := range itemPlanIDs {
			if pid != "" {
				planIDs = append(planIDs, pid)
			}
		}
	}

	return planIDs, effectiveModule
}

func (c *SubscriptionClient) findDefaultPlanIDForKey(restrictionKey string) string {
	var defaultPlan struct {
		ID string
	}
	if err := c.db.Table("plans_readonly p").
		Joins("JOIN plan_restrictions_readonly pr ON pr.plan_id = p.id AND pr.deleted_at IS NULL").
		Where("p.is_default = TRUE AND p.is_active = TRUE AND pr.restriction_key = ?", restrictionKey).
		Order("CASE WHEN pr.restriction_value = -1 THEN 999999 ELSE pr.restriction_value END DESC").
		Select("p.id").
		Limit(1).
		Scan(&defaultPlan).Error; err == nil && defaultPlan.ID != "" {
		return defaultPlan.ID
	}
	if err := c.db.Table("plans_readonly").
		Where("is_default = TRUE AND is_active = TRUE").
		Select("id").
		Limit(1).
		Scan(&defaultPlan).Error; err == nil && defaultPlan.ID != "" {
		return defaultPlan.ID
	}
	return "10000000-0000-0000-0000-000000000001"
}

type billingPlanItemCandidate struct {
	PlanID    string    `gorm:"column:plan_id"`
	ValidDays int       `gorm:"column:valid_days"`
	Price     float64   `gorm:"column:price"`
	IsDefault bool      `gorm:"column:is_default"`
	CreatedAt time.Time `gorm:"column:created_at"`
}

func appendUniquePlanID(list []string, pid string) []string {
	if pid == "" {
		return list
	}
	for _, existing := range list {
		if existing == pid {
			return list
		}
	}
	return append(list, pid)
}

func isSubscriptionExpired(subID, status string, expiryDate, now time.Time) bool {
	if subID == "" {
		return true
	}
	statusLower := strings.ToLower(status)
	if statusLower == "expired" || statusLower == "cancelled" {
		return true
	}
	if (statusLower == "active" || statusLower == "trial" || statusLower == "paid") && !expiryDate.IsZero() && now.After(expiryDate) {
		return true
	}
	return false
}

func partitionActivePlanItems(items []billingPlanItemCandidate, now time.Time) ([]string, []string) {
	var nonDefaultPlanIDs []string
	var allPlanIDs []string
	for _, item := range items {
		if item.PlanID == "" {
			continue
		}
		// Default fallback plans never expire. Temporary non-default trials expire after valid_days.
		if !item.IsDefault && item.Price == 0 && item.ValidDays > 0 {
			if now.After(item.CreatedAt.AddDate(0, 0, item.ValidDays)) {
				continue
			}
		}
		allPlanIDs = appendUniquePlanID(allPlanIDs, item.PlanID)
		if !item.IsDefault {
			nonDefaultPlanIDs = appendUniquePlanID(nonDefaultPlanIDs, item.PlanID)
		}
	}
	return nonDefaultPlanIDs, allPlanIDs
}

func pickHighestRestrictionLimit(values []int) (int, bool) {
	if len(values) == 0 {
		return 0, false
	}
	best := values[0]
	for _, v := range values[1:] {
		if v == -1 {
			return -1, true
		}
		if best != -1 && v > best {
			best = v
		}
	}
	return best, true
}

// ValidateRestrictionDetailed checks restriction with full details
// Returns (allowed, message, limit, currentValue, error)
func (c *SubscriptionClient) ValidateRestrictionDetailed(orgID, restrictionKey string, optionalModules ...string) (bool, string, int, int, error) {
	if c.db == nil {
		return true, "no db connection — allowed (fail-open)", -1, 0, nil
	}

	targetModule := normalizeBillingModule(optionalModules)

	// 1. Get active subscription from organization_subscriptions_readonly
	var sub struct {
		ID         string
		PlanID     string
		Status     string
		ExpiryDate time.Time
	}
	err := c.db.Table("organization_subscriptions_readonly").
		Where("organization_id = ? AND LOWER(status) IN ?", orgID, []string{"active", "trial", "paid", "unpaid"}).
		Order("created_at DESC").
		Limit(1).
		Scan(&sub).Error

	now := time.Now().UTC()
	isExpired := err != nil || isSubscriptionExpired(sub.ID, sub.Status, sub.ExpiryDate, now)

	// 2. Collect non-default (purchased) and default plan_ids with module isolation when specified or inferred
	var nonDefaultPlanIDs []string
	var allPlanIDs []string
	effectiveModule := targetModule

	if isExpired {
		if targetModule != "" {
			for _, pid := range resolveDefaultBillingPlanIDs(c.db, restrictionKey, targetModule) {
				allPlanIDs = appendUniquePlanID(allPlanIDs, pid)
			}
		}
		if len(allPlanIDs) == 0 {
			allPlanIDs = appendUniquePlanID(allPlanIDs, c.findDefaultPlanIDForKey(restrictionKey))
		}
	} else if targetModule != "" {
		var validItems []billingPlanItemCandidate
		c.db.Table("organization_subscription_items_readonly").
			Select("organization_subscription_items_readonly.plan_id, COALESCE(plans_readonly.valid_days, 0) AS valid_days, COALESCE(plans_readonly.price, 0) AS price, COALESCE(plans_readonly.is_default, FALSE) AS is_default, organization_subscription_items_readonly.created_at").
			Joins("LEFT JOIN plans_readonly ON organization_subscription_items_readonly.plan_id = plans_readonly.id").
			Where("organization_subscription_items_readonly.subscription_id = ? AND LOWER(organization_subscription_items_readonly.module_slug) = ?", sub.ID, targetModule).
			Scan(&validItems)

		nonDefaultPlanIDs, allPlanIDs = partitionActivePlanItems(validItems, now)

		if len(allPlanIDs) == 0 {
			var itemCount int64
			c.db.Table("organization_subscription_items_readonly").
				Where("subscription_id = ?", sub.ID).
				Count(&itemCount)
			if itemCount > 0 {
				for _, pid := range resolveDefaultBillingPlanIDs(c.db, restrictionKey, targetModule) {
					allPlanIDs = appendUniquePlanID(allPlanIDs, pid)
				}
			} else if sub.PlanID != "" {
				var subPlanDefault bool
				_ = c.db.Table("plans_readonly").Where("id = ?", sub.PlanID).Select("is_default").Scan(&subPlanDefault).Error
				allPlanIDs = appendUniquePlanID(allPlanIDs, sub.PlanID)
				if !subPlanDefault {
					nonDefaultPlanIDs = appendUniquePlanID(nonDefaultPlanIDs, sub.PlanID)
				}
			}
		}
	} else {
		if imsRestrictionKeys[restrictionKey] {
			var imsItem struct{ PlanID string }
			if errIMS := c.db.Table("organization_subscription_items_readonly").
				Where("subscription_id = ? AND LOWER(module_slug) = ?", sub.ID, "ims").
				Select("plan_id").
				Limit(1).
				Scan(&imsItem).Error; errIMS == nil && imsItem.PlanID != "" {
				effectiveModule = "ims"
			}
		}

		var validItems []billingPlanItemCandidate
		c.db.Table("organization_subscription_items_readonly").
			Select("organization_subscription_items_readonly.plan_id, COALESCE(plans_readonly.valid_days, 0) AS valid_days, COALESCE(plans_readonly.price, 0) AS price, COALESCE(plans_readonly.is_default, FALSE) AS is_default, organization_subscription_items_readonly.created_at").
			Joins("LEFT JOIN plans_readonly ON organization_subscription_items_readonly.plan_id = plans_readonly.id").
			Where("organization_subscription_items_readonly.subscription_id = ?", sub.ID).
			Scan(&validItems)

		nonDefaultPlanIDs, allPlanIDs = partitionActivePlanItems(validItems, now)

		if sub.PlanID != "" {
			var subPlanDefault bool
			_ = c.db.Table("plans_readonly").Where("id = ?", sub.PlanID).Select("is_default").Scan(&subPlanDefault).Error
			allPlanIDs = appendUniquePlanID(allPlanIDs, sub.PlanID)
			if !subPlanDefault {
				nonDefaultPlanIDs = appendUniquePlanID(nonDefaultPlanIDs, sub.PlanID)
			}
		}
	}

	if len(allPlanIDs) == 0 {
		if targetModule != "" {
			for _, pid := range resolveDefaultBillingPlanIDs(c.db, restrictionKey, targetModule) {
				allPlanIDs = appendUniquePlanID(allPlanIDs, pid)
			}
		}
		if len(allPlanIDs) == 0 {
			allPlanIDs = appendUniquePlanID(allPlanIDs, c.findDefaultPlanIDForKey(restrictionKey))
		}
	}

	// 3. Query restriction value: prefer explicit non-default purchased plans first;
	// fallback to all active plan IDs, then to the module/key default plan defining this restriction.
	lookupRestriction := func(candidatePlanIDs []string) (int, bool) {
		if len(candidatePlanIDs) == 0 {
			return 0, false
		}
		var cnt int64
		c.db.Table("plan_restrictions_readonly").
			Where("plan_id IN ? AND restriction_key = ? AND deleted_at IS NULL", candidatePlanIDs, restrictionKey).
			Count(&cnt)
		if cnt == 0 {
			return 0, false
		}
		var restriction struct {
			RestrictionValue int
		}
		if err := c.db.Table("plan_restrictions_readonly").
			Where("plan_id IN ? AND restriction_key = ? AND deleted_at IS NULL", candidatePlanIDs, restrictionKey).
			Order("CASE WHEN restriction_value = -1 THEN 999999 ELSE restriction_value END DESC").
			Limit(1).
			Scan(&restriction).Error; err != nil {
			return 0, false
		}
		return restriction.RestrictionValue, true
	}

	limitValue, found := lookupRestriction(nonDefaultPlanIDs)
	if !found {
		limitValue, found = lookupRestriction(allPlanIDs)
	}
	if !found {
		var defaultCandidates []string
		if targetModule != "" {
			defaultCandidates = resolveDefaultBillingPlanIDs(c.db, restrictionKey, targetModule)
		}
		if len(defaultCandidates) == 0 {
			defaultCandidates = []string{c.findDefaultPlanIDForKey(restrictionKey)}
		}
		limitValue, found = lookupRestriction(defaultCandidates)
	}
	if !found {
		return true, "No limit specified", -1, 0, nil
	}

	if strings.HasPrefix(restrictionKey, "allow_") {
		allowed := limitValue == 1
		msg := "Feature allowed"
		if !allowed {
			msg = "Feature disabled under your current plan"
		}
		return allowed, msg, limitValue, 0, nil
	}

	if limitValue == -1 {
		return true, "Unlimited usage allowed", -1, 0, nil
	}

	// 4. Calculate dynamic local usage count
	currentValue := 0
	switch restrictionKey {
	case RestrictionKeyMaxInvoices:
		var cnt int64
		c.db.Table("invoices").Where("organization_id = ? AND deleted_at IS NULL", orgID).Count(&cnt)
		currentValue = int(cnt)

	case RestrictionKeyMaxSalesOrders:
		var cnt int64
		c.db.Table("sales_orders").Where("organization_id = ? AND deleted_at IS NULL", orgID).Count(&cnt)
		currentValue = int(cnt)

	case RestrictionKeyMaxSalesReturns:
		var cnt int64
		c.db.Table("sales_returns").Where("organization_id = ? AND deleted_at IS NULL", orgID).Count(&cnt)
		currentValue = int(cnt)
	}

	allowed := currentValue < limitValue
	msg := fmt.Sprintf("Usage: %d / %d", currentValue, limitValue)
	if !allowed {
		msg = formatBillingRestrictionMessage(restrictionKey, effectiveModule, currentValue, limitValue)
	}

	return allowed, msg, limitValue, currentValue, nil
}

func formatBillingRestrictionMessage(restrictionKey, effectiveModule string, currentValue, limitValue int) string {
	moduleLabel := formatBillingModuleLabel(effectiveModule)
	planPhrase := "your purchased plan"
	if moduleLabel != "" {
		planPhrase = fmt.Sprintf("your purchased %s plan", moduleLabel)
	}
	switch restrictionKey {
	case RestrictionKeyMaxSalesOrders:
		return fmt.Sprintf("Sales order creation limit reached (%d/%d) under %s. Please upgrade your subscription to create more sales orders.", currentValue, limitValue, planPhrase)
	case RestrictionKeyMaxSalesReturns:
		return fmt.Sprintf("Sales return creation limit reached (%d/%d) under %s. Please upgrade your subscription to create more sales returns.", currentValue, limitValue, planPhrase)
	default:
		return fmt.Sprintf("(%d/%d). Please upgrade your subscription.", currentValue, limitValue)
	}
}

// IncrementUsage is a local no-op since usage is calculated dynamically via DB COUNT
func (c *SubscriptionClient) IncrementUsage(orgID, restrictionKey string) error {
	return nil
}

// DecrementUsage is a local no-op since usage is calculated dynamically via DB COUNT
func (c *SubscriptionClient) DecrementUsage(orgID, restrictionKey string) error {
	return nil
}

// =========================================================================
// Real-time Database Replication Worker
// =========================================================================

type rawPlan struct {
	ID          string    `gorm:"column:id"`
	Name        string    `gorm:"column:name"`
	Description string    `gorm:"column:description"`
	Price       float64   `gorm:"column:price"`
	ValidDays   int       `gorm:"column:valid_days"`
	Badge       string    `gorm:"column:badge"`
	Status      string    `gorm:"column:status"`
	IsPerUser   bool      `gorm:"column:is_per_user"`
	IsActive    bool      `gorm:"column:is_active"`
	SortOrder   int       `gorm:"column:sort_order"`
	IsDefault   bool      `gorm:"column:is_default"`
	CreatedAt   time.Time `gorm:"column:created_at"`
	UpdatedAt   time.Time `gorm:"column:updated_at"`
}

type rawRestriction struct {
	ID               string     `gorm:"column:id"`
	PlanID           string     `gorm:"column:plan_id"`
	RestrictionKey   string     `gorm:"column:restriction_key"`
	RestrictionValue int        `gorm:"column:restriction_value"`
	Description      string     `gorm:"column:description"`
	CreatedAt        time.Time  `gorm:"column:created_at"`
	UpdatedAt        time.Time  `gorm:"column:updated_at"`
	DeletedAt        *time.Time `gorm:"column:deleted_at"`
}

type rawSubscription struct {
	ID                     string     `gorm:"column:id"`
	OrganizationID         string     `gorm:"column:organization_id"`
	PlanID                 string     `gorm:"column:plan_id"`
	Status                 string     `gorm:"column:status"`
	StartDate              time.Time  `gorm:"column:start_date"`
	ExpiryDate             time.Time  `gorm:"column:expiry_date"`
	SelectedModuleIDs      string     `gorm:"column:selected_module_ids"`
	TotalPrice             float64    `gorm:"column:total_price"`
	UserCount              int        `gorm:"column:user_count"`
	Notes                  string     `gorm:"column:notes"`
	CancelledAt            *time.Time `gorm:"column:cancelled_at"`
	TrialEndsAt            *time.Time `gorm:"column:trial_ends_at"`
	BillingPeriod          string     `gorm:"column:billing_period"`
	RazorpaySubscriptionID string     `gorm:"column:razorpay_subscription_id"`
	PaymentMethodID        *string    `gorm:"column:payment_method_id"`
	CreatedAt              time.Time  `gorm:"column:created_at"`
	UpdatedAt              time.Time  `gorm:"column:updated_at"`
}

type rawSubscriptionItem struct {
	ID             string    `gorm:"column:id"`
	SubscriptionID string    `gorm:"column:subscription_id"`
	ModuleSlug     string    `gorm:"column:module_slug"`
	PlanID         string    `gorm:"column:plan_id"`
	Quantity       int       `gorm:"column:quantity"`
	CreatedAt      time.Time `gorm:"column:created_at"`
	UpdatedAt      time.Time `gorm:"column:updated_at"`
}

// StartSync starts the background DB replication worker alongside Kafka sync so missed events never leave stale restrictions.
func StartSync(ctx context.Context, localDB *gorm.DB, sourceDSN string) {
	if sourceDSN == "" {
		sourceDSN = os.Getenv("ORGANIZATION_DB_URL")
	}
	if sourceDSN == "" {
		sourceDSN = os.Getenv("ORGANIZATION_DATABASE_URL")
	}
	if sourceDSN == "" {
		sourceDSN = os.Getenv("ORG_DB_URL")
	}
	if sourceDSN == "" {
		sourceDSN = "postgresql://efsorgdbdev:efsorgdbdev@123@192.168.0.26:5455/efsorgdbdev?sslmode=disable"
	}

	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()

		log.Println("[BillingSubscriptionSync] Initial replication started...")
		syncTables(localDB, sourceDSN)

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				syncTables(localDB, sourceDSN)
			}
		}
	}()
}

func syncTables(localDB *gorm.DB, sourceDSN string) {
	if localDB == nil {
		return
	}
	srcDB, err := gorm.Open(postgres.Open(sourceDSN), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		log.Printf("[BillingSubscriptionSync] Warning: Failed to connect to source organization-service DB: %v", err)
		return
	}
	sqlDB, _ := srcDB.DB()
	defer sqlDB.Close()

	localDB = localDB.Session(&gorm.Session{
		Logger: localDB.Logger.LogMode(logger.Silent),
	})

	var srcPlans []rawPlan
	if err := srcDB.Table("plans").Find(&srcPlans).Error; err == nil {
		for _, p := range srcPlans {
			localDB.Exec(`
				INSERT INTO plans_readonly (id, name, description, price, valid_days, badge, status, is_per_user, is_active, sort_order, is_default, created_at, updated_at)
				VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
				ON CONFLICT (id) DO UPDATE SET
					name = EXCLUDED.name, description = EXCLUDED.description, price = EXCLUDED.price,
					valid_days = EXCLUDED.valid_days, badge = EXCLUDED.badge, status = EXCLUDED.status,
					is_per_user = EXCLUDED.is_per_user, is_active = EXCLUDED.is_active,
					sort_order = EXCLUDED.sort_order, is_default = EXCLUDED.is_default, created_at = EXCLUDED.created_at, updated_at = EXCLUDED.updated_at
			`, p.ID, p.Name, p.Description, p.Price, p.ValidDays, p.Badge, p.Status, p.IsPerUser, p.IsActive, p.SortOrder, p.IsDefault, p.CreatedAt, p.UpdatedAt)
		}
	}

	var srcRest []rawRestriction
	if err := srcDB.Table("plan_restrictions").Find(&srcRest).Error; err == nil {
		for _, r := range srcRest {
			localDB.Exec(`
				INSERT INTO plan_restrictions_readonly (id, plan_id, restriction_key, restriction_value, description, created_at, updated_at, deleted_at)
				VALUES (?, ?, ?, ?, ?, ?, ?, ?)
				ON CONFLICT (id) DO UPDATE SET
					plan_id = EXCLUDED.plan_id, restriction_key = EXCLUDED.restriction_key,
					restriction_value = EXCLUDED.restriction_value, description = EXCLUDED.description,
					created_at = EXCLUDED.created_at, updated_at = EXCLUDED.updated_at, deleted_at = EXCLUDED.deleted_at
			`, r.ID, r.PlanID, r.RestrictionKey, r.RestrictionValue, r.Description, r.CreatedAt, r.UpdatedAt, r.DeletedAt)
		}
	}

	var srcSub []rawSubscription
	if err := srcDB.Table("organization_subscriptions").Find(&srcSub).Error; err == nil {
		activeIDs := make([]string, 0)
		for _, s := range srcSub {
			st := strings.ToLower(strings.TrimSpace(s.Status))
			if st == "active" || st == "trial" || st == "paid" || st == "unpaid" {
				activeIDs = append(activeIDs, s.ID)
			}
			localDB.Exec(`
				INSERT INTO organization_subscriptions_readonly (id, organization_id, plan_id, status, start_date, expiry_date, selected_module_ids, total_price, user_count, notes, cancelled_at, trial_ends_at, billing_period, razorpay_subscription_id, payment_method_id, created_at, updated_at)
				VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
				ON CONFLICT (id) DO UPDATE SET
					organization_id = EXCLUDED.organization_id, plan_id = EXCLUDED.plan_id, status = EXCLUDED.status,
					start_date = EXCLUDED.start_date, expiry_date = EXCLUDED.expiry_date, selected_module_ids = EXCLUDED.selected_module_ids,
					total_price = EXCLUDED.total_price, user_count = EXCLUDED.user_count, notes = EXCLUDED.notes,
					cancelled_at = EXCLUDED.cancelled_at, trial_ends_at = EXCLUDED.trial_ends_at, billing_period = EXCLUDED.billing_period,
					razorpay_subscription_id = EXCLUDED.razorpay_subscription_id, payment_method_id = EXCLUDED.payment_method_id,
					created_at = EXCLUDED.created_at, updated_at = EXCLUDED.updated_at
			`, s.ID, s.OrganizationID, s.PlanID, s.Status, s.StartDate, s.ExpiryDate, s.SelectedModuleIDs, s.TotalPrice, s.UserCount, s.Notes, s.CancelledAt, s.TrialEndsAt, s.BillingPeriod, s.RazorpaySubscriptionID, s.PaymentMethodID, s.CreatedAt, s.UpdatedAt)
		}
		if len(activeIDs) > 0 {
			localDB.Exec(`UPDATE organization_subscriptions_readonly SET status = 'expired' WHERE LOWER(status) IN ('active','trial','paid','unpaid') AND id NOT IN ?`, activeIDs)
		}
	}

	var srcItems []rawSubscriptionItem
	if err := srcDB.Table("organization_subscription_items").Find(&srcItems).Error; err == nil {
		for _, item := range srcItems {
			localDB.Exec(`
				INSERT INTO organization_subscription_items_readonly (id, subscription_id, module_slug, plan_id, quantity, created_at, updated_at)
				VALUES (?, ?, ?, ?, ?, ?, ?)
				ON CONFLICT (id) DO UPDATE SET
					subscription_id = EXCLUDED.subscription_id, module_slug = EXCLUDED.module_slug,
					plan_id = EXCLUDED.plan_id, quantity = EXCLUDED.quantity,
					created_at = EXCLUDED.created_at, updated_at = EXCLUDED.updated_at
			`, item.ID, item.SubscriptionID, item.ModuleSlug, item.PlanID, item.Quantity, item.CreatedAt, item.UpdatedAt)
		}
	}
}


