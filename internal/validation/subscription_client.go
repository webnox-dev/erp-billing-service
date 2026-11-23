package validation

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"gorm.io/gorm"
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
func (c *SubscriptionClient) ValidateRestriction(orgID, restrictionKey string) (bool, string, error) {
	allowed, msg, _, _, err := c.ValidateRestrictionDetailed(orgID, restrictionKey)
	return allowed, msg, err
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
func (c *SubscriptionClient) ValidateRestrictionDetailed(orgID, restrictionKey string) (bool, string, int, int, error) {
	if c.db == nil {
		return true, "no db connection — allowed (fail-open)", -1, 0, nil
	}

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

	// 2. Collect non-default (purchased) and default plan_ids
	var nonDefaultPlanIDs []string
	var allPlanIDs []string

	if isExpired {
		allPlanIDs = appendUniquePlanID(allPlanIDs, c.findDefaultPlanIDForKey(restrictionKey))
	} else {
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
		allPlanIDs = appendUniquePlanID(allPlanIDs, c.findDefaultPlanIDForKey(restrictionKey))
	}

	// 3. Query restriction value: prefer explicit non-default purchased plans first;
	// fallback to all active plan IDs, then to the default plan defining this restriction.
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
		defaultPlanID := c.findDefaultPlanIDForKey(restrictionKey)
		limitValue, found = lookupRestriction([]string{defaultPlanID})
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
		switch restrictionKey {
		case RestrictionKeyMaxSalesOrders:
			msg = fmt.Sprintf("Sales order creation limit reached (%d/%d) under your purchased IMS plan. Please upgrade your subscription to create more sales orders.", currentValue, limitValue)
		case RestrictionKeyMaxSalesReturns:
			msg = fmt.Sprintf("Sales return creation limit reached (%d/%d) under your purchased IMS plan. Please upgrade your subscription to create more sales returns.", currentValue, limitValue)
		default:
			msg = fmt.Sprintf("(%d/%d). Please upgrade your subscription.", currentValue, limitValue)
		}
	}

	return allowed, msg, limitValue, currentValue, nil
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

// StartSync is retained for backward compatibility but is now a no-op.
// Billing subscription data is synced via Kafka (subscription_consumer.go) instead of direct DB polling.
func StartSync(ctx context.Context, localDB *gorm.DB, sourceDSN string) {
	log.Println("[BillingSubscriptionSync] Direct DB sync is deprecated. Using Kafka-based sync instead.")
}


