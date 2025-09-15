package imap

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// MockDialer for testing EXAMINE/SELECT redundancy
type MockDialer struct {
	execCalls    []string
	examineCount int
	selectCount  int
	responses    map[string]string
	errors       map[string]error
	Folder       string
	ReadOnly     bool
}

func (m *MockDialer) Exec(command string, expectOK bool, retryCount int, handler func(string) error) (string, error) {
	m.execCalls = append(m.execCalls, command)

	// Check for configured errors first
	if err, ok := m.errors[command]; ok {
		if strings.HasPrefix(command, "EXAMINE") {
			m.examineCount++
		}
		if strings.HasPrefix(command, "SELECT") {
			m.selectCount++
		}
		return "", err
	}

	// Check for custom responses
	if response, ok := m.responses[command]; ok {
		if strings.HasPrefix(command, "EXAMINE") {
			m.examineCount++
		}
		if strings.HasPrefix(command, "SELECT") {
			m.selectCount++
		}
		return response, nil
	}

	if strings.HasPrefix(command, "EXAMINE") {
		m.examineCount++
		// Default EXAMINE response includes EXISTS count
		return "* FLAGS (\\Answered \\Flagged \\Deleted \\Seen \\Draft)\r\n* OK [PERMANENTFLAGS ()] Read-only mailbox.\r\n* 23 EXISTS\r\n* 0 RECENT\r\n* OK [UIDVALIDITY 1] UIDs valid\r\nA1 OK [READ-ONLY] EXAMINE completed\r\n", nil
	}

	if strings.HasPrefix(command, "SELECT") {
		m.selectCount++
		// Default SELECT response also includes EXISTS count
		return "* FLAGS (\\Answered \\Flagged \\Deleted \\Seen \\Draft)\r\n* OK [PERMANENTFLAGS (\\Answered \\Flagged \\Deleted \\Seen \\Draft \\*)] Flags permitted.\r\n* 23 EXISTS\r\n* 0 RECENT\r\n* OK [UIDVALIDITY 1] UIDs valid\r\nA1 OK [READ-WRITE] SELECT completed\r\n", nil
	}

	return "", fmt.Errorf("mock error: no response configured for command: %s", command)
}

func (m *MockDialer) ExamineFolder(folder string) error {
	_, err := m.Exec(`EXAMINE "`+AddSlashes.Replace(folder)+`"`, true, RetryCount, nil)
	if err != nil {
		return err
	}
	m.Folder = folder
	m.ReadOnly = true
	return nil
}

func (m *MockDialer) selectAndGetCount(folder string) (int, error) {
	r, err := m.Exec("SELECT \""+AddSlashes.Replace(folder)+"\"", true, RetryCount, nil)
	if err != nil {
		return 0, err
	}

	// Parse EXISTS response for message count
	re := regexp.MustCompile(`\* (\d+) EXISTS`)
	matches := re.FindStringSubmatch(r)
	if len(matches) > 1 {
		if count, parseErr := strconv.Atoi(matches[1]); parseErr == nil {
			return count, nil
		}
	}

	return 0, nil
}

func TestExamineSelectRedundancy(t *testing.T) {
	tests := []struct {
		name        string
		folders     []string
		description string
	}{
		{
			name:        "single folder",
			folders:     []string{"INBOX"},
			description: "Should demonstrate redundant EXAMINE+SELECT for single folder",
		},
		{
			name:        "multiple folders",
			folders:     []string{"INBOX", "Sent", "Drafts"},
			description: "Should demonstrate redundant EXAMINE+SELECT for multiple folders",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := &MockDialer{
				execCalls: make([]string, 0),
				responses: make(map[string]string),
			}

			// Simulate the current inefficient code pattern
			for _, folder := range tt.folders {
				// Current code: First calls ExamineFolder
				err := mock.ExamineFolder(folder)
				if err != nil {
					t.Errorf("ExamineFolder() error = %v", err)
					continue
				}

				// Current code: Then immediately calls SELECT
				_, err = mock.Exec("SELECT \""+AddSlashes.Replace(folder)+"\"", true, RetryCount, nil)
				if err != nil {
					t.Errorf("SELECT error = %v", err)
				}
			}

			// Verify the redundancy
			expectedCalls := len(tt.folders) * 2 // Both EXAMINE and SELECT for each folder
			if len(mock.execCalls) != expectedCalls {
				t.Errorf("Expected %d total calls, got %d", expectedCalls, len(mock.execCalls))
			}

			if mock.examineCount != len(tt.folders) {
				t.Errorf("Expected %d EXAMINE calls, got %d", len(tt.folders), mock.examineCount)
			}

			if mock.selectCount != len(tt.folders) {
				t.Errorf("Expected %d SELECT calls, got %d", len(tt.folders), mock.selectCount)
			}

			// Verify that both EXAMINE and SELECT were called for each folder
			for i, folder := range tt.folders {
				examineCall := `EXAMINE "` + AddSlashes.Replace(folder) + `"`
				selectCall := `SELECT "` + AddSlashes.Replace(folder) + `"`

				if !contains(mock.execCalls[i*2], "EXAMINE") {
					t.Errorf("Expected EXAMINE call for folder %s, got %s", folder, mock.execCalls[i*2])
				}

				if !contains(mock.execCalls[i*2+1], "SELECT") {
					t.Errorf("Expected SELECT call for folder %s, got %s", folder, mock.execCalls[i*2+1])
				}

				_ = examineCall
				_ = selectCall
			}

			t.Logf("Redundancy demonstrated: %d folders resulted in %d calls (%d EXAMINE + %d SELECT)",
				len(tt.folders), len(mock.execCalls), mock.examineCount, mock.selectCount)
			t.Logf("Optimization opportunity: Could reduce to %d calls by using only SELECT", len(tt.folders))
		})
	}
}

