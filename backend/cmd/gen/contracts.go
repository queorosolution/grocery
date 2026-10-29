package main

import (
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strings"
)

// systemActor is the one transition actor that is not a Role: timers and webhooks.
const systemActor = "SYSTEM"

type Contract struct {
	Comment                string                       `json:"$comment"`
	Version                int                          `json:"version"`
	Enums                  map[string][]string          `json:"enums"`
	RolePermissions        map[string][]string          `json:"role_permissions"`
	ConditionalPermissions map[string]map[string]string `json:"conditional_permissions"`
	OrderTransitions       []Transition                 `json:"order_transitions"`
	FinalOrderStatuses     []string                     `json:"final_order_statuses"`
	ErrorCodes             map[string]ErrorCode         `json:"error_codes"`
}

type Transition struct {
	ID     string   `json:"id"`
	From   *string  `json:"from"` // null for the first state (T01)
	To     string   `json:"to"`
	Actors []string `json:"actors"`
	Reason bool     `json:"reason"`
}

type ErrorCode struct {
	HTTP    int    `json:"http"`
	Message string `json:"message"`
}

var valuePattern = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_.]*$`)

// loadContract reads enums.json. Unknown keys are rejected so a typo in the
// contract fails here instead of silently generating nothing.
func loadContract(path string) (*Contract, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	dec := json.NewDecoder(f)
	dec.DisallowUnknownFields()
	var c Contract
	if err := dec.Decode(&c); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	if err := c.validate(); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return &c, nil
}

func (c *Contract) has(enum, value string) bool {
	for _, v := range c.Enums[enum] {
		if v == value {
			return true
		}
	}
	return false
}

func (c *Contract) validate() error {
	var errs []string
	add := func(format string, a ...any) { errs = append(errs, fmt.Sprintf(format, a...)) }

	for _, name := range sortedKeys(c.Enums) {
		seen := map[string]bool{}
		if len(c.Enums[name]) == 0 {
			add("enum %s is empty", name)
		}
		for _, v := range c.Enums[name] {
			if !valuePattern.MatchString(v) {
				add("enum %s: bad value %q", name, v)
			}
			if seen[v] {
				add("enum %s: duplicate value %q", name, v)
			}
			seen[v] = true
		}
	}
	for _, role := range sortedKeys(c.RolePermissions) {
		if !c.has("Role", role) {
			add("role_permissions: unknown role %s", role)
		}
		for _, p := range c.RolePermissions[role] {
			if !c.has("Permission", p) {
				add("role_permissions.%s: unknown permission %s", role, p)
			}
		}
	}
	for _, role := range sortedKeys(c.ConditionalPermissions) {
		for _, p := range sortedKeys(c.ConditionalPermissions[role]) {
			if !c.has("Role", role) || !c.has("Permission", p) {
				add("conditional_permissions: unknown %s / %s", role, p)
			}
		}
	}
	c.validateFlow(add)
	for _, code := range sortedKeys(c.ErrorCodes) {
		e := c.ErrorCodes[code]
		if !valuePattern.MatchString(code) || e.HTTP < 400 || e.HTTP > 599 || e.Message == "" {
			add("error_codes.%s: needs an http status 400-599 and a message", code)
		}
	}
	if _, ok := c.ErrorCodes["INTERNAL"]; !ok {
		add("error_codes: INTERNAL is required")
	}
	if len(errs) > 0 {
		return fmt.Errorf("invalid contract:\n  - %s", strings.Join(errs, "\n  - "))
	}
	return nil
}

func (c *Contract) validateFlow(add func(string, ...any)) {
	final := map[string]bool{}
	for _, s := range c.FinalOrderStatuses {
		if !c.has("OrderStatus", s) {
			add("final_order_statuses: unknown status %s", s)
		}
		final[s] = true
	}
	pairs := map[string]bool{}
	for _, t := range c.OrderTransitions {
		from := ""
		if t.From != nil {
			from = *t.From
			if !c.has("OrderStatus", from) {
				add("transition %s: unknown from %s", t.ID, from)
			}
		}
		if !c.has("OrderStatus", t.To) {
			add("transition %s: unknown to %s", t.ID, t.To)
		}
		if final[from] {
			add("transition %s leaves final status %s", t.ID, from)
		}
		key := from + ">" + t.To
		if pairs[key] {
			add("transition %s: duplicate pair %s", t.ID, key)
		}
		pairs[key] = true
		if t.ID == "" || len(t.Actors) == 0 {
			add("transition %s>%s needs an id and actors", from, t.To)
		}
		for _, a := range t.Actors {
			if a != systemActor && !c.has("Role", a) {
				add("transition %s: unknown actor %s", t.ID, a)
			}
		}
	}
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
