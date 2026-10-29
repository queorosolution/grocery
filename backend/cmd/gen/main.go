// Command gen turns contracts/enums.json into Go and TypeScript code, and with
// -check verifies that contracts/schema.sql uses the same values.
package main

import (
	"bytes"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// generate builds every output file. Paths are relative to backend/ or web/.
func generate(c *Contract) (backend, web []output, err error) {
	for _, emit := range []func(*Contract) (output, error){emitEnums, emitPermissions, emitFlow, emitErrorCodes} {
		o, err := emit(c)
		if err != nil {
			return nil, nil, err
		}
		backend = append(backend, o)
	}
	web = []output{emitTSEnums(c), emitTSPerms(c), emitTSErrors(c)}
	return backend, web, nil
}

func writeAll(root string, files []output) error {
	for _, f := range files {
		path := filepath.Join(root, f.Path)
		if old, err := os.ReadFile(path); err == nil && bytes.Equal(old, f.Data) {
			continue
		}
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(path, f.Data, 0o644); err != nil {
			return err
		}
		fmt.Println("wrote", path)
	}
	return nil
}

func run() error {
	contracts := flag.String("contracts", "../contracts", "folder with enums.json and schema.sql")
	backend := flag.String("backend", ".", "backend root")
	web := flag.String("web", "../web", "web root")
	check := flag.Bool("check", false, "check schema.sql against enums.json instead of generating")
	flag.Parse()

	c, err := loadContract(filepath.Join(*contracts, "enums.json"))
	if err != nil {
		return err
	}
	if *check {
		sql, err := os.ReadFile(filepath.Join(*contracts, "schema.sql"))
		if err != nil {
			return err
		}
		if problems := checkSchema(string(sql), c); len(problems) > 0 {
			return fmt.Errorf("schema.sql and enums.json differ:\n  - %s", strings.Join(problems, "\n  - "))
		}
		fmt.Println("schema.sql matches enums.json")
		return nil
	}
	be, w, err := generate(c)
	if err != nil {
		return err
	}
	if err := writeAll(*backend, be); err != nil {
		return err
	}
	return writeAll(*web, w)
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "gen:", err)
		os.Exit(1)
	}
}
