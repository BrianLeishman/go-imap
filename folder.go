package imap

import (
	"bytes"
	"context"
	"fmt"
	"regexp"
	"strconv"
	"time"
)

// FolderStat represents statistics for a single folder.
type FolderStat struct {
	Name   string
	Count  int
	MaxUID UID
	Error  error
}

// CountOptions configures folder iteration for TotalEmailCount and FolderStats.
// The zero value walks every folder.
type CountOptions struct {
	// StartFolder skips folders returned by LIST before this one (matched by
	// exact name). An empty value starts at the first folder.
	StartFolder string
	// ExcludeFolders names folders to skip entirely.
	ExcludeFolders []string
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
			folders = append(folders, removeSlashes.Replace(string(line[i+1:end+1])))
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
	_, err := d.Exec(ctx, `EXAMINE "`+addSlashes.Replace(folder)+`"`, true, d.effectiveRetryCount(), nil)
	if err != nil {
		return err
	}
	d.Folder = folder
	d.ReadOnly = true
	return nil
}

// SelectFolder selects a folder in read-write mode.
func (d *Client) SelectFolder(ctx context.Context, folder string) error {
	_, err := d.Exec(ctx, `SELECT "`+addSlashes.Replace(folder)+`"`, true, d.effectiveRetryCount(), nil)
	if err != nil {
		return err
	}
	d.Folder = folder
	d.ReadOnly = false
	return nil
}

// selectAndGetCount executes SELECT command and extracts message count from EXISTS response.
func (d *Client) selectAndGetCount(ctx context.Context, folder string) (int, error) {
	r, err := d.Exec(ctx, "SELECT \""+addSlashes.Replace(folder)+"\"", true, d.effectiveRetryCount(), nil)
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
	_, err := d.Exec(ctx, `CREATE "`+addSlashes.Replace(name)+`"`, false, 0, nil)
	if err != nil {
		return fmt.Errorf("imap create folder: %w", err)
	}
	return nil
}

// DeleteFolder permanently removes a mailbox.
// If the deleted folder is currently selected, the folder state is cleared.
// This command is not retried because DELETE is not idempotent.
func (d *Client) DeleteFolder(ctx context.Context, name string) error {
	_, err := d.Exec(ctx, `DELETE "`+addSlashes.Replace(name)+`"`, false, 0, nil)
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
	_, err := d.Exec(ctx, `RENAME "`+addSlashes.Replace(oldName)+`" "`+addSlashes.Replace(newName)+`"`, false, 0, nil)
	if err != nil {
		return fmt.Errorf("imap rename folder: %w", err)
	}
	if d.Folder == oldName {
		d.Folder = newName
	}
	return nil
}

// TotalEmailCount sums message counts across folders. Per-folder failures
// are reported in folderErrors but do not abort iteration. err is non-nil
// if the initial folder list could not be retrieved or if ctx is cancelled
// mid-iteration; in the latter case count and folderErrors hold the
// partial result accumulated before cancellation.
func (d *Client) TotalEmailCount(ctx context.Context, opts CountOptions) (count int, folderErrors []error, err error) {
	folders, err := d.GetFolders(ctx)
	if err != nil {
		return 0, nil, err
	}

	excludeMap := make(map[string]bool, len(opts.ExcludeFolders))
	for _, folder := range opts.ExcludeFolders {
		excludeMap[folder] = true
	}

	defer d.restoreSelection(ctx, d.Folder, d.ReadOnly)

	startFound := opts.StartFolder == ""
	for _, folder := range folders {
		if cerr := ctx.Err(); cerr != nil {
			return count, folderErrors, cerr
		}
		if !startFound {
			if folder != opts.StartFolder {
				continue
			}
			startFound = true
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

	return count, folderErrors, nil
}

// FolderStats returns per-folder statistics. Per-folder failures are reported
// in FolderStat.Error rather than aborting the iteration. If ctx is cancelled
// mid-iteration the partial slice is returned with ctx.Err().
func (d *Client) FolderStats(ctx context.Context, opts CountOptions) ([]FolderStat, error) {
	folders, err := d.GetFolders(ctx)
	if err != nil {
		return nil, err
	}

	excludeMap := make(map[string]bool, len(opts.ExcludeFolders))
	for _, folder := range opts.ExcludeFolders {
		excludeMap[folder] = true
	}

	defer d.restoreSelection(ctx, d.Folder, d.ReadOnly)

	var stats []FolderStat
	startFound := opts.StartFolder == ""
	for _, folder := range folders {
		if cerr := ctx.Err(); cerr != nil {
			return stats, cerr
		}
		if !startFound {
			if folder != opts.StartFolder {
				continue
			}
			startFound = true
		}
		if excludeMap[folder] {
			continue
		}

		stat := FolderStat{Name: folder}
		count, err := d.selectAndGetCount(ctx, folder)
		if err != nil {
			stat.Error = err
			stats = append(stats, stat)
			continue
		}
		stat.Count = count

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

	return stats, nil
}

// restoreSelection re-selects the previously selected folder (if any) using
// the original read-only/read-write mode. The caller's context cancellation
// is intentionally detached so a cancelled or timed-out iteration does not
// leave the client selected on an unrelated mailbox; an explicit cleanup
// deadline still bounds the SELECT/EXAMINE so a stalled server cannot
// block the call forever.
func (d *Client) restoreSelection(ctx context.Context, folder string, readOnly bool) {
	if folder == "" {
		return
	}
	restoreCtx, cancel := cleanupContext(ctx)
	defer cancel()
	if readOnly {
		_ = d.ExamineFolder(restoreCtx, folder)
	} else {
		_ = d.SelectFolder(restoreCtx, folder)
	}
}

// cleanupContext returns a derived context suitable for best-effort cleanup
// after the caller's ctx has been cancelled or timed out. Cancellation is
// detached (so the cleanup runs to completion) but a fallback deadline is
// applied so a stalled server cannot hang the cleanup indefinitely.
func cleanupContext(ctx context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
}