func TestEfficientFolderAccess(t *testing.T) {
	tests := []struct {
		name        string
		folders     []string
		description string
	}{
		{
			name:        "single folder optimized",
			folders:     []string{"INBOX"},
			description: "Should use only SELECT to get EXISTS count",
		},
		{
			name:        "multiple folders optimized",
			folders:     []string{"INBOX", "Sent", "Drafts"},
			description: "Should use only SELECT for all folders",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := &MockDialer{
				execCalls: make([]string, 0),
				responses: make(map[string]string),
			}

			// Optimized approach: Use only SELECT
			for _, folder := range tt.folders {
				_, err := mock.Exec("SELECT \""+AddSlashes.Replace(folder)+"\"", true, RetryCount, nil)
				if err != nil {
					t.Errorf("SELECT error = %v", err)
				}
			}

			// Verify efficiency
			expectedCalls := len(tt.folders) // Only SELECT for each folder
			if len(mock.execCalls) != expectedCalls {
				t.Errorf("Expected %d total calls, got %d", expectedCalls, len(mock.execCalls))
			}

			if mock.examineCount != 0 {
				t.Errorf("Expected 0 EXAMINE calls, got %d", mock.examineCount)
			}

			if mock.selectCount != len(tt.folders) {
				t.Errorf("Expected %d SELECT calls, got %d", len(tt.folders), mock.selectCount)
			}

			t.Logf("Optimized approach: %d folders resulted in %d calls (only SELECT)",
				len(tt.folders), len(mock.execCalls))
			t.Logf("Performance improvement: 50%% reduction in IMAP commands")
		})
	}
}

func TestSelectAndGetCount(t *testing.T) {
	tests := []struct {
		name          string
		folder        string
		response      string
		expectedCount int
		expectedError bool
		description   string
	}{
		{
			name:   "successful count extraction",
			folder: "INBOX",
			response: "* FLAGS (\\Answered \\Flagged \\Deleted \\Seen \\Draft)\r\n" +
				"* OK [PERMANENTFLAGS (\\Answered \\Flagged \\Deleted \\Seen \\Draft \\*)] Flags permitted.\r\n" +
				"* 23 EXISTS\r\n" +
				"* 0 RECENT\r\n" +
				"* OK [UIDVALIDITY 1] UIDs valid\r\n" +
				"A1 OK [READ-WRITE] SELECT completed\r\n",
			expectedCount: 23,
			expectedError: false,
			description:   "Should extract EXISTS count from SELECT response",
		},
		{
			name:   "zero count extraction",
			folder: "Empty",
			response: "* FLAGS (\\Answered \\Flagged \\Deleted \\Seen \\Draft)\r\n" +
				"* OK [PERMANENTFLAGS (\\Answered \\Flagged \\Deleted \\Seen \\Draft \\*)] Flags permitted.\r\n" +
				"* 0 EXISTS\r\n" +
				"* 0 RECENT\r\n" +
				"* OK [UIDVALIDITY 1] UIDs valid\r\n" +
				"A1 OK [READ-WRITE] SELECT completed\r\n",
			expectedCount: 0,
			expectedError: false,
			description:   "Should handle empty folders correctly",
		},
		{
			name:   "large count extraction",
			folder: "Archive",
			response: "* FLAGS (\\Answered \\Flagged \\Deleted \\Seen \\Draft)\r\n" +
				"* OK [PERMANENTFLAGS (\\Answered \\Flagged \\Deleted \\Seen \\Draft \\*)] Flags permitted.\r\n" +
				"* 1500 EXISTS\r\n" +
				"* 0 RECENT\r\n" +
				"* OK [UIDVALIDITY 1] UIDs valid\r\n" +
				"A1 OK [READ-WRITE] SELECT completed\r\n",
			expectedCount: 1500,
			expectedError: false,
			description:   "Should handle large message counts",
		},
		{
			name:          "exec failure",
			folder:        "NonExistent",
			response:      "",
			expectedCount: 0,
			expectedError: true,
			description:   "Should handle SELECT command failures",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := &MockDialer{
				execCalls: make([]string, 0),
				responses: make(map[string]string),
				errors:    make(map[string]error),
			}

			if tt.expectedError {
				mock.errors["SELECT \""+AddSlashes.Replace(tt.folder)+"\""] = fmt.Errorf("folder not found: %s", tt.folder)
			} else {
				mock.responses["SELECT \""+AddSlashes.Replace(tt.folder)+"\""] = tt.response
			}

			count, err := mock.selectAndGetCount(tt.folder)

			if tt.expectedError {
				if err == nil {
					t.Errorf("selectAndGetCount() expected error but got none")
				}
			} else {
				if err != nil {
					t.Errorf("selectAndGetCount() unexpected error: %v", err)
				}
				if count != tt.expectedCount {
					t.Errorf("selectAndGetCount() got count %d, want %d", count, tt.expectedCount)
				}
			}

			// Verify only one SELECT call was made
			if len(mock.execCalls) != 1 {
				t.Errorf("selectAndGetCount() made %d calls, expected 1", len(mock.execCalls))
			}

			if mock.selectCount != 1 {
				t.Errorf("selectAndGetCount() made %d SELECT calls, expected 1", mock.selectCount)
			}

			if mock.examineCount != 0 {
				t.Errorf("selectAndGetCount() made %d EXAMINE calls, expected 0", mock.examineCount)
			}
		})
	}
}
