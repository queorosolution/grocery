package main

import (
	"fmt"
	"regexp"
	"slices"
	"strings"
)

// listRule says which enum a CHECK (col IN (...)) list must match. Without and
// Extra cover the few columns that hold a slice of an enum or one extra value.
type listRule struct {
	Enum    string
	Without []string
	Extra   []string
}

// columnEnums maps "table.column" to its enum. Every simple CHECK (col IN (...))
// in schema.sql must appear here, so a new constraint cannot skip the check.
var columnEnums = map[string]listRule{
	"users.status":                   {Enum: "UserStatus"},
	"shops.status":                   {Enum: "ShopStatus"},
	"memberships.scope":              {Enum: "MembershipScope"},
	"memberships.role":               {Enum: "Role", Without: []string{"CUSTOMER"}},
	"memberships.status":             {Enum: "MembershipStatus"},
	"invites.role":                   {Enum: "Role", Without: []string{"SUPER_ADMIN", "CUSTOMER"}},
	"otp_codes.purpose":              {Enum: "OtpPurpose"},
	"payment_accounts.owner_type":    {Enum: "PaymentAccountOwner"},
	"payment_accounts.provider":      {Enum: "PaymentProvider"},
	"payment_accounts.mode":          {Enum: "PaymentMode"},
	"payment_accounts.status":        {Enum: "PaymentAccountStatus"},
	"shop_settings.delivery_type":    {Enum: "DeliveryType"},
	"shop_settings.payment_provider": {Enum: "PaymentProvider"},
	"shop_settings.driver_source":    {Enum: "DriverSource"},
	"slots.status":                   {Enum: "SlotStatus"},
	"products.unit":                  {Enum: "Unit"},
	"products.status":                {Enum: "ProductStatus"},
	"shop_products.managed_by":       {Enum: "ManagedBy"},
	"price_changes.source":           {Enum: "PriceChangeSource"},
	"connectors.kind":                {Enum: "ConnectorKind"},
	"connectors.money_unit":          {Enum: "MoneyUnit"},
	"connectors.status":              {Enum: "ConnectorStatus"},
	"connector_runs.trigger":         {Enum: "RunTrigger"},
	"connector_runs.status":          {Enum: "RunStatus"},
	"sync_issues.kind":               {Enum: "SyncIssueKind"},
	"sync_issues.status":             {Enum: "SyncIssueStatus"},
	"coupons.type":                   {Enum: "CouponType"},
	"coupons.funded_by":              {Enum: "FundedBy"},
	"coupons.status":                 {Enum: "CouponStatus"},
	"orders.status":                  {Enum: "OrderStatus"},
	"orders.delivery_type":           {Enum: "DeliveryType"},
	"orders.discount_funded_by":      {Enum: "FundedBy"},
	"orders.payment_provider":        {Enum: "PaymentProvider"},
	"order_items.unit":               {Enum: "Unit"},
	"order_items.sub_pref":           {Enum: "SubstitutionPref"},
	"order_items.state":              {Enum: "OrderItemState"},
	"order_events.actor_role":        {Enum: "Role", Extra: []string{systemActor}},
	"payments.provider":              {Enum: "PaymentProvider"},
	"payments.status":                {Enum: "PaymentStatus"},
	"refunds.type":                   {Enum: "RefundType"},
	"refunds.status":                 {Enum: "RefundStatus"},
	"refunds.method":                 {Enum: "RefundMethod"},
	"support_tickets.reason":         {Enum: "TicketReason"},
	"support_tickets.status":         {Enum: "TicketStatus"},
	"payouts.status":                 {Enum: "PayoutStatus"},
	"ledger_entries.type":            {Enum: "LedgerType"},
	"ledger_entries.party":           {Enum: "LedgerParty"},
	"notifications.channel":          {Enum: "NotifyChannel"},
	"notifications.status":           {Enum: "NotificationStatus"},
}

var (
	sqlComment  = regexp.MustCompile(`--[^\n]*`)
	createTable = regexp.MustCompile(`CREATE TABLE(?: IF NOT EXISTS)?\s+(\w+)`)
	checkInList = regexp.MustCompile(`CHECK\s*\(\s*(\w+)\s+IN\s*\(([^)]*)\)\s*\)`)
	sqlLiteral  = regexp.MustCompile(`'([^']*)'`)
)

// checkSchema compares every simple CHECK (col IN (...)) list in the SQL with
// the enum it is mapped to. It returns one message per problem.
func checkSchema(sql string, c *Contract) []string {
	sql = sqlComment.ReplaceAllString(sql, "")
	tables := createTable.FindAllStringSubmatchIndex(sql, -1)
	var problems []string
	found := map[string]bool{}

	for _, loc := range checkInList.FindAllStringSubmatchIndex(sql, -1) {
		table := tableBefore(sql, tables, loc[0])
		key := table + "." + sql[loc[2]:loc[3]]
		found[key] = true
		rule, ok := columnEnums[key]
		if !ok {
			problems = append(problems, fmt.Sprintf("%s: CHECK list has no entry in columnEnums (cmd/gen/check.go)", key))
			continue
		}
		var got []string
		for _, l := range sqlLiteral.FindAllStringSubmatch(sql[loc[4]:loc[5]], -1) {
			got = append(got, l[1])
		}
		if msg := compareList(c, rule, got); msg != "" {
			problems = append(problems, fmt.Sprintf("%s: %s", key, msg))
		}
	}
	for _, key := range sortedKeys(columnEnums) {
		if !found[key] {
			problems = append(problems, fmt.Sprintf("%s: mapped in columnEnums but no CHECK list found in the schema", key))
		}
	}
	return problems
}

func tableBefore(sql string, tables [][]int, pos int) string {
	name := ""
	for _, t := range tables {
		if t[0] < pos {
			name = sql[t[2]:t[3]]
		}
	}
	return name
}

func compareList(c *Contract, rule listRule, got []string) string {
	if _, ok := c.Enums[rule.Enum]; !ok {
		return fmt.Sprintf("enum %s does not exist in enums.json", rule.Enum)
	}
	var want []string
	for _, v := range c.Enums[rule.Enum] {
		if !slices.Contains(rule.Without, v) {
			want = append(want, v)
		}
	}
	want = append(want, rule.Extra...)
	slices.Sort(want)
	slices.Sort(got)
	if slices.Equal(want, got) {
		return ""
	}
	return fmt.Sprintf("SQL has [%s] but enum %s expects [%s]", strings.Join(got, " "), rule.Enum, strings.Join(want, " "))
}
