package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

var version = "dev"

// TestEvent represents a single event from go test -json output
type TestEvent struct {
	Time    time.Time // Time when the event occurred
	Action  string    // Action: "run", "pause", "cont", "pass", "bench", "fail", "skip", "output"
	Test    string    // Test name
	Package string    // Package being tested
	Output  string    // Output text (for "output" action)
	Elapsed float64   // Elapsed time in seconds for "pass" or "fail" events
}

// TestResult holds the aggregated result for a single test
type TestResult struct {
	Name       string
	Package    string
	Status     string // "PASS", "FAIL", "SKIP"
	Duration   float64
	Output     []string
	ParentTest string // For subtests
	SubTests   []string
	IsSubTest  bool
}

// ReportData contains all data needed for the report
type ReportData struct {
	TotalTests      int
	PassedTests     int
	FailedTests     int
	SkippedTests    int
	TotalDuration   float64
	Results         map[string]*TestResult
	SortedTestNames []string
}

func main() {
	inputFile := flag.String("input", "", "go test -json output file (default is stdin)")
	outputFile := flag.String("output", "test-report.md", "Output markdown file")
	showVersion := flag.Bool("version", false, "Show version information")
	flag.Parse()

	if *showVersion {
		fmt.Printf("gotest-report version %s\n", version)
		os.Exit(0)
	}

	var reader io.Reader = os.Stdin
	if *inputFile != "" {
		file, err := os.Open(*inputFile)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error opening input file: %v\n", err)
			os.Exit(1)
		}
		defer file.Close()
		reader = file
	}

	reportData, err := processTestEvents(reader)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error processing test events: %v\n", err)
		os.Exit(1)
	}

	markdown := generateMarkdownReport(reportData)

	if err := os.WriteFile(*outputFile, []byte(markdown), 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "Error writing report: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Report generated successfully: %s\n", *outputFile)
}

