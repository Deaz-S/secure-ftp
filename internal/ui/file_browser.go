// Package ui provides the file browser component.
package ui

import (
	"context"
	"fmt"
	"image/color"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"secure-ftp/internal/protocol"
)

// FileItem represents a file or directory in the browser.
type FileItem struct {
	Name        string
	Path        string
	IsDir       bool
	Size        int64
	Permissions string
	Selected    bool
}

// FileBrowser provides a file navigation component.
type FileBrowser struct {
	window      fyne.Window
	isLocal     bool
	client      protocol.Protocol
	currentPath string
	files       []FileItem
	disabled    bool
	showHidden  bool

	// UI components
	container     *fyne.Container
	pathEntry     *widget.Entry
	fileList      *widget.List
	pathLabel     *widget.Label
	dropHighlight *canvas.Rectangle

	// Action buttons
	newFolderBtn *widget.Button
	deleteBtn    *widget.Button
	renameBtn    *widget.Button

	// Cache
	cache *DirCache

	// Callbacks
	onFileDoubleClick func(path string, isDir bool)
	onSelectionChange func([]string)
	onNewFolder       func()
	onDelete          func()
	onRename          func()
	onDragStart       func(items []FileItem)

	// Selection state
	selectedIndices map[int]bool
	lastSelectedIdx int
}

// NewFileBrowser creates a new file browser.
func NewFileBrowser(window fyne.Window, isLocal bool, startPath string) *FileBrowser {
	fb := &FileBrowser{
		window:          window,
		isLocal:         isLocal,
		currentPath:     startPath,
		files:           make([]FileItem, 0),
		selectedIndices: make(map[int]bool),
		cache:           NewDirCache(DefaultCacheTTL),
		lastSelectedIdx: -1,
	}

	fb.buildUI()

	if isLocal {
		fb.NavigateTo(startPath)
	}

	return fb
}

