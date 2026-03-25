// internal/actions/scriptdetect.go
package actions

import (
	"regexp"
	"strings"
)

// ScriptDownload represents a detected download-and-execute pattern.
type ScriptDownload struct {
	URL     string // the URL being downloaded
	Pattern string // e.g., "curl|bash", "wget|sh"
	Line    string // the matched line
}

// urlRegex matches http(s) URLs.
var urlRegex = regexp.MustCompile(`https?://\S+`)

// pipePatterRegex matches: (curl|wget) [flags] URL | (bash|sh|zsh|python|python3)
// The pipe executor must appear after the URL fetcher on the same line.
var pipePattern = regexp.MustCompile(
	`(curl|wget)\s+\S.*\|\s*(bash|sh|zsh|python3?)`,
)

// downloadThenExecPattern detects multi-line patterns:
//   curl -o <file> <URL>
//   sh/bash/python <file>
//
// We track lines individually and look for the combination.
var downloadLinePattern = regexp.MustCompile(
	`(curl|wget)\s+.*-o\s+(\S+)`,
)

var execLinePattern = regexp.MustCompile(
	`\b(sh|bash|zsh|python3?)\s+(\S+)`,
)

// DetectScriptDownloads scans a run: block for dangerous download-and-execute patterns.
func DetectScriptDownloads(script string) []ScriptDownload {
	var results []ScriptDownload

	lines := strings.Split(script, "\n")

	// Track files downloaded via -o for multi-line detection.
	// Maps filename -> (downloader, URL, line text)
	type downloadEntry struct {
		tool string
		url  string
		line string
	}
	downloadedFiles := map[string]downloadEntry{}

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		// Pattern 1: pipe to executor on same line
		if m := pipePattern.FindStringSubmatch(trimmed); m != nil {
			tool := m[1]
			executor := m[2]
			url := extractURL(trimmed)
			results = append(results, ScriptDownload{
				URL:     url,
				Pattern: tool + "|" + executor,
				Line:    trimmed,
			})
			continue
		}

		// Pattern 2a: track "curl -o <file> <url>" downloads
		if m := downloadLinePattern.FindStringSubmatch(trimmed); m != nil {
			tool := m[1]
			filename := m[2]
			url := extractURL(trimmed)
			downloadedFiles[filename] = downloadEntry{tool: tool, url: url, line: trimmed}
			continue
		}

		// Pattern 2b: detect "sh/bash <file>" where file was previously downloaded
		if m := execLinePattern.FindStringSubmatch(trimmed); m != nil {
			executor := m[1]
			filename := m[2]
			if entry, ok := downloadedFiles[filename]; ok {
				results = append(results, ScriptDownload{
					URL:     entry.url,
					Pattern: entry.tool + "-o|" + executor,
					Line:    entry.line + "\n" + trimmed,
				})
				// Remove to avoid double-counting
				delete(downloadedFiles, filename)
			}
		}
	}

	return results
}

// extractURL returns the first http(s) URL found in a string, or empty string.
func extractURL(s string) string {
	if m := urlRegex.FindString(s); m != "" {
		// Strip trailing punctuation that may not be part of the URL
		return strings.TrimRight(m, ".,;\"')")
	}
	return ""
}