func processTestEvents(reader io.Reader) (*ReportData, error) {
	// Use a Scanner with an increased buffer to safely handle long JSON lines from `go test -json`.
	scanner := bufio.NewScanner(reader)
	// Set the initial and maximum token size to allow large outputs (up to ~10MB per line).
	buf := make([]byte, 0, 1024*1024)
	scanner.Buffer(buf, 10*1024*1024)
	results := make(map[string]*TestResult)
	testOutputMap := make(map[string][]string)

	testStartTime := make(map[string]time.Time)

	for scanner.Scan() {
		line := scanner.Text()
		if strings.TrimSpace(line) == "" {
			// Skip blank lines that can occur in piped or concatenated outputs
			continue
		}
		var event TestEvent
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			return nil, fmt.Errorf("error unmarshalling JSON: %v", err)
		}

		testFullName := event.Test
		if testFullName == "" {
			// Skip package-level events
			continue
		}

		if _, exists := results[testFullName]; !exists && (event.Action == "run" || event.Action == "pass" || event.Action == "fail" || event.Action == "skip") {
			results[testFullName] = &TestResult{
				Name:      testFullName,
				Package:   event.Package,
				Status:    "UNKNOWN",
				Duration:  0,
				Output:    []string{},
				IsSubTest: strings.Contains(testFullName, "/"),
			}

			if results[testFullName].IsSubTest {
				parentName := testFullName[:strings.LastIndex(testFullName, "/")]
				results[testFullName].ParentTest = parentName

				if _, exists := results[parentName]; !exists {
					results[parentName] = &TestResult{
						Name:      parentName,
						Package:   event.Package,
						Status:    "UNKNOWN",
						Duration:  0,
						Output:    []string{},
						SubTests:  []string{},
						IsSubTest: strings.Contains(parentName, "/"),
					}
				}

				results[parentName].SubTests = append(results[parentName].SubTests, testFullName)
			}
		}

		switch event.Action {
		case "run":
			testStartTime[testFullName] = event.Time

		case "pass":
			results[testFullName].Status = "PASS"
			if event.Elapsed > 0 {
				results[testFullName].Duration = event.Elapsed
			} else if !testStartTime[testFullName].IsZero() {
				results[testFullName].Duration = event.Time.Sub(testStartTime[testFullName]).Seconds()
			}

		case "fail":
			results[testFullName].Status = "FAIL"
			if event.Elapsed > 0 {
				results[testFullName].Duration = event.Elapsed
			} else if !testStartTime[testFullName].IsZero() {
				results[testFullName].Duration = event.Time.Sub(testStartTime[testFullName]).Seconds()
			}

		case "skip":
			results[testFullName].Status = "SKIP"

		case "output":
			// Collect test output lines
			if _, exists := testOutputMap[testFullName]; !exists {
				testOutputMap[testFullName] = []string{}
			}
			// Clean output (remove trailing newlines)
			output := strings.TrimSuffix(event.Output, "\n")
			if output != "" {
				testOutputMap[testFullName] = append(testOutputMap[testFullName], output)
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error reading input: %v", err)
	}

	// Add collected output to each test
	for testName, output := range testOutputMap {
		if result, exists := results[testName]; exists {
			result.Output = output
		}
	}

	reportData := &ReportData{
		Results: results,
	}

	var sortedNames []string
	for name, result := range results {
		// Only count root tests in summary (not subtests)
		if !result.IsSubTest {
			sortedNames = append(sortedNames, name)
			reportData.TotalTests++
			reportData.TotalDuration += result.Duration

			switch result.Status {
			case "PASS":
				reportData.PassedTests++
			case "FAIL":
				reportData.FailedTests++
			case "SKIP":
				reportData.SkippedTests++
			}
		}
	}

	sort.Strings(sortedNames)
	reportData.SortedTestNames = sortedNames

	return reportData, nil
}

func generateMarkdownReport(data *ReportData) string {
	var sb strings.Builder

	// Generate header with emoji
	sb.WriteString("# 🧪 Test Summary Report\n\n")

	// Generate summary with emojis
	passPercentage := 0.0
	passPercentageDisplay := "N/A"
	if data.TotalTests > 0 {
		passPercentage = float64(data.PassedTests) / float64(data.TotalTests) * 100
		passPercentageDisplay = fmt.Sprintf("%.1f%%", passPercentage)
	}

	sb.WriteString("## 📊 Summary\n\n")
	sb.WriteString(fmt.Sprintf("- 🧪 **Total Tests:** %d\n", data.TotalTests))
	sb.WriteString(fmt.Sprintf("- ✅ **Passed:** %d (%s)\n", data.PassedTests, passPercentageDisplay))
	sb.WriteString(fmt.Sprintf("- ❌ **Failed:** %d\n", data.FailedTests))
	sb.WriteString(fmt.Sprintf("- ⏭️ **Skipped:** %d\n", data.SkippedTests))
	sb.WriteString(fmt.Sprintf("- ⏱️ **Total Duration:** %.2fs\n\n", data.TotalDuration))

	// Add visual progress bar for pass rate
	if data.TotalTests > 0 {
		sb.WriteString("### Pass Rate Progress\n\n")
		progressBar := generateProgressBar(passPercentage)
		sb.WriteString(fmt.Sprintf("%s **%.1f%%**\n\n", progressBar, passPercentage))
	}

	// Visual pass/fail indicator with emojis
	sb.WriteString("## 🎯 Test Status\n\n")

	// Create status badges with celebration or warning emojis
	if data.FailedTests > 0 {
		sb.WriteString("⚠️ ![Status](https://img.shields.io/badge/Status-FAILED-red) ⚠️\n\n")
		sb.WriteString("> 💔 Some tests failed. Please review the failed tests below.\n\n")
	} else if data.SkippedTests == data.TotalTests {
		sb.WriteString("⏸️ ![Status](https://img.shields.io/badge/Status-SKIPPED-yellow) ⏸️\n\n")
		sb.WriteString("> ⚡ All tests were skipped.\n\n")
	} else {
		sb.WriteString("🎉 ![Status](https://img.shields.io/badge/Status-PASSED-brightgreen) 🎉\n\n")
		sb.WriteString("> ✨ Excellent! All tests passed successfully!\n\n")
	}

	sb.WriteString("---\n\n")

	// Create a table of test results
	sb.WriteString("## 📝 Test Results\n\n")
	sb.WriteString("| Test | Status | Duration | Details |\n")
	sb.WriteString("| ---- | ------ | -------- | ------- |\n")

	// Sort tests by package and name for a more organized report
	for _, testName := range data.SortedTestNames {
		result := data.Results[testName]

		// Skip subtests here - we'll show them nested
		if result.IsSubTest {
			continue
		}

		// Determine status emoji
		statusEmoji := "⏺️"
		switch result.Status {
		case "PASS":
			statusEmoji = "✅"
		case "FAIL":
			statusEmoji = "❌"
		case "SKIP":
			statusEmoji = "⏭️"
		}

		// Format test name to be more readable (remove package prefix if present)
		displayName := result.Name
		if strings.Contains(displayName, "/") && !result.IsSubTest {
			displayName = filepath.Base(displayName)
		}

		// Prepare details column content
		detailsColumn := ""
		if len(result.SubTests) > 0 {
			detailsColumn = fmt.Sprintf("<details><summary>%d subtests</summary>", len(result.SubTests))

			// Add a nested table for subtests
			detailsColumn += "<table><tr><th>Subtest</th><th>Status</th><th>Duration</th></tr>"

			sort.Strings(result.SubTests)
			for _, subTestName := range result.SubTests {
				subTest := data.Results[subTestName]
				subTestDisplayName := subTestName[strings.LastIndex(subTestName, "/")+1:]

				statusEmoji := "⏺️"
				switch subTest.Status {
				case "PASS":
					statusEmoji = "✅"
				case "FAIL":
					statusEmoji = "❌"
				case "SKIP":
					statusEmoji = "⏭️"
				}

				detailsColumn += fmt.Sprintf("<tr><td>%s</td><td>%s %s</td><td>%.3fs</td></tr>",
					subTestDisplayName, statusEmoji, subTest.Status, subTest.Duration)
			}

			detailsColumn += "</table></details>"
		} else {
			detailsColumn = "-"
		}

		sb.WriteString(fmt.Sprintf("| **%s** | %s %s | %.3fs | %s |\n",
			displayName, statusEmoji, result.Status, result.Duration, detailsColumn))
	}
	sb.WriteString("\n")

	if data.FailedTests > 0 {
		sb.WriteString("## 🔴 Failed Tests Details\n\n")
		sb.WriteString("<details>\n")
		sb.WriteString("<summary>💥 Click to expand failed test details</summary>\n\n")
		sb.WriteString("<br>\n\n")

		for _, testName := range data.SortedTestNames {
			result := data.Results[testName]

			// Check if this test or any of its subtests failed
			testFailed := result.Status == "FAIL"

			// Check subtests for failures
			for _, subTestName := range result.SubTests {
				if data.Results[subTestName].Status == "FAIL" {
					testFailed = true
					break
				}
			}

			if testFailed {
				displayName := testName
				if strings.Contains(displayName, "/") && !result.IsSubTest {
					displayName = filepath.Base(displayName)
				}

				sb.WriteString(fmt.Sprintf("### ❌ %s\n\n", displayName))

				// Output for the main test
				if result.Status == "FAIL" && len(result.Output) > 0 {
					formattedOutput := formatFailureOutput(result.Output)
					sb.WriteString(formattedOutput)
				}

				// Output for failed subtests
				for _, subTestName := range result.SubTests {
					subTest := data.Results[subTestName]
					if subTest.Status == "FAIL" {
						subTestDisplayName := subTestName[strings.LastIndex(subTestName, "/")+1:]
						sb.WriteString(fmt.Sprintf("#### ❌ %s\n\n", subTestDisplayName))

						if len(subTest.Output) > 0 {
							formattedOutput := formatFailureOutput(subTest.Output)
							sb.WriteString(formattedOutput)
						}
					}
				}
				sb.WriteString("---\n\n")
			}
		}

		// Close the details tag
		sb.WriteString("</details>\n\n")
	}

	// Add duration metrics
	sb.WriteString("## ⏱️ Test Durations\n\n")
	sb.WriteString("<details>\n")
	sb.WriteString("<summary>⚡ Click to expand test durations</summary>\n\n")
	sb.WriteString("| Test | Duration |\n")
	sb.WriteString("| ---- | -------- |\n")

	// Sort tests by duration (descending)
	type testDuration struct {
		name     string
		duration float64
		isRoot   bool
	}

	var durations []testDuration
	for testName, result := range data.Results {
		durations = append(durations, testDuration{
			name:     testName,
			duration: result.Duration,
			isRoot:   !result.IsSubTest,
		})
	}

	sort.Slice(durations, func(i, j int) bool {
		return durations[i].duration > durations[j].duration
	})

	// Scale factor for bar chart - handle outliers better and empty datasets safely
	maxDuration := 0.0
	if len(durations) > 0 {
		maxDuration = durations[0].duration
		if len(durations) > 1 && maxDuration > durations[1].duration*3 {
			// If top test is 3x longer than second, use second test as scale to prevent skewed visualization
			maxDuration = durations[1].duration * 1.5
		}
		if maxDuration <= 0 {
			// Avoid division by zero; fallback to 1s scale when all durations are zero
			maxDuration = 1.0
		}
	}

	// Take top 15 longest tests (increased from 10)
	count := 0
	for _, d := range durations {
		if count >= 15 {
			break
		}

		// Format test name to be more readable
		displayName := d.name
		if d.isRoot {
			if strings.Contains(displayName, "/") {
				displayName = filepath.Base(displayName)
			}
		} else {
			// For subtests, show parent/child relationship
			displayName = "↳ " + d.name[strings.LastIndex(d.name, "/")+1:]
		}

		// Add bar chart using unicode block characters
		durationBar := ""
		scaleFactor := 25.0
		if maxDuration <= 0 {
			// Nothing to chart
			durationBar = ""
		} else {
			barLength := int(d.duration * scaleFactor / maxDuration)
			if barLength < 1 && d.duration > 0 {
				barLength = 1
			}
			if barLength > 0 {
				durationBar = strings.Repeat("█", barLength)
			}
		}

		sb.WriteString(fmt.Sprintf("| %s | %.3fs %s |\n", displayName, d.duration, durationBar))
		count++
	}

	// Close the details tag
	sb.WriteString("\n</details>\n\n")
	sb.WriteString("---\n\n")
	sb.WriteString(fmt.Sprintf("📅 **Report generated at:** %s\n", time.Now().Format("2006-01-02 15:04:05 MST")))

	return sb.String()
}

// generateProgressBar creates a visual progress bar based on percentage
func generateProgressBar(percentage float64) string {
	barLength := 20
	filled := int(percentage / 5) // 5% per block
	if filled > barLength {
		filled = barLength
	}

	var bar strings.Builder
	bar.WriteString("`[")

	for i := 0; i < barLength; i++ {
		if i < filled {
			bar.WriteString("█")
		} else {
			bar.WriteString("░")
		}
	}

	bar.WriteString("]`")
	return bar.String()
}

// formatFailureOutput formats test failure output with better visualization
func formatFailureOutput(output []string) string {
	var sb strings.Builder
	var errorLines []string
	var hasAssertion bool

	// First pass: collect relevant lines and detect assertions
	for _, line := range output {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}

		// Check for common failure patterns
		if strings.Contains(line, "FAIL") || strings.Contains(line, "Error") ||
			strings.Contains(line, "panic:") || strings.Contains(line, "--- FAIL") ||
			strings.Contains(line, "expected") || strings.Contains(line, "actual") ||
			strings.Contains(line, "got:") || strings.Contains(line, "want:") {
			errorLines = append(errorLines, line)

			// Detect assertion-style failures
			if strings.Contains(strings.ToLower(line), "expected") ||
				strings.Contains(strings.ToLower(line), "got:") ||
				strings.Contains(strings.ToLower(line), "want:") {
				hasAssertion = true
			}
		}
	}

	if len(errorLines) == 0 {
		// If no specific error lines, show all output
		errorLines = output
	}

	// Format the output
	if hasAssertion {
		sb.WriteString("<details>\n")
		sb.WriteString("<summary>🔍 <b>Assertion Failure Details</b></summary>\n\n")
		sb.WriteString("```diff\n")

		for _, line := range errorLines {
			// Highlight expected/actual differences
			if strings.Contains(strings.ToLower(line), "expected") ||
				strings.Contains(strings.ToLower(line), "want") {
				sb.WriteString("- " + line + "\n")
			} else if strings.Contains(strings.ToLower(line), "actual") ||
				strings.Contains(strings.ToLower(line), "got") {
				sb.WriteString("+ " + line + "\n")
			} else if strings.Contains(line, "Error") || strings.Contains(line, "FAIL") {
				sb.WriteString("! " + line + "\n")
			} else {
				sb.WriteString("  " + line + "\n")
			}
		}

		sb.WriteString("```\n")
		sb.WriteString("</details>\n\n")
	} else {
		// Standard error output
		sb.WriteString("<details>\n")
		sb.WriteString("<summary>🐛 <b>Error Details</b></summary>\n\n")
		sb.WriteString("```go\n")

		for _, line := range errorLines {
			sb.WriteString(line + "\n")
		}

		sb.WriteString("```\n")
		sb.WriteString("</details>\n\n")
	}

	return sb.String()
}
