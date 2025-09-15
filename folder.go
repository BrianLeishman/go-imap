package imap

import (
	"bytes"
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

// GetFolders retrieves the list of available folders
func (d *Dialer) GetFolders() (folders []string, err error) {
	folders = make([]string, 0)
	_, err = d.Exec(`LIST "" "*"`, false, RetryCount, func(line []byte) (err error) {
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

// ExamineFolder selects a folder in read-only mode
func (d *Dialer) ExamineFolder(folder string) (err error) {
	_, err = d.Exec(`EXAMINE "`+AddSlashes.Replace(folder)+`"`, true, RetryCount, nil)
	if err != nil {
		return err
	}
	d.Folder = folder
	d.ReadOnly = true
	return nil
}

// SelectFolder selects a folder in read-write mode
func (d *Dialer) SelectFolder(folder string) (err error) {
	_, err = d.Exec(`SELECT "`+AddSlashes.Replace(folder)+`"`, true, RetryCount, nil)
	if err != nil {
		return err
	}
	d.Folder = folder
	d.ReadOnly = false
	return nil
}

// GetTotalEmailCount returns the total email count across all folders
func (d *Dialer) GetTotalEmailCount() (count int, err error) {
	return d.GetTotalEmailCountStartingFromExcluding("", nil)
}

// GetTotalEmailCountExcluding returns total email count excluding specified folders
func (d *Dialer) GetTotalEmailCountExcluding(excludedFolders []string) (count int, err error) {
	return d.GetTotalEmailCountStartingFromExcluding("", excludedFolders)
}

// GetTotalEmailCountStartingFrom returns total email count starting from a specific folder
func (d *Dialer) GetTotalEmailCountStartingFrom(startFolder string) (count int, err error) {
	return d.GetTotalEmailCountStartingFromExcluding(startFolder, nil)
}

// GetTotalEmailCountSafe returns total email count with error handling per folder
func (d *Dialer) GetTotalEmailCountSafe() (count int, folderErrors []error, err error) {
	return d.GetTotalEmailCountSafeStartingFromExcluding("", nil)
}

// GetTotalEmailCountSafeExcluding returns total email count excluding folders with error handling
func (d *Dialer) GetTotalEmailCountSafeExcluding(excludedFolders []string) (count int, folderErrors []error, err error) {
	return d.GetTotalEmailCountSafeStartingFromExcluding("", excludedFolders)
}

// GetTotalEmailCountSafeStartingFrom returns total email count starting from folder with error handling
func (d *Dialer) GetTotalEmailCountSafeStartingFrom(startFolder string) (count int, folderErrors []error, err error) {
	return d.GetTotalEmailCountSafeStartingFromExcluding(startFolder, nil)
}

// GetFolderStats returns statistics for all folders
func (d *Dialer) GetFolderStats() ([]FolderStats, error) {
	return d.GetFolderStatsStartingFromExcluding("", nil)
}

// GetFolderStatsExcluding returns statistics for folders excluding specified ones
func (d *Dialer) GetFolderStatsExcluding(excludedFolders []string) ([]FolderStats, error) {
	return d.GetFolderStatsStartingFromExcluding("", excludedFolders)
}

// GetFolderStatsStartingFrom returns statistics for folders starting from a specific one
func (d *Dialer) GetFolderStatsStartingFrom(startFolder string) ([]FolderStats, error) {
	return d.GetFolderStatsStartingFromExcluding(startFolder, nil)
}

// GetTotalEmailCountStartingFromExcluding returns total email count with options for starting folder and exclusions
func (d *Dialer) GetTotalEmailCountStartingFromExcluding(startFolder string, excludedFolders []string) (count int, err error) {
	folders, err := d.GetFolders()
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

		if err = d.ExamineFolder(folder); err != nil {
			continue
		}

		folderCount := 0
		r, err := d.Exec("SELECT \""+AddSlashes.Replace(folder)+"\"", true, RetryCount, nil)
		if err == nil {
			re := regexp.MustCompile(`\* (\d+) EXISTS`)
			matches := re.FindStringSubmatch(r)
			if len(matches) > 1 {
				if folderCount, err = strconv.Atoi(matches[1]); err == nil {
					count += folderCount
				}
			}
		}
	}

	// Restore original folder state
	if currentFolder != "" {
		if currentReadOnly {
			_ = d.ExamineFolder(currentFolder)
		} else {
			_ = d.SelectFolder(currentFolder)
		}
	}

	return count, nil
}

// GetTotalEmailCountSafeStartingFromExcluding returns total email count with per-folder error handling
func (d *Dialer) GetTotalEmailCountSafeStartingFromExcluding(startFolder string, excludedFolders []string) (count int, folderErrors []error, err error) {
	folders, err := d.GetFolders()
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

		if err := d.ExamineFolder(folder); err != nil {
			folderErrors = append(folderErrors, fmt.Errorf("folder %s: %w", folder, err))
			continue
		}

		folderCount := 0
		r, folderErr := d.Exec("SELECT \""+AddSlashes.Replace(folder)+"\"", true, RetryCount, nil)
		if folderErr != nil {
			folderErrors = append(folderErrors, fmt.Errorf("folder %s: %w", folder, folderErr))
			continue
		}

		re := regexp.MustCompile(`\* (\d+) EXISTS`)
		matches := re.FindStringSubmatch(r)
		if len(matches) > 1 {
			if folderCount, folderErr = strconv.Atoi(matches[1]); folderErr != nil {
				folderErrors = append(folderErrors, fmt.Errorf("folder %s: %w", folder, folderErr))
				continue
			}
			count += folderCount
		}
	}

	// Restore original folder state
	if currentFolder != "" {
		if currentReadOnly {
			_ = d.ExamineFolder(currentFolder)
		} else {
			_ = d.SelectFolder(currentFolder)
		}
	}

	return count, folderErrors, nil
}

// GetFolderStatsStartingFromExcluding returns detailed statistics for folders with options
func (d *Dialer) GetFolderStatsStartingFromExcluding(startFolder string, excludedFolders []string) ([]FolderStats, error) {
	folders, err := d.GetFolders()
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

		if err := d.ExamineFolder(folder); err != nil {
			stat.Error = err
			stats = append(stats, stat)
			continue
		}

		// Get message count and max UID
		r, err := d.Exec("SELECT \""+AddSlashes.Replace(folder)+"\"", true, RetryCount, nil)
		if err != nil {
			stat.Error = err
			stats = append(stats, stat)
			continue
		}

		// Parse EXISTS response for message count
		re := regexp.MustCompile(`\* (\d+) EXISTS`)
		matches := re.FindStringSubmatch(r)
		if len(matches) > 1 {
			if count, err := strconv.Atoi(matches[1]); err == nil {
				stat.Count = count
			}
		}

		// Get highest UID
		if stat.Count > 0 {
			uidResponse, err := d.Exec("UID SEARCH ALL", true, RetryCount, nil)
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
			_ = d.ExamineFolder(currentFolder)
		} else {
			_ = d.SelectFolder(currentFolder)
		}
	}

	return stats, nil
}
