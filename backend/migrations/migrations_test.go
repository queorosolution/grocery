package migrations

import (
	"os"
	"strings"
	"testing"
)

// While the project is pre-release, 0001_init.sql is schema.sql plus goose markers.
// After the first production deploy this test is replaced by the 0002+ rules.
func TestInitMigrationMatchesContract(t *testing.T) {
	schema, err := os.ReadFile("../../contracts/schema.sql")
	if err != nil {
		t.Fatal(err)
	}
	migration, err := os.ReadFile("0001_init.sql")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(migration), string(schema)) {
		t.Fatal("0001_init.sql does not contain contracts/schema.sql verbatim: copy it again (see the header of 0001_init.sql)")
	}
	if !strings.HasPrefix(string(migration), "-- +goose Up") {
		t.Fatal("0001_init.sql must start with the goose Up marker")
	}
}
