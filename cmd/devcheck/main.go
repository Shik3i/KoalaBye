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
