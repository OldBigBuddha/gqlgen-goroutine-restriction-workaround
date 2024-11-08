// go:build ignore

package main

import (
	"fmt"
	"os"
	"regexp"

	"github.com/99designs/gqlgen/api"
	"github.com/99designs/gqlgen/codegen/config"
)

func main() {
	cfg, err := config.LoadConfigFromDefaultLocations()
	if err != nil {
		fmt.Fprintln(os.Stderr, "failed to load config", err.Error())
		os.Exit(2)
	}

	if err = api.Generate(cfg); err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(3)
	}

	filename := cfg.Exec.Filename
	if err := replaceGeneratedCodeToRestrictNumberOfGoroutines(filename); err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(4)
	}
}

// The minimum memory size of goroutine is 2KiB
// if 10,000 objects require further execution of the resolver, at least 20 MiB (= 2KiB * 10,000) of memory is needed
// By changing this number, you can limit the number of goroutines **generated at the same time(not running at the same time)**
const MAX_GOROUTINES = 1000

func replaceGeneratedCodeToRestrictNumberOfGoroutines(filename string) error {
	content, err := os.ReadFile(filename)
	if err != nil {
		return err
	}
	text := string(content)

	// prepare regex pattern
	importRegexP := regexp.MustCompile(`\"github\.com\/vektah\/gqlparser\/v2\/ast\"`)
	waitGroupRegexP := regexp.MustCompile(`var wg sync\.WaitGroup`)
	deferRegexP := regexp.MustCompile(`if\s+!\w+\s*\{\s*\n\s*defer\s+wg\.Done\(\)\s*\n\s*\}`)
	goroutineRegexP := regexp.MustCompile(`go f\(i\)`)

	// replaces
	replacedText := importRegexP.ReplaceAllString(text, "\"github.com/vektah/gqlparser/v2/ast\"\n\t\"golang.org/x/sync/semaphore\"")
	replacedText = waitGroupRegexP.ReplaceAllString(replacedText, fmt.Sprintf("var wg sync.WaitGroup\n\tsm := semaphore.NewWeighted(%d)", MAX_GOROUTINES))
	replacedText = deferRegexP.ReplaceAllString(replacedText, "if !isLen1 {\n\t\t\t\tdefer func() {\n\t\t\t\t\tsm.Release(1)\n\t\t\t\t\twg.Done()\n\t\t\t\t}()\n\t\t\t}")
	replacedText = goroutineRegexP.ReplaceAllString(replacedText, "if err := sm.Acquire(ctx, 1); err != nil {\n\t\t\t\tec.Error(ctx, ctx.Err())\n\t\t\t} else {\n\t\t\t\tgo f(i)\n\t\t\t}")

	err = os.WriteFile(filename, []byte(replacedText), 0644)
	if err != nil {
		return err
	}

	return nil
}