// buildUI constructs the browser UI.
func (fb *FileBrowser) buildUI() {
	// Title
	title := "Local"
	if !fb.isLocal {
		title = "Distant"
	}

	// Path entry with navigation
	fb.pathEntry = widget.NewEntry()
	fb.pathEntry.SetText(fb.currentPath)
	fb.pathEntry.OnSubmitted = func(path string) {
		fb.NavigateTo(path)
	}

	goBtn := widget.NewButtonWithIcon("", theme.NavigateNextIcon(), func() {
		fb.NavigateTo(fb.pathEntry.Text)
	})

	upBtn := widget.NewButtonWithIcon("", theme.MoveUpIcon(), fb.navigateUp)
	refreshBtn := widget.NewButtonWithIcon("", theme.ViewRefreshIcon(), fb.Refresh)
	homeBtn := widget.NewButtonWithIcon("", theme.HomeIcon(), fb.navigateHome)

	// Action buttons for file operations
	fb.newFolderBtn = widget.NewButtonWithIcon("", theme.FolderNewIcon(), func() {
		if fb.onNewFolder != nil {
			fb.onNewFolder()
		}
	})
	fb.newFolderBtn.Importance = widget.LowImportance

	fb.deleteBtn = widget.NewButtonWithIcon("", theme.DeleteIcon(), func() {
		if fb.onDelete != nil {
			fb.onDelete()
		}
	})
	fb.deleteBtn.Importance = widget.LowImportance
	fb.deleteBtn.Disable()

	fb.renameBtn = widget.NewButtonWithIcon("", theme.DocumentCreateIcon(), func() {
		if fb.onRename != nil {
			fb.onRename()
		}
	})
	fb.renameBtn.Importance = widget.LowImportance
	fb.renameBtn.Disable()

	pathBar := container.NewBorder(
		nil, nil,
		container.NewHBox(upBtn, homeBtn),
		container.NewHBox(fb.newFolderBtn, fb.renameBtn, fb.deleteBtn, goBtn, refreshBtn),
		fb.pathEntry,
	)

	// File list
	fb.fileList = widget.NewList(
		func() int {
			return len(fb.files)
		},
		func() fyne.CanvasObject {
			return container.NewHBox(
				widget.NewIcon(theme.FileIcon()),
				widget.NewLabel("filename.txt"),
				widget.NewLabel("1.2 MB"),
			)
		},
		func(id widget.ListItemID, obj fyne.CanvasObject) {
			if id >= len(fb.files) {
				return
			}

			item := fb.files[id]
			box := obj.(*fyne.Container)

			// Icon
			icon := box.Objects[0].(*widget.Icon)
			if item.IsDir {
				icon.SetResource(theme.FolderIcon())
			} else {
				icon.SetResource(theme.FileIcon())
			}

			// Name
			nameLabel := box.Objects[1].(*widget.Label)
			nameLabel.SetText(item.Name)

			// Size
			sizeLabel := box.Objects[2].(*widget.Label)
			if item.IsDir {
				sizeLabel.SetText("<DIR>")
			} else {
				sizeLabel.SetText(formatSize(item.Size))
			}
		},
	)

	// Selection handling
	fb.fileList.OnSelected = func(id widget.ListItemID) {
		if id >= len(fb.files) {
			return
		}

		item := fb.files[id]
		fb.lastSelectedIdx = id
		fb.selectedIndices[id] = true

		// Enable action buttons when something is selected (not "..")
		if item.Name != ".." {
			fb.deleteBtn.Enable()
			fb.renameBtn.Enable()
		} else {
			fb.deleteBtn.Disable()
			fb.renameBtn.Disable()
		}

		if fb.onSelectionChange != nil {
			fb.onSelectionChange(fb.GetSelectedFiles())
		}

		// For directories, navigate on selection
		if item.IsDir && item.Name != ".." {
			// Don't auto-navigate, let user double-click or press enter
		} else if item.IsDir && item.Name == ".." {
			fb.NavigateTo(item.Path)
		} else {
			// File selected - trigger callback
			if fb.onFileDoubleClick != nil {
				fb.onFileDoubleClick(item.Path, false)
			}
		}
	}

	fb.fileList.OnUnselected = func(id widget.ListItemID) {
		delete(fb.selectedIndices, id)

		// Disable action buttons if nothing is selected
		if len(fb.selectedIndices) == 0 {
			fb.deleteBtn.Disable()
			fb.renameBtn.Disable()
		}

		if fb.onSelectionChange != nil {
			fb.onSelectionChange(fb.GetSelectedFiles())
		}
	}

	// Build container
	header := widget.NewLabelWithStyle(title, fyne.TextAlignCenter, fyne.TextStyle{Bold: true})

	// Create drop highlight overlay (initially hidden)
	fb.dropHighlight = canvas.NewRectangle(color.NRGBA{R: 0, G: 150, B: 255, A: 60})
	fb.dropHighlight.StrokeColor = color.NRGBA{R: 0, G: 150, B: 255, A: 200}
	fb.dropHighlight.StrokeWidth = 3
	fb.dropHighlight.Hide()

	// Use a stack to overlay the drop highlight on the file list
	listWithHighlight := container.NewStack(fb.fileList, fb.dropHighlight)

	fb.container = container.NewBorder(
		container.NewVBox(header, pathBar),
		nil, nil, nil,
		listWithHighlight,
	)
}

// GetContainer returns the browser's container.
func (fb *FileBrowser) GetContainer() *fyne.Container {
	return fb.container
}

// SetClient sets the protocol client for remote browsing.
func (fb *FileBrowser) SetClient(client protocol.Protocol) {
	fb.client = client
}

// SetDisabled enables or disables the browser.
func (fb *FileBrowser) SetDisabled(disabled bool) {
	fb.disabled = disabled
	if disabled {
		fb.pathEntry.Disable()
	} else {
		fb.pathEntry.Enable()
	}
}

