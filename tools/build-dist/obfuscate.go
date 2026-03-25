// obfuscate merges all pkg/core/*.go files into a single c0.go
// with private identifiers renamed to short opaque names.
//
// Usage: go run obfuscate.go <core-dir> <output-file>
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// nameMap maps private identifiers to obfuscated names.
// Public identifiers (exported) keep their original names.
var nameMap = map[string]string{
	// Constants
	"hbInterval": "_p2",

	// Package-level vars
	"encodedURL":    "_k0",
	"xorSeed":       "_k1",
	"httpTransport": "_h0",
	"runtimeSalt":   "_s0",
	"globalDB":      "_db0",

	// Private functions
	"resolveEndpoint":      "_d0",
	"signPayload":          "_sg",
	"postSigned":           "_ps",
	"getUnsigned":          "_gu",
	"postUnsigned":         "_pu",
	"readErrorBody":        "_re",
	"activateInstance":      "_ai",
	"sendHeartbeat":        "_hb",
	"sendDeactivate":       "_sd",
	"loadRuntimeData":      "_lrd",
	"saveRuntimeData":      "_srd",
	"removeRuntimeData":    "_rrd",
	"loadOrCreateInstanceID": "_lid",
	"generateHardwareID":   "_ghi",
	"getPrimaryMAC":        "_gpm",
	"newUUID":              "_uuid",
	"resolveDataPath":      "_rdp",
	"hexEnc":               "_he",
	"getConfig":            "_gc",
	"setConfig":            "_sc",
	"deleteConfig":         "_dc",
	"resolveAPIKey":        "_rk",
	"completeActivation":   "_ca",

	// RuntimeContext private fields
	"apiKey":       "_a0",
	"globalApiKey": "_a9",
	"instanceID":   "_a1",
	"active":       "_a2",
	"ctxHash":      "_a3",
	"regURL":       "_a5",
	"regToken":     "_a6",
	"tier":         "_a7",
	"version":      "_a8",

	// RuntimeData struct
	"RuntimeData": "_rtd",

	// RuntimeConfig constants
	"ConfigKeyInstanceID": "_ck0",
	"ConfigKeyAPIKey":     "_ck1",
	"ConfigKeyTier":       "_ck2",
	"ConfigKeyCustomerID": "_ck3",
}

// Files to merge in order
var coreFiles = []string{
	"endpoint.go",
	"transport.go",
	"model.go",
	"store.go",
	"integrity.go",
	"runtime.go",
}

func main() {
	if len(os.Args) < 3 {
		fmt.Fprintf(os.Stderr, "Usage: %s <core-dir> <output-file>\n", os.Args[0])
		os.Exit(1)
	}

	coreDir := os.Args[1]
	outFile := os.Args[2]

	var imports []string
	var bodies []string
	importSet := make(map[string]bool)
	importRe := regexp.MustCompile(`"([^"]+)"`)

	for _, f := range coreFiles {
		data, err := os.ReadFile(filepath.Join(coreDir, f))
		if err != nil {
			fmt.Fprintf(os.Stderr, "  skip %s: %v\n", f, err)
			continue
		}

		lines := strings.Split(string(data), "\n")
		inImport := false
		bodyStart := 0

		for i, line := range lines {
			trimmed := strings.TrimSpace(line)

			if trimmed == "package core" {
				continue
			}

			if strings.HasPrefix(trimmed, "import (") {
				inImport = true
				continue
			}

			if inImport {
				if trimmed == ")" {
					inImport = false
					bodyStart = i + 1
					continue
				}
				if trimmed != "" && !strings.HasPrefix(trimmed, "//") {
					if !importSet[trimmed] {
						importSet[trimmed] = true
						imports = append(imports, "\t"+trimmed)
					}
				}
				continue
			}

			if strings.HasPrefix(trimmed, "import \"") {
				matches := importRe.FindStringSubmatch(trimmed)
				if len(matches) > 1 {
					imp := "\"" + matches[1] + "\""
					if !importSet[imp] {
						importSet[imp] = true
						imports = append(imports, "\t"+imp)
					}
				}
				bodyStart = i + 1
				continue
			}

			if bodyStart == 0 && trimmed != "" && !strings.HasPrefix(trimmed, "//") {
				bodyStart = i
			}
		}

		if bodyStart > 0 && bodyStart < len(lines) {
			body := strings.TrimSpace(strings.Join(lines[bodyStart:], "\n"))
			if body != "" {
				bodies = append(bodies, body)
			}
		}
	}

	sort.Strings(imports)

	// Merge all bodies
	merged := strings.Join(bodies, "\n\n")

	// Apply obfuscation — longest names first to avoid partial matches
	keys := make([]string, 0, len(nameMap))
	for k := range nameMap {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool { return len(keys[i]) > len(keys[j]) })

	for _, k := range keys {
		re := regexp.MustCompile(`\b` + regexp.QuoteMeta(k) + `\b`)
		merged = re.ReplaceAllString(merged, nameMap[k])
	}

	// Collapse excessive blank lines
	for strings.Contains(merged, "\n\n\n") {
		merged = strings.ReplaceAll(merged, "\n\n\n", "\n\n")
	}

	// Remove standalone comment lines (keeps inline comments)
	commentRe := regexp.MustCompile(`(?m)^[ \t]*//.*\n`)
	merged = commentRe.ReplaceAllString(merged, "")

	// Build output
	var out strings.Builder
	out.WriteString("package core\n\nimport (\n")
	for _, imp := range imports {
		out.WriteString(imp + "\n")
	}
	out.WriteString(")\n\n")
	out.WriteString(merged)
	out.WriteString("\n")

	if err := os.WriteFile(outFile, []byte(out.String()), 0644); err != nil {
		fmt.Fprintf(os.Stderr, "  ERROR writing %s: %v\n", outFile, err)
		os.Exit(1)
	}

	fmt.Printf("  ✓ Generated %s (%d bytes)\n", outFile, out.Len())
}
