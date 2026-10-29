package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRealSchemaMatchesEnums(t *testing.T) {
	sql, err := os.ReadFile(filepath.Join(contractsDir, "schema.sql"))
	if err != nil {
		t.Fatal(err)
	}
	if problems := checkSchema(string(sql), realContract(t)); len(problems) > 0 {
		t.Fatalf("schema.sql differs from enums.json:\n%s", strings.Join(problems, "\n"))
	}
}

func checkContract() *Contract {
	return &Contract{Enums: map[string][]string{"UserStatus": {"ACTIVE", "BLOCKED"}, "Role": {"A", "B", "CUSTOMER"}}}
}

func withRules(t *testing.T, rules map[string]listRule) {
	t.Helper()
	old := columnEnums
	columnEnums = rules
	t.Cleanup(func() { columnEnums = old })
}

func TestCheckSchema(t *testing.T) {
	const ok = "CREATE TABLE users (\n status text CHECK (status IN ('ACTIVE',\n 'BLOCKED'))\n);"
	cases := map[string]struct {
		sql   string
		rules map[string]listRule
		want  string // substring of the single expected problem, empty for none
	}{
		"matches, any order":    {ok, map[string]listRule{"users.status": {Enum: "UserStatus"}}, ""},
		"value missing in SQL":  {strings.Replace(ok, ",\n 'BLOCKED'", "", 1), map[string]listRule{"users.status": {Enum: "UserStatus"}}, "expects"},
		"extra value in SQL":    {strings.Replace(ok, "'BLOCKED'", "'BLOCKED','GONE'", 1), map[string]listRule{"users.status": {Enum: "UserStatus"}}, "GONE"},
		"unmapped CHECK list":   {ok, map[string]listRule{}, "no entry in columnEnums"},
		"stale mapping":         {"CREATE TABLE users (id int);", map[string]listRule{"users.status": {Enum: "UserStatus"}}, "no CHECK list found"},
		"comment is ignored":    {"-- CHECK (status IN ('X'))\n" + ok, map[string]listRule{"users.status": {Enum: "UserStatus"}}, ""},
		"without and extra":     {"CREATE TABLE t (r text CHECK (r IN ('A','B','SYSTEM')));", map[string]listRule{"t.r": {Enum: "Role", Without: []string{"CUSTOMER"}, Extra: []string{"SYSTEM"}}}, ""},
		"unknown enum in rules": {ok, map[string]listRule{"users.status": {Enum: "Nope"}}, "does not exist"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			withRules(t, tc.rules)
			problems := checkSchema(tc.sql, checkContract())
			switch {
			case tc.want == "" && len(problems) > 0:
				t.Fatalf("unexpected problems: %v", problems)
			case tc.want != "" && (len(problems) != 1 || !strings.Contains(problems[0], tc.want)):
				t.Fatalf("want one problem containing %q, got %v", tc.want, problems)
			}
		})
	}
}
