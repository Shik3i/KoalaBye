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
	out, err := exec.Command("git", "status", "--porcelain", "templates").Output()
	if err != nil {
		return fmt.Errorf("git status failed: %w", err)
	}
	if len(bytes.TrimSpace(out)) > 0 {
		return fmt.Errorf("templates/ has uncommitted changes. Please commit or stash them before running devcheck")
	}

	// Try running via docker if available to bypass Windows file locking
	var cmd *exec.Cmd
	if _, err := exec.LookPath("docker"); err == nil {
		cmd = exec.Command("docker", "run", "--rm", "-v", fmt.Sprintf("%s:/src", getwd()), "-w", "/src", "docker.io/library/golang:1.26.4-alpine", "sh", "-c", "go run github.com/a-h/templ/cmd/templ@v0.3.960 generate")
	} else {
		cmd = exec.Command("go", "run", "github.com/a-h/templ/cmd/templ@v0.3.960", "generate")
	}
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("templ generate failed: %w", err)
	}

	out, err = exec.Command("git", "status", "--porcelain", "templates").Output()
	if err != nil {
		return fmt.Errorf("git status failed: %w", err)
	}
	if len(bytes.TrimSpace(out)) > 0 {
		exec.Command("git", "checkout", "--", "templates").Run()
		return fmt.Errorf("templ generated files are out of date! Please run \"go run github.com/a-h/templ/cmd/templ@v0.3.960 generate\" and commit the changes")
	}
	return nil
}

func checkSqlcGenerated() error {
	out, err := exec.Command("git", "status", "--porcelain", "internal/db/dbgen").Output()
	if err != nil {
		return fmt.Errorf("git status failed: %w", err)
	}
	if len(bytes.TrimSpace(out)) > 0 {
		return fmt.Errorf("internal/db/dbgen/ has uncommitted changes. Please commit or stash them before running devcheck")
	}

	var cmd *exec.Cmd
	if _, err := exec.LookPath("docker"); err == nil {
		cmd = exec.Command("docker", "run", "--rm", "-v", fmt.Sprintf("%s:/src", getwd()), "-w", "/src", "docker.io/library/golang:1.26.4-alpine", "sh", "-c", "go run github.com/sqlc-dev/sqlc/cmd/sqlc@v1.29.0 generate")
	} else {
		cmd = exec.Command("go", "run", "github.com/sqlc-dev/sqlc/cmd/sqlc@v1.29.0", "generate")
	}
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("sqlc generate failed: %w", err)
	}

	out, err = exec.Command("git", "status", "--porcelain", "internal/db/dbgen").Output()
	if err != nil {
		return fmt.Errorf("git status failed: %w", err)
	}
	if len(bytes.TrimSpace(out)) > 0 {
		exec.Command("git", "checkout", "--", "internal/db/dbgen").Run()
		return fmt.Errorf("sqlc generated files are out of date! Please run \"go run github.com/sqlc-dev/sqlc/cmd/sqlc@v1.29.0 generate\" and commit the changes")
	}
	return nil
}

func getwd() string {
	d, _ := os.Getwd()
	return d
}