// NavigateTo navigates to a specific path.
func (fb *FileBrowser) NavigateTo(path string) {
	if fb.disabled {
		return
	}

	// Clean path
	path = filepath.Clean(path)

	var items []FileItem
	var err error
	var fromCache bool

	// Try to get from cache first
	if fb.cache != nil {
		cacheKey := fb.getCacheKey(path)
		if cachedItems, ok := fb.cache.Get(cacheKey); ok {
			items = cachedItems
			fromCache = true
		}
	}

	if !fromCache {
		if fb.isLocal {
			items, err = fb.readLocalDirectory(path)
		} else {
			items, err = fb.readRemoteDirectory(path)
		}

		if err != nil {
			// Show error but don't change directory
			return
		}

		// Store in cache
		if fb.cache != nil {
			cacheKey := fb.getCacheKey(path)
			fb.cache.Set(cacheKey, items)
		}
	}

	fb.currentPath = path
	fb.files = items
	fb.selectedIndices = make(map[int]bool)
	fb.lastSelectedIdx = -1
	fb.pathEntry.SetText(path)
	fb.fileList.Refresh()

	// Reset action buttons
	fb.deleteBtn.Disable()
	fb.renameBtn.Disable()
}

// getCacheKey generates a cache key for a path.
func (fb *FileBrowser) getCacheKey(path string) string {
	prefix := "local:"
	if !fb.isLocal {
		prefix = "remote:"
	}
	return prefix + path
}

// readLocalDirectory reads the contents of a local directory.
func (fb *FileBrowser) readLocalDirectory(path string) ([]FileItem, error) {
	entries, err := os.ReadDir(path)
	if err != nil {
		return nil, err
	}

	var items []FileItem

	// Add parent directory
	if path != "/" {
		items = append(items, FileItem{
			Name:  "..",
			Path:  filepath.Dir(path),
			IsDir: true,
		})
	}

	for _, entry := range entries {
		info, err := entry.Info()
		if err != nil {
			continue
		}

		// Skip hidden files unless configured to show them
		if !fb.showHidden && strings.HasPrefix(entry.Name(), ".") {
			continue
		}

		items = append(items, FileItem{
			Name:        entry.Name(),
			Path:        filepath.Join(path, entry.Name()),
			IsDir:       entry.IsDir(),
			Size:        info.Size(),
			Permissions: info.Mode().String(),
		})
	}

	// Sort: directories first, then by name
	sort.Slice(items, func(i, j int) bool {
		if items[i].Name == ".." {
			return true
		}
		if items[j].Name == ".." {
			return false
		}
		if items[i].IsDir != items[j].IsDir {
			return items[i].IsDir
		}
		return strings.ToLower(items[i].Name) < strings.ToLower(items[j].Name)
	})

	return items, nil
}

// readRemoteDirectory reads the contents of a remote directory.
func (fb *FileBrowser) readRemoteDirectory(path string) ([]FileItem, error) {
	if fb.client == nil {
		return nil, fmt.Errorf("non connecté")
	}

	entries, err := fb.client.List(context.Background(), path)
	if err != nil {
		return nil, err
	}

	var items []FileItem

	// Add parent directory
	if path != "/" {
		items = append(items, FileItem{
			Name:  "..",
			Path:  filepath.Dir(path),
			IsDir: true,
		})
	}

	for _, entry := range entries {
		// Skip hidden files unless configured to show them
		if !fb.showHidden && strings.HasPrefix(entry.Name, ".") {
			continue
		}

		items = append(items, FileItem{
			Name:        entry.Name,
			Path:        filepath.Join(path, entry.Name),
			IsDir:       entry.IsDir,
			Size:        entry.Size,
			Permissions: entry.Permissions,
		})
	}

	// Sort: directories first, then by name
	sort.Slice(items, func(i, j int) bool {
		if items[i].Name == ".." {
			return true
		}
		if items[j].Name == ".." {
			return false
		}
		if items[i].IsDir != items[j].IsDir {
			return items[i].IsDir
		}
		return strings.ToLower(items[i].Name) < strings.ToLower(items[j].Name)
	})

	return items, nil
}

// navigateUp navigates to the parent directory.
func (fb *FileBrowser) navigateUp() {
	parent := filepath.Dir(fb.currentPath)
	fb.NavigateTo(parent)
}

// navigateHome navigates to the home directory.
func (fb *FileBrowser) navigateHome() {
	if fb.isLocal {
		home, _ := os.UserHomeDir()
		fb.NavigateTo(home)
	} else {
		fb.NavigateTo("/")
	}
}

