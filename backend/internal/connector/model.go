// Package connector links a shop's own Postgres database to its catalog
// (docs/07 section 11, docs/09 section 5). The admin saves a read-only
// connection string, picks a table, maps its columns and previews the result.
// The connection string is write-only: it lives in the secret store and no
// API returns it. All network access to the shop's database goes through the
// source package, which holds the SSRF rules.
package connector

import (
	"time"

	"github.com/google/uuid"

	"gp/internal/shared/enum"
)

// Connector is what the admin sees. HostDisplay is the connection string with
// the password hidden; the real string is only in the secret store.
type Connector struct {
	ID                  uuid.UUID            `json:"id"`
	ShopID              uuid.UUID            `json:"shopId"`
	Kind                enum.ConnectorKind   `json:"kind"`
	HostDisplay         string               `json:"host"`
	SourceTable         *string              `json:"sourceTable"`
	MoneyUnit           enum.MoneyUnit       `json:"moneyUnit"`
	ChangeColumn        *string              `json:"changeColumn"`
	IntervalMin         int                  `json:"intervalMin"`
	AutoCreateProducts  bool                 `json:"autoCreateProducts"`
	Columns             map[string]string    `json:"columns"`
	Status              enum.ConnectorStatus `json:"status"`
	HasTriggerSecret    bool                 `json:"hasTriggerSecret"`
	LastRunAt           *time.Time           `json:"lastRunAt"`
	LastSuccessAt       *time.Time           `json:"lastSuccessAt"`
	ConsecutiveFailures int                  `json:"consecutiveFailures"`
	CreatedAt           time.Time            `json:"createdAt"`
	UpdatedAt           time.Time            `json:"updatedAt"`
}

// SecretInput is the body of create and of "replace the connection string".
type SecretInput struct {
	ConnectionString string `json:"connectionString"`
}

// MappingInput is the body of PUT /connectors/{id}/mapping. It replaces the
// whole mapping. Columns maps our field names (see Fields) to source columns.
type MappingInput struct {
	SourceTable        string            `json:"sourceTable"`
	MoneyUnit          enum.MoneyUnit    `json:"moneyUnit"`
	ChangeColumn       *string           `json:"changeColumn"`
	IntervalMin        int               `json:"intervalMin"`
	AutoCreateProducts bool              `json:"autoCreateProducts"`
	Columns            map[string]string `json:"columns"`
}

// TestResult is the answer to "Test connection".
type TestResult struct {
	OK            bool     `json:"ok"`
	ReadOnly      bool     `json:"readOnly"`
	ServerVersion string   `json:"serverVersion"`
	Warnings      []string `json:"warnings"`
}

// Field is one thing we can read from the source.
type Field struct {
	Key      string
	Required bool
}

// Fields are the mappable fields (docs/09 section 5.4), in the order shown.
var Fields = []Field{
	{"externalId", true}, {"name", true}, {"price", true},
	{"barcode", false}, {"mrp", false}, {"stock", false}, {"available", false}, {"brand", false},
	{"unit", false}, {"weightG", false}, {"category", false}, {"imageUrl", false}, {"updatedAt", false},
}

func isField(key string) bool {
	for _, f := range Fields {
		if f.Key == key {
			return true
		}
	}
	return false
}

const (
	defaultIntervalMin = 60
	minIntervalMin     = 10
	maxIntervalMin     = 1440
	secretPrefix       = "connectors/" // SSM name: connectors/<id> (docs/13 section 2)
	previewRows        = 20
)
