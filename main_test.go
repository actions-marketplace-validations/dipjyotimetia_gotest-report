package main

import (
	"strings"
	"testing"
)

func TestProcessTestEvents(t *testing.T) {
	tests := []struct {
		name           string
		jsonInput      string
		expectedReport *ReportData
		expectError    bool
	}{
		{
			name: "simple passing test",
			jsonInput: `
{"Time":"2023-04-01T10:00:00Z","Action":"run","Test":"TestExample","Package":"pkg/example"}
{"Time":"2023-04-01T10:00:01Z","Action":"output","Test":"TestExample","Output":"running test\n"}
{"Time":"2023-04-01T10:00:02Z","Action":"pass","Test":"TestExample","Package":"pkg/example","Elapsed":1.5}
`,
			expectedReport: &ReportData{
				TotalTests:    1,
				PassedTests:   1,
				FailedTests:   0,
				SkippedTests:  0,
				TotalDuration: 1.5,
				SortedTestNames: []string{
					"TestExample",
				},
			},
			expectError: false,
		},
		{
			name: "simple failing test",
			jsonInput: `
{"Time":"2023-04-01T10:00:00Z","Action":"run","Test":"TestFailing","Package":"pkg/example"}
{"Time":"2023-04-01T10:00:01Z","Action":"output","Test":"TestFailing","Output":"=== RUN   TestFailing\n"}
{"Time":"2023-04-01T10:00:01Z","Action":"output","Test":"TestFailing","Output":"--- FAIL: TestFailing (0.10s)\n"}
{"Time":"2023-04-01T10:00:01Z","Action":"output","Test":"TestFailing","Output":"    failtest.go:10: some assertion failed\n"}
{"Time":"2023-04-01T10:00:02Z","Action":"fail","Test":"TestFailing","Package":"pkg/example","Elapsed":0.1}
`,
			expectedReport: &ReportData{
				TotalTests:    1,
				PassedTests:   0,
				FailedTests:   1,
				SkippedTests:  0,
				TotalDuration: 0.1,
				SortedTestNames: []string{
					"TestFailing",
				},
			},
			expectError: false,
		},
		{
			name: "test with subtests",
			jsonInput: `
{"Time":"2023-04-01T10:00:00Z","Action":"run","Test":"TestWithSubtests","Package":"pkg/example"}
{"Time":"2023-04-01T10:00:01Z","Action":"output","Test":"TestWithSubtests","Output":"=== RUN   TestWithSubtests\n"}
{"Time":"2023-04-01T10:00:01Z","Action":"run","Test":"TestWithSubtests/SubtestA","Package":"pkg/example"}
{"Time":"2023-04-01T10:00:01Z","Action":"output","Test":"TestWithSubtests/SubtestA","Output":"=== RUN   TestWithSubtests/SubtestA\n"}
{"Time":"2023-04-01T10:00:02Z","Action":"pass","Test":"TestWithSubtests/SubtestA","Package":"pkg/example","Elapsed":0.05}
{"Time":"2023-04-01T10:00:02Z","Action":"run","Test":"TestWithSubtests/SubtestB","Package":"pkg/example"}
{"Time":"2023-04-01T10:00:02Z","Action":"output","Test":"TestWithSubtests/SubtestB","Output":"=== RUN   TestWithSubtests/SubtestB\n"}
{"Time":"2023-04-01T10:00:03Z","Action":"fail","Test":"TestWithSubtests/SubtestB","Package":"pkg/example","Elapsed":0.1}
{"Time":"2023-04-01T10:00:03Z","Action":"pass","Test":"TestWithSubtests","Package":"pkg/example","Elapsed":0.3}
`,
			expectedReport: &ReportData{
				TotalTests:    1,
				PassedTests:   1, // Parent test is counted as passed
				FailedTests:   0, // Failed subtest doesn't count in the total
				SkippedTests:  0,
				TotalDuration: 0.3,
				SortedTestNames: []string{
					"TestWithSubtests",
				},
			},
			expectError: false,
		},
		{
			name: "skipped test",
			jsonInput: `
{"Time":"2023-04-01T10:00:00Z","Action":"run","Test":"TestSkipped","Package":"pkg/example"}
{"Time":"2023-04-01T10:00:01Z","Action":"output","Test":"TestSkipped","Output":"=== RUN   TestSkipped\n"}
{"Time":"2023-04-01T10:00:01Z","Action":"output","Test":"TestSkipped","Output":"--- SKIP: TestSkipped (0.01s)\n"}
{"Time":"2023-04-01T10:00:01Z","Action":"output","Test":"TestSkipped","Output":"    skip_test.go:15: skipping this test\n"}
{"Time":"2023-04-01T10:00:02Z","Action":"skip","Test":"TestSkipped","Package":"pkg/example","Elapsed":0.01}
`,
			expectedReport: &ReportData{
				TotalTests:    1,
				PassedTests:   0,
				FailedTests:   0,
				SkippedTests:  1,
				TotalDuration: 0.01,
				SortedTestNames: []string{
					"TestSkipped",
				},
			},
			expectError: false,
		},
		{
			name: "invalid json input",
			jsonInput: `
{"Time":"2023-04-01T10:00:00Z","Action":"run","Test":"TestExample"
{"Time":"2023-04-01T10:00:01Z","Action":"output","Test":"TestExample","Output":"running test\n"}
`,
			expectedReport: nil,
			expectError:    true,
		},
		{
			name: "nested subtests with multiple levels",
			jsonInput: `
{"Time":"2023-04-01T10:00:00Z","Action":"run","Test":"TestNested","Package":"pkg/example"}
{"Time":"2023-04-01T10:00:01Z","Action":"run","Test":"TestNested/Level1","Package":"pkg/example"}
{"Time":"2023-04-01T10:00:02Z","Action":"run","Test":"TestNested/Level1/Level2","Package":"pkg/example"}
{"Time":"2023-04-01T10:00:03Z","Action":"pass","Test":"TestNested/Level1/Level2","Package":"pkg/example","Elapsed":0.05}
{"Time":"2023-04-01T10:00:04Z","Action":"pass","Test":"TestNested/Level1","Package":"pkg/example","Elapsed":0.1}
{"Time":"2023-04-01T10:00:05Z","Action":"pass","Test":"TestNested","Package":"pkg/example","Elapsed":0.2}
`,
			expectedReport: &ReportData{
				TotalTests:    1,
				PassedTests:   1,
				FailedTests:   0,
				SkippedTests:  0,
				TotalDuration: 0.2,
				SortedTestNames: []string{
					"TestNested",
				},
			},
			expectError: false,
		},
		{
			name:      "empty test input",
			jsonInput: "",
			expectedReport: &ReportData{
				TotalTests:      0,
				PassedTests:     0,
				FailedTests:     0,
				SkippedTests:    0,
				TotalDuration:   0,
				SortedTestNames: []string{},
				Results:         map[string]*TestResult{},
			},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reader := strings.NewReader(tt.jsonInput)
			reportData, err := processTestEvents(reader)

			if tt.expectError && err == nil {
				t.Fatal("Expected an error but got none")
			}

			if !tt.expectError && err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}

			if tt.expectError {
				return // No need to check results if we expected an error
			}

			// Verify basic counts
			if reportData.TotalTests != tt.expectedReport.TotalTests {
				t.Errorf("TotalTests: got %d, want %d", reportData.TotalTests, tt.expectedReport.TotalTests)
			}

			if reportData.PassedTests != tt.expectedReport.PassedTests {
				t.Errorf("PassedTests: got %d, want %d", reportData.PassedTests, tt.expectedReport.PassedTests)
			}

			if reportData.FailedTests != tt.expectedReport.FailedTests {
				t.Errorf("FailedTests: got %d, want %d", reportData.FailedTests, tt.expectedReport.FailedTests)
			}

			if reportData.SkippedTests != tt.expectedReport.SkippedTests {
				t.Errorf("SkippedTests: got %d, want %d", reportData.SkippedTests, tt.expectedReport.SkippedTests)
			}

			if len(reportData.SortedTestNames) != len(tt.expectedReport.SortedTestNames) {
				t.Errorf("SortedTestNames length: got %d, want %d",
					len(reportData.SortedTestNames), len(tt.expectedReport.SortedTestNames))
			}

			// Check if expected test names exist in the report
			for _, expectedName := range tt.expectedReport.SortedTestNames {
				found := false
				for _, actualName := range reportData.SortedTestNames {
					if actualName == expectedName {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("Expected test name %s not found in results", expectedName)
				}
			}

			// Verify test results map has entries for each test
			for _, testName := range tt.expectedReport.SortedTestNames {
				if _, exists := reportData.Results[testName]; !exists {
					t.Errorf("Expected test %s not found in results map", testName)
				}
			}
		})
	}
}

