package order

import (
	"slices"

	"gp/internal/shared/db"
	"gp/internal/shared/enum"
)

// Action is a button the screen may show. The server decides the list for each
// viewer; the screen never works it out itself (docs/06 section 6).
//
// These names are not in contracts/enums.json yet, so they are code constants.
type Action string

const (
	ActionCancel               Action = "CANCEL"
	ActionAcceptRevision       Action = "ACCEPT_REVISION"
	ActionApplyCoupon          Action = "APPLY_COUPON"
	ActionRemoveCoupon         Action = "REMOVE_COUPON"
	ActionPay                  Action = "PAY"
	ActionReportProblem        Action = "REPORT_PROBLEM"
	ActionApprove              Action = "APPROVE"
	ActionRevise               Action = "REVISE"
	ActionReject               Action = "REJECT"
	ActionStartPicking         Action = "START_PICKING"
	ActionMarkPacked           Action = "MARK_PACKED"
	ActionAssignDriver         Action = "ASSIGN_DRIVER"
	ActionSendOut              Action = "SEND_OUT"
	ActionRetryDelivery        Action = "RETRY_DELIVERY"
	ActionRequestCancel        Action = "REQUEST_CANCEL"
	ActionRequestPartialRefund Action = "REQUEST_PARTIAL_REFUND"
	ActionComplete             Action = "COMPLETE"
	ActionFail                 Action = "FAIL"
)

// actions lists what this viewer can do with the order now. A move that is in
// the transition table is offered only when this role may make it from this
// status, so the buttons and Transition can never disagree.
func (d orderData) actions(v Viewer, sc db.Scope) []Action {
	can := func(to enum.OrderStatus) bool { return Allowed(d.Status, to, string(sc.Role)) }
	is := func(statuses ...enum.OrderStatus) bool { return slices.Contains(statuses, d.Status) }
	out := []Action{}
	add := func(ok bool, a Action) {
		if ok {
			out = append(out, a)
		}
	}
	switch v {
	case ViewCustomer:
		pay := is(enum.OrderStatusAwaitingPayment)
		add(is(enum.OrderStatusRevised) && can(enum.OrderStatusAwaitingPayment), ActionAcceptRevision)
		add(pay, ActionPay)
		add(pay && d.Coupon == nil, ActionApplyCoupon)
		add(pay && d.Coupon != nil, ActionRemoveCoupon)
		add(can(enum.OrderStatusCancelled), ActionCancel)
		add(d.PaidAt != nil, ActionReportProblem)
	case ViewConsole:
		shop := sc.Role == enum.RoleShopOwner || sc.Role == enum.RoleShopStaff
		add(is(enum.OrderStatusSubmitted) && can(enum.OrderStatusAwaitingPayment), ActionApprove)
		add(can(enum.OrderStatusRevised), ActionRevise)
		add(can(enum.OrderStatusRejected), ActionReject)
		add(can(enum.OrderStatusPicking), ActionStartPicking)
		add(can(enum.OrderStatusPacked), ActionMarkPacked)
		add(is(enum.OrderStatusPaid, enum.OrderStatusPicking, enum.OrderStatusPacked, enum.OrderStatusDeliveryFailed), ActionAssignDriver)
		add(is(enum.OrderStatusPacked) && d.DriverID != nil && can(enum.OrderStatusOutForDelivery), ActionSendOut)
		add(is(enum.OrderStatusDeliveryFailed) && can(enum.OrderStatusOutForDelivery), ActionRetryDelivery)
		add(shop && is(enum.OrderStatusPaid, enum.OrderStatusPicking), ActionRequestCancel)
		add(shop && is(enum.OrderStatusPicking), ActionRequestPartialRefund)
	case ViewDriver:
		mine := d.DriverID != nil && *d.DriverID == sc.UserID
		add(mine && can(enum.OrderStatusDelivered), ActionComplete)
		add(mine && can(enum.OrderStatusDeliveryFailed), ActionFail)
	}
	return out
}