// Refresh reloads the current directory, bypassing cache.
func (fb *FileBrowser) Refresh() {
	// Invalidate cache for current path
	if fb.cache != nil {
		cacheKey := fb.getCacheKey(fb.currentPath)
		fb.cache.Invalidate(cacheKey)
	}
	fb.NavigateTo(fb.currentPath)
}

// InvalidateCache invalidates the cache for a specific path.
func (fb *FileBrowser) InvalidateCache(path string) {
	if fb.cache != nil {
		cacheKey := fb.getCacheKey(path)
		fb.cache.Invalidate(cacheKey)
	}
}

// ClearCache clears the entire cache.
func (fb *FileBrowser) ClearCache() {
	if fb.cache != nil {
		fb.cache.InvalidateAll()
	}
}

// Clear clears the file list.
func (fb *FileBrowser) Clear() {
	fb.files = make([]FileItem, 0)
	fb.selectedIndices = make(map[int]bool)
	fb.fileList.Refresh()
}

// GetCurrentPath returns the current directory path.
func (fb *FileBrowser) GetCurrentPath() string {
	return fb.currentPath
}

// GetSelectedFiles returns the paths of selected files.
func (fb *FileBrowser) GetSelectedFiles() []string {
	var selected []string
	for idx := range fb.selectedIndices {
		if idx < len(fb.files) && !fb.files[idx].IsDir && fb.files[idx].Name != ".." {
			selected = append(selected, fb.files[idx].Path)
		}
	}
	return selected
}

// GetFileName extracts the filename from a path.
func (fb *FileBrowser) GetFileName(path string) string {
	return filepath.Base(path)
}

// SetOnFileDoubleClick sets the callback for file double-clicks.
func (fb *FileBrowser) SetOnFileDoubleClick(callback func(path string, isDir bool)) {
	fb.onFileDoubleClick = callback
}

// SetOnSelectionChange sets the callback for selection changes.
func (fb *FileBrowser) SetOnSelectionChange(callback func([]string)) {
	fb.onSelectionChange = callback
}

// SetShowHidden sets whether to show hidden files.
func (fb *FileBrowser) SetShowHidden(show bool) {
	fb.showHidden = show
	fb.Refresh()
}

// SetOnNewFolder sets the callback for new folder action.
func (fb *FileBrowser) SetOnNewFolder(callback func()) {
	fb.onNewFolder = callback
}

// SetOnDelete sets the callback for delete action.
func (fb *FileBrowser) SetOnDelete(callback func()) {
	fb.onDelete = callback
}

// SetOnRename sets the callback for rename action.
func (fb *FileBrowser) SetOnRename(callback func()) {
	fb.onRename = callback
}

// GetSelectedItem returns the currently selected file item, or nil if none.
func (fb *FileBrowser) GetSelectedItem() *FileItem {
	if fb.lastSelectedIdx >= 0 && fb.lastSelectedIdx < len(fb.files) {
		item := fb.files[fb.lastSelectedIdx]
		if item.Name != ".." {
			return &item
		}
	}
	return nil
}

// GetSelectedItems returns all selected file items.
func (fb *FileBrowser) GetSelectedItems() []FileItem {
	var items []FileItem
	for idx := range fb.selectedIndices {
		if idx < len(fb.files) && fb.files[idx].Name != ".." {
			items = append(items, fb.files[idx])
		}
	}
	return items
}

// formatSize formats a file size in human-readable form.
func formatSize(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}

// SetDropHighlight sets the drop highlight state for drag & drop visual feedback.
func (fb *FileBrowser) SetDropHighlight(highlighted bool) {
	if fb.dropHighlight == nil {
		return
	}
	if highlighted {
		fb.dropHighlight.Show()
	} else {
		fb.dropHighlight.Hide()
	}
	fb.dropHighlight.Refresh()
}

// SetOnDragStart sets the callback for when a drag operation starts.
func (fb *FileBrowser) SetOnDragStart(callback func(items []FileItem)) {
	fb.onDragStart = callback
}

// IsLocal returns whether this is a local file browser.
func (fb *FileBrowser) IsLocal() bool {
	return fb.isLocal
}