func TestGenerateMarkdownReportSections(t *testing.T) {
	tests := []struct {
		name                string
		reportData          *ReportData
		expectedSections    []string
		notExpectedSections []string
	}{
		{
			name: "passing tests should not show failures section",
			reportData: &ReportData{
				TotalTests:      2,
				PassedTests:     2,
				FailedTests:     0,
				SkippedTests:    0,
				TotalDuration:   0.5,
				SortedTestNames: []string{"Test1", "Test2"},
				Results: map[string]*TestResult{
					"Test1": {Name: "Test1", Status: "PASS", Duration: 0.2},
					"Test2": {Name: "Test2", Status: "PASS", Duration: 0.3},
				},
			},
			expectedSections: []string{
				"# 🧪 Test Summary Report",
				"## 📊 Summary",
				"## 🎯 Test Status",
				"## 📝 Test Results",
				"## ⏱️ Test Durations",
				"![Status](https://img.shields.io/badge/Status-PASSED-brightgreen)",
			},
			notExpectedSections: []string{
				"## 🔴 Failed Tests Details",
				"![Status](https://img.shields.io/badge/Status-FAILED-red)",
			},
		},
		{
			name: "failing tests should show failures section",
			reportData: &ReportData{
				TotalTests:      2,
				PassedTests:     1,
				FailedTests:     1,
				SkippedTests:    0,
				TotalDuration:   0.5,
				SortedTestNames: []string{"PassingTest", "FailingTest"},
				Results: map[string]*TestResult{
					"PassingTest": {Name: "PassingTest", Status: "PASS", Duration: 0.2},
					"FailingTest": {
						Name:     "FailingTest",
						Status:   "FAIL",
						Duration: 0.3,
						Output: []string{
							"=== RUN   FailingTest",
							"--- FAIL: FailingTest (0.30s)",
							"    failtest.go:20: failure message",
						},
					},
				},
			},
			expectedSections: []string{
				"# 🧪 Test Summary Report",
				"## 📊 Summary",
				"## 🎯 Test Status",
				"## 📝 Test Results",
				"## 🔴 Failed Tests Details",
				"## ⏱️ Test Durations",
				"![Status](https://img.shields.io/badge/Status-FAILED-red)",
				"<summary>💥 Click to expand failed test details</summary>",
			},
			notExpectedSections: []string{
				"![Status](https://img.shields.io/badge/Status-PASSED-brightgreen)",
			},
		},
		{
			name: "skipped tests only should show skipped status",
			reportData: &ReportData{
				TotalTests:      1,
				PassedTests:     0,
				FailedTests:     0,
				SkippedTests:    1,
				TotalDuration:   0.01,
				SortedTestNames: []string{"SkippedTest"},
				Results: map[string]*TestResult{
					"SkippedTest": {Name: "SkippedTest", Status: "SKIP", Duration: 0.01},
				},
			},
			expectedSections: []string{
				"# 🧪 Test Summary Report",
				"## 📊 Summary",
				"## 🎯 Test Status",
				"## 📝 Test Results",
				"## ⏱️ Test Durations",
				"![Status](https://img.shields.io/badge/Status-SKIPPED-yellow)",
				"⏭️ SKIP",
			},
			notExpectedSections: []string{
				"## 🔴 Failed Tests Details",
				"![Status](https://img.shields.io/badge/Status-PASSED-brightgreen)",
				"![Status](https://img.shields.io/badge/Status-FAILED-red)",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			markdown := generateMarkdownReport(tt.reportData)

			// Check expected sections
			for _, section := range tt.expectedSections {
				if !strings.Contains(markdown, section) {
					t.Errorf("Expected section not found: %s", section)
				}
			}

			// Check sections that shouldn't be there
			for _, section := range tt.notExpectedSections {
				if strings.Contains(markdown, section) {
					t.Errorf("Unexpected section found: %s", section)
				}
			}
		})
	}
}

