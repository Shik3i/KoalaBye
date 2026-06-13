package main

import (
	"bytes"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

const templVersion = "v0.3.1020"

func main() {
	check := flag.Bool("check", false, "verify generated templates without writing files")
	outputDir := flag.String("output-dir", "templates", "directory for generated Go files")
	flag.Parse()

	sources, err := filepath.Glob(filepath.Join("templates", "*.templ"))
	if err != nil || len(sources) == 0 {
		fail("find templates: %v", err)
	}
	tempDir, err := os.MkdirTemp("", "koalabye-templ-")
	if err != nil {
		fail("create temporary template directory: %v", err)
	}
	defer os.RemoveAll(tempDir)
	stale := false
	for _, source := range sources {
		targetName := filepath.Base(strings.TrimSuffix(source, ".templ") + "_templ.go")
		target := filepath.Join("templates", targetName)
		sourceContent, err := os.ReadFile(source)
		if err != nil {
			fail("read %s: %v", source, err)
		}
		tempSource := filepath.Join(tempDir, filepath.Base(source))
		if err := os.WriteFile(tempSource, sourceContent, 0o644); err != nil {
			fail("copy %s for deterministic generation: %v", source, err)
		}
		command := exec.Command("go", "run", "github.com/a-h/templ/cmd/templ@"+templVersion, "generate", "-f", tempSource, "-stdout")
		command.Stderr = os.Stderr
		generated, err := command.Output()
		if err != nil {
			fail("generate %s: %v", source, err)
		}
		generated = normalizeLines(generated)
		fileNamePattern := regexp.MustCompile("FileName: `[^`]*" + regexp.QuoteMeta(filepath.Base(source)) + "`")
		generated = fileNamePattern.ReplaceAll(generated, []byte("FileName: `"+filepath.ToSlash(source)+"`"))
		if *check {
			current, err := os.ReadFile(target)
			if err != nil || !bytes.Equal(normalizeLines(current), generated) {
				fmt.Fprintf(os.Stderr, "%s is out of date\n", target)
				stale = true
			}
			continue
		}
		outputPath := filepath.Join(*outputDir, targetName)
		if err := os.MkdirAll(*outputDir, 0o755); err != nil {
			fail("create output directory %s: %v", *outputDir, err)
		}
		if err := os.WriteFile(outputPath, generated, 0o644); err != nil {
			fail("write %s: %v", outputPath, err)
		}
		fmt.Println(outputPath)
	}
	if stale {
		os.Exit(1)
	}
}

func normalizeLines(content []byte) []byte {
	content = bytes.ReplaceAll(content, []byte("\r\n"), []byte("\n"))
	if len(content) > 0 && content[len(content)-1] != '\n' {
		content = append(content, '\n')
	}
	return content
}

func fail(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}
