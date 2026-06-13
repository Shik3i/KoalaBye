package main

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

type check struct {
	name string
	cmd  string
	args []string
}

func main() {
	checks := []check{
		{name: "tests", cmd: "go", args: []string{"test", "./..."}},
		{name: "vet", cmd: "go", args: []string{"vet", "./..."}},
		{name: "whitespace", cmd: "git", args: []string{"diff", "--check"}},
	}

	fmt.Println("==> templ generated")
	if err := checkTemplGenerated(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	fmt.Println("==> sqlc generated")
	if err := checkSqlcGenerated(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	fmt.Println("==> Go formatting")
	if err := checkGoFormatting(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	for _, item := range checks {
		fmt.Printf("==> %s\n", item.name)
		command := exec.Command(item.cmd, item.args...)
		command.Stdout = os.Stdout
		command.Stderr = os.Stderr
		if err := command.Run(); err != nil {
			fmt.Fprintf(os.Stderr, "%s failed: %v\n", item.name, err)
			os.Exit(1)
		}
	}

	fmt.Println("==> govulncheck")
	if _, err := exec.LookPath("govulncheck"); err != nil {
		fmt.Println("govulncheck not found. Skipping vulnerability scan.")
		fmt.Println("Please install it with: go install golang.org/x/vuln/cmd/govulncheck@latest")
	} else {
		cmd := exec.Command("govulncheck", "./...")
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			fmt.Fprintf(os.Stderr, "govulncheck failed: %v\n", err)
			os.Exit(1)
		}
	}
}

func checkGoFormatting() error {
	files := goFiles()
	if len(files) == 0 {
		return fmt.Errorf("no Go files found")
	}
	for _, path := range files {
		content, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("read %s: %w", path, err)
		}
		command := exec.Command("gofmt")
		command.Stdin = bytes.NewReader(content)
		formatted, err := command.Output()
		if err != nil {
			return fmt.Errorf("format %s: %w", path, err)
		}
		normalized := bytes.ReplaceAll(content, []byte("\r\n"), []byte("\n"))
		if !bytes.Equal(normalized, formatted) {
			return fmt.Errorf("Go file needs formatting: %s", path)
		}
	}
	return nil
}

func goFiles() []string {
	var files []string
	for _, root := range []string{"cmd", "internal", "migrations", "templates", filepath.Join("web", "static")} {
		_ = filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
			if err != nil || info.IsDir() || filepath.Ext(path) != ".go" {
				return nil
			}
			files = append(files, path)
			return nil
		})
	}
	return files
}

func checkTemplGenerated() error {
	command := exec.Command("go", "run", "./cmd/templgenerate", "-check")
	command.Stdout = os.Stdout
	command.Stderr = os.Stderr
	if err := command.Run(); err != nil {
		return fmt.Errorf(`templ generated files are out of date; run "go run ./cmd/templgenerate": %w`, err)
	}
	return nil
}

func checkSqlcGenerated() error {
	tempDir, err := os.MkdirTemp("", "koalabye-sqlc-")
	if err != nil {
		return fmt.Errorf("create sqlc temporary directory: %w", err)
	}
	defer os.RemoveAll(tempDir)
	workDir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("get working directory: %w", err)
	}
	configPath := filepath.Join(tempDir, "sqlc.yaml")
	outputDir := filepath.Join(tempDir, "dbgen")
	config := fmt.Sprintf(`version: "2"
sql:
  - engine: "sqlite"
    schema: %q
    queries: %q
    gen:
      go:
        package: "dbgen"
        out: %q
        emit_json_tags: true
        emit_empty_slices: true
`, relativePath(tempDir, filepath.Join(workDir, "migrations")), relativePath(tempDir, filepath.Join(workDir, "queries")), "dbgen")
	if err := os.WriteFile(configPath, []byte(config), 0o644); err != nil {
		return fmt.Errorf("write temporary sqlc config: %w", err)
	}
	command := exec.Command("go", "run", "github.com/sqlc-dev/sqlc/cmd/sqlc@v1.29.0", "generate", "-f", configPath)
	command.Stdout = os.Stdout
	command.Stderr = os.Stderr
	if err := command.Run(); err != nil {
		return fmt.Errorf("sqlc generate failed: %w", err)
	}
	committed, err := filepath.Glob(filepath.Join("internal", "db", "dbgen", "*.go"))
	if err != nil {
		return fmt.Errorf("find committed sqlc files: %w", err)
	}
	generated, err := filepath.Glob(filepath.Join(outputDir, "*.go"))
	if err != nil || len(generated) != len(committed) {
		return fmt.Errorf("sqlc generated file set differs from committed output")
	}
	for _, currentPath := range committed {
		generatedPath := filepath.Join(outputDir, filepath.Base(currentPath))
		current, err := os.ReadFile(currentPath)
		if err != nil {
			return fmt.Errorf("read %s: %w", currentPath, err)
		}
		fresh, err := os.ReadFile(generatedPath)
		if err != nil || !bytes.Equal(normalizeLines(current), normalizeLines(fresh)) {
			return fmt.Errorf(`%s is out of date; run "go run github.com/sqlc-dev/sqlc/cmd/sqlc@v1.29.0 generate"`, currentPath)
		}
	}
	return nil
}

func normalizeLines(content []byte) []byte {
	return bytes.ReplaceAll(content, []byte("\r\n"), []byte("\n"))
}

func relativePath(base, target string) string {
	value, err := filepath.Rel(base, target)
	if err != nil {
		return filepath.ToSlash(target)
	}
	return filepath.ToSlash(value)
}