func TestFormattingInMarkdownReport(t *testing.T) {
	// Test focusing on specific formatting details in the report
	tests := []struct {
		name            string
		reportData      *ReportData
		checkFormatting func(t *testing.T, markdown string)
	}{
		{
			name: "test name formatting includes emoji",
			reportData: &ReportData{
				TotalTests:      3,
				PassedTests:     1,
				FailedTests:     1,
				SkippedTests:    1,
				TotalDuration:   0.6,
				SortedTestNames: []string{"PassTest", "FailTest", "SkipTest"},
				Results: map[string]*TestResult{
					"PassTest": {Name: "PassTest", Status: "PASS", Duration: 0.2},
					"FailTest": {Name: "FailTest", Status: "FAIL", Duration: 0.3},
					"SkipTest": {Name: "SkipTest", Status: "SKIP", Duration: 0.1},
				},
			},
			checkFormatting: func(t *testing.T, markdown string) {
				// Check for status emojis in test results
				if !strings.Contains(markdown, "✅ PASS") {
					t.Error("Pass emoji not found in markdown")
				}
				if !strings.Contains(markdown, "❌ FAIL") {
					t.Error("Fail emoji not found in markdown")
				}
				if !strings.Contains(markdown, "⏭️ SKIP") {
					t.Error("Skip emoji not found in markdown")
				}

				// Check for test name bolding
				if !strings.Contains(markdown, "| **PassTest**") {
					t.Error("Test name should be bold in table")
				}
			},
		},
		{
			name: "duration formatting in test table",
			reportData: &ReportData{
				TotalTests:      3,
				PassedTests:     3,
				FailedTests:     0,
				SkippedTests:    0,
				TotalDuration:   1.234,
				SortedTestNames: []string{"TestA", "TestB", "TestC"},
				Results: map[string]*TestResult{
					"TestA": {Name: "TestA", Status: "PASS", Duration: 0.123},
					"TestB": {Name: "TestB", Status: "PASS", Duration: 0.456},
					"TestC": {Name: "TestC", Status: "PASS", Duration: 0.655},
				},
			},
			checkFormatting: func(t *testing.T, markdown string) {
				// Check for properly formatted durations (3 decimal places)
				if !strings.Contains(markdown, "0.123s") {
					t.Error("Duration for TestA should be formatted as 0.123s")
				}
				if !strings.Contains(markdown, "0.456s") {
					t.Error("Duration for TestB should be formatted as 0.456s")
				}
				if !strings.Contains(markdown, "0.655s") {
					t.Error("Duration for TestC should be formatted as 0.655s")
				}

				// Check total duration formatting (2 decimal places)
				if !strings.Contains(markdown, "**Total Duration:** 1.23s") {
					t.Error("Total duration should be formatted with 2 decimal places")
				}
			},
		},
		{
			name: "bar chart in duration metrics",
			reportData: &ReportData{
				TotalTests:      2,
				PassedTests:     2,
				FailedTests:     0,
				SkippedTests:    0,
				TotalDuration:   1.5,
				SortedTestNames: []string{"ShortTest", "LongTest"},
				Results: map[string]*TestResult{
					"ShortTest": {Name: "ShortTest", Status: "PASS", Duration: 0.1},
					"LongTest":  {Name: "LongTest", Status: "PASS", Duration: 1.0},
				},
			},
			checkFormatting: func(t *testing.T, markdown string) {
				// Check for duration bar charts with block characters
				if !strings.Contains(markdown, "█") {
					t.Error("Bar chart block characters not found in duration metrics")
				}

				// The LongTest's bar should be longer than ShortTest's
				longBar := 0
				shortBar := 0
				lines := strings.Split(markdown, "\n")
				for _, line := range lines {
					if strings.Contains(line, "LongTest") && strings.Contains(line, "1.000s") {
						longBar = strings.Count(line, "█")
					}
					if strings.Contains(line, "ShortTest") && strings.Contains(line, "0.100s") {
						shortBar = strings.Count(line, "█")
					}
				}

				if longBar <= shortBar {
					t.Errorf("LongTest bar (%d blocks) should be longer than ShortTest bar (%d blocks)",
						longBar, shortBar)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			markdown := generateMarkdownReport(tt.reportData)
			tt.checkFormatting(t, markdown)
		})
	}
}
