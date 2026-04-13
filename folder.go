package imap

import (
	"bytes"
	"context"
	"fmt"
	"regexp"
	"strconv"
)

// FolderStats represents statistics for a folder
type FolderStats struct {
	Name   string
	Count  int
	MaxUID int
	Error  error
}

// GetFolders retrieves the list of available folders.
func (d *Client) GetFolders(ctx context.Context) (folders []string, err error) {
	folders = make([]string, 0)
	_, err = d.Exec(ctx, `LIST "" "*"`, false, d.effectiveRetryCount(), func(line []byte) (err error) {
		line = dropNl(line)
		if b := bytes.IndexByte(line, '\n'); b != -1 {
			folders = append(folders, string(line[b+1:]))
		} else {
			if len(line) == 0 {
				return err
			}
			i := len(line) - 1
			quoted := line[i] == '"'
			delim := byte(' ')
			if quoted {
				delim = '"'
				i--
			}
			end := i
			for i > 0 {
				if line[i] == delim {
					if !quoted || line[i-1] != '\\' {
						break
					}
				}
				i--
			}
			folders = append(folders, RemoveSlashes.Replace(string(line[i+1:end+1])))
		}
		return err
	})
	if err != nil {
		return nil, err
	}

	return folders, nil
}

// ExamineFolder selects a folder in read-only mode.
func (d *Client) ExamineFolder(ctx context.Context, folder string) error {
	_, err := d.Exec(ctx, `EXAMINE "`+AddSlashes.Replace(folder)+`"`, true, d.effectiveRetryCount(), nil)
	if err != nil {
		return err
	}
	d.Folder = folder
	d.ReadOnly = true
	return nil
}

// SelectFolder selects a folder in read-write mode.
func (d *Client) SelectFolder(ctx context.Context, folder string) error {
	_, err := d.Exec(ctx, `SELECT "`+AddSlashes.Replace(folder)+`"`, true, d.effectiveRetryCount(), nil)
	if err != nil {
		return err
	}
	d.Folder = folder
	d.ReadOnly = false
	return nil
}

// selectAndGetCount executes SELECT command and extracts message count from EXISTS response.
func (d *Client) selectAndGetCount(ctx context.Context, folder string) (int, error) {
	r, err := d.Exec(ctx, "SELECT \""+AddSlashes.Replace(folder)+"\"", true, d.effectiveRetryCount(), nil)
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

// CreateFolder creates a new mailbox with the given name.
// This command is not retried because CREATE is not idempotent.
func (d *Client) CreateFolder(ctx context.Context, name string) error {
	_, err := d.Exec(ctx, `CREATE "`+AddSlashes.Replace(name)+`"`, false, 0, nil)
	if err != nil {
		return fmt.Errorf("imap create folder: %w", err)
	}
	return nil
}

// DeleteFolder permanently removes a mailbox.
// If the deleted folder is currently selected, the folder state is cleared.
// This command is not retried because DELETE is not idempotent.
func (d *Client) DeleteFolder(ctx context.Context, name string) error {
	_, err := d.Exec(ctx, `DELETE "`+AddSlashes.Replace(name)+`"`, false, 0, nil)
	if err != nil {
		return fmt.Errorf("imap delete folder: %w", err)
	}
	if d.Folder == name {
		d.Folder = ""
		d.ReadOnly = false
	}
	return nil
}

// RenameFolder renames a mailbox from oldName to newName.
// If the renamed folder is currently selected, the tracked folder name is updated.
// This command is not retried because RENAME is not idempotent.
func (d *Client) RenameFolder(ctx context.Context, oldName, newName string) error {
	_, err := d.Exec(ctx, `RENAME "`+AddSlashes.Replace(oldName)+`" "`+AddSlashes.Replace(newName)+`"`, false, 0, nil)
	if err != nil {
		return fmt.Errorf("imap rename folder: %w", err)
	}
	if d.Folder == oldName {
		d.Folder = newName
	}
	return nil
}

// GetTotalEmailCount returns the total email count across all folders.
func (d *Client) GetTotalEmailCount(ctx context.Context) (count int, err error) {
	return d.GetTotalEmailCountStartingFromExcluding(ctx, "", nil)
}

// GetTotalEmailCountExcluding returns total email count excluding specified folders.
func (d *Client) GetTotalEmailCountExcluding(ctx context.Context, excludedFolders []string) (count int, err error) {
	return d.GetTotalEmailCountStartingFromExcluding(ctx, "", excludedFolders)
}

// GetTotalEmailCountStartingFrom returns total email count starting from a specific folder.
func (d *Client) GetTotalEmailCountStartingFrom(ctx context.Context, startFolder string) (count int, err error) {
	return d.GetTotalEmailCountStartingFromExcluding(ctx, startFolder, nil)
}

// GetTotalEmailCountSafe returns total email count with error handling per folder.
func (d *Client) GetTotalEmailCountSafe(ctx context.Context) (count int, folderErrors []error, err error) {
	return d.GetTotalEmailCountSafeStartingFromExcluding(ctx, "", nil)
}

// GetTotalEmailCountSafeExcluding returns total email count excluding folders with error handling.
func (d *Client) GetTotalEmailCountSafeExcluding(ctx context.Context, excludedFolders []string) (count int, folderErrors []error, err error) {
	return d.GetTotalEmailCountSafeStartingFromExcluding(ctx, "", excludedFolders)
}

// GetTotalEmailCountSafeStartingFrom returns total email count starting from folder with error handling.
func (d *Client) GetTotalEmailCountSafeStartingFrom(ctx context.Context, startFolder string) (count int, folderErrors []error, err error) {
	return d.GetTotalEmailCountSafeStartingFromExcluding(ctx, startFolder, nil)
}

// GetFolderStats returns statistics for all folders.
func (d *Client) GetFolderStats(ctx context.Context) ([]FolderStats, error) {
	return d.GetFolderStatsStartingFromExcluding(ctx, "", nil)
}

// GetFolderStatsExcluding returns statistics for folders excluding specified ones.
func (d *Client) GetFolderStatsExcluding(ctx context.Context, excludedFolders []string) ([]FolderStats, error) {
	return d.GetFolderStatsStartingFromExcluding(ctx, "", excludedFolders)
}

// GetFolderStatsStartingFrom returns statistics for folders starting from a specific one.
func (d *Client) GetFolderStatsStartingFrom(ctx context.Context, startFolder string) ([]FolderStats, error) {
	return d.GetFolderStatsStartingFromExcluding(ctx, startFolder, nil)
}

// GetTotalEmailCountStartingFromExcluding returns total email count with options for starting folder and exclusions.
func (d *Client) GetTotalEmailCountStartingFromExcluding(ctx context.Context, startFolder string, excludedFolders []string) (count int, err error) {
	folders, err := d.GetFolders(ctx)
	if err != nil {
		return 0, err
	}

	startFound := startFolder == ""
	excludeMap := make(map[string]bool)
	for _, folder := range excludedFolders {
		excludeMap[folder] = true
	}

	currentFolder := d.Folder
	currentReadOnly := d.ReadOnly

	for _, folder := range folders {
		if !startFound {
			if folder == startFolder {
				startFound = true
			} else {
				continue
			}
		}

		if excludeMap[folder] {
			continue
		}

		folderCount, err := d.selectAndGetCount(ctx, folder)
		if err == nil {
			count += folderCount
		}
	}

	// Restore original folder state
	if currentFolder != "" {
		if currentReadOnly {
			_ = d.ExamineFolder(ctx, currentFolder)
		} else {
			_ = d.SelectFolder(ctx, currentFolder)
		}
	}

	return count, nil
}

// GetTotalEmailCountSafeStartingFromExcluding returns total email count with per-folder error handling.
func (d *Client) GetTotalEmailCountSafeStartingFromExcluding(ctx context.Context, startFolder string, excludedFolders []string) (count int, folderErrors []error, err error) {
	folders, err := d.GetFolders(ctx)
	if err != nil {
		return 0, nil, err
	}

	startFound := startFolder == ""
	excludeMap := make(map[string]bool)
	for _, folder := range excludedFolders {
		excludeMap[folder] = true
	}

	currentFolder := d.Folder
	currentReadOnly := d.ReadOnly

	for _, folder := range folders {
		if !startFound {
			if folder == startFolder {
				startFound = true
			} else {
				continue
			}
		}

		if excludeMap[folder] {
			continue
		}

		folderCount, folderErr := d.selectAndGetCount(ctx, folder)
		if folderErr != nil {
			folderErrors = append(folderErrors, fmt.Errorf("folder %s: %w", folder, folderErr))
			continue
		}
		count += folderCount
	}

	// Restore original folder state
	if currentFolder != "" {
		if currentReadOnly {
			_ = d.ExamineFolder(ctx, currentFolder)
		} else {
			_ = d.SelectFolder(ctx, currentFolder)
		}
	}

	return count, folderErrors, nil
}

// GetFolderStatsStartingFromExcluding returns detailed statistics for folders with options.
func (d *Client) GetFolderStatsStartingFromExcluding(ctx context.Context, startFolder string, excludedFolders []string) ([]FolderStats, error) {
	folders, err := d.GetFolders(ctx)
	if err != nil {
		return nil, err
	}

	startFound := startFolder == ""
	excludeMap := make(map[string]bool)
	for _, folder := range excludedFolders {
		excludeMap[folder] = true
	}

	currentFolder := d.Folder
	currentReadOnly := d.ReadOnly

	var stats []FolderStats

	for _, folder := range folders {
		if !startFound {
			if folder == startFolder {
				startFound = true
			} else {
				continue
			}
		}

		if excludeMap[folder] {
			continue
		}

		stat := FolderStats{Name: folder}

		// Get message count using helper function
		count, err := d.selectAndGetCount(ctx, folder)
		if err != nil {
			stat.Error = err
			stats = append(stats, stat)
			continue
		}
		stat.Count = count

		// Get highest UID
		if stat.Count > 0 {
			uidResponse, err := d.Exec(ctx, "UID SEARCH ALL", true, d.effectiveRetryCount(), nil)
			if err == nil {
				uids, err := parseUIDSearchResponse(uidResponse)
				if err == nil && len(uids) > 0 {
					stat.MaxUID = uids[len(uids)-1]
				}
			}
		}

		stats = append(stats, stat)
	}

	// Restore original folder state
	if currentFolder != "" {
		if currentReadOnly {
			_ = d.ExamineFolder(ctx, currentFolder)
		} else {
			_ = d.SelectFolder(ctx, currentFolder)
		}
	}

	return stats, nil
}
