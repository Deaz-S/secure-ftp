// Package ui provides drag and drop support for file transfers.
package ui

import (
	"image/color"
	"sync"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/widget"
)

// DragDropManager manages drag and drop operations between file browsers.
type DragDropManager struct {
	mu sync.Mutex

	// Drag state
	isDragging bool
	dragSource *FileBrowser
	dragItems  []FileItem

	// Drop targets
	localBrowser  *FileBrowser
	remoteBrowser *FileBrowser

	// Visual feedback
	dragOverlay *canvas.Rectangle
	dragLabel   *canvas.Text
	window      fyne.Window

	// Callbacks
	onUpload   func(localPath string)
	onDownload func(remotePath string)
}

// NewDragDropManager creates a new drag and drop manager.
func NewDragDropManager(window fyne.Window) *DragDropManager {
	return &DragDropManager{
		window: window,
	}
}

// SetBrowsers sets the local and remote file browsers.
func (ddm *DragDropManager) SetBrowsers(local, remote *FileBrowser) {
	ddm.localBrowser = local
	ddm.remoteBrowser = remote
}

// SetOnUpload sets the callback for upload operations.
func (ddm *DragDropManager) SetOnUpload(fn func(localPath string)) {
	ddm.onUpload = fn
}

// SetOnDownload sets the callback for download operations.
func (ddm *DragDropManager) SetOnDownload(fn func(remotePath string)) {
	ddm.onDownload = fn
}

// StartDrag begins a drag operation from the specified browser.
func (ddm *DragDropManager) StartDrag(source *FileBrowser, items []FileItem) {
	ddm.mu.Lock()
	defer ddm.mu.Unlock()

	if len(items) == 0 {
		return
	}

	ddm.isDragging = true
	ddm.dragSource = source
	ddm.dragItems = items

	// Show visual feedback on target browser
	ddm.showDropTarget()
}

// EndDrag ends the current drag operation.
func (ddm *DragDropManager) EndDrag() {
	ddm.mu.Lock()
	defer ddm.mu.Unlock()

	ddm.isDragging = false
	ddm.dragSource = nil
	ddm.dragItems = nil

	// Hide visual feedback
	ddm.hideDropTarget()
}

// Drop performs the drop at the current position.
func (ddm *DragDropManager) Drop(target *FileBrowser) {
	ddm.mu.Lock()
	source := ddm.dragSource
	items := ddm.dragItems
	isDragging := ddm.isDragging
	ddm.mu.Unlock()

	if !isDragging || source == nil || len(items) == 0 {
		ddm.EndDrag()
		return
	}

	// Determine operation based on source and target
	if source.isLocal && target == ddm.remoteBrowser {
		// Local → Remote = Upload
		for _, item := range items {
			if !item.IsDir && ddm.onUpload != nil {
				ddm.onUpload(item.Path)
			}
		}
	} else if !source.isLocal && target == ddm.localBrowser {
		// Remote → Local = Download
		for _, item := range items {
			if !item.IsDir && ddm.onDownload != nil {
				ddm.onDownload(item.Path)
			}
		}
	}

	ddm.EndDrag()
}

// IsDragging returns whether a drag operation is in progress.
func (ddm *DragDropManager) IsDragging() bool {
	ddm.mu.Lock()
	defer ddm.mu.Unlock()
	return ddm.isDragging
}

// GetDragSource returns the source browser of the current drag.
func (ddm *DragDropManager) GetDragSource() *FileBrowser {
	ddm.mu.Lock()
	defer ddm.mu.Unlock()
	return ddm.dragSource
}

// showDropTarget shows visual feedback on the drop target.
func (ddm *DragDropManager) showDropTarget() {
	// Highlight the target browser
	if ddm.dragSource == ddm.localBrowser && ddm.remoteBrowser != nil {
		ddm.remoteBrowser.SetDropHighlight(true)
	} else if ddm.dragSource == ddm.remoteBrowser && ddm.localBrowser != nil {
		ddm.localBrowser.SetDropHighlight(true)
	}
}

// hideDropTarget hides the drop target visual feedback.
func (ddm *DragDropManager) hideDropTarget() {
	if ddm.localBrowser != nil {
		ddm.localBrowser.SetDropHighlight(false)
	}
	if ddm.remoteBrowser != nil {
		ddm.remoteBrowser.SetDropHighlight(false)
	}
}

// DropZone is a widget that accepts drops.
type DropZone struct {
	widget.BaseWidget
	content     fyne.CanvasObject
	highlighted bool
	highlightBg *canvas.Rectangle
	onDrop      func()
	ddm         *DragDropManager
	browser     *FileBrowser
}

// NewDropZone creates a new drop zone wrapping the given content.
func NewDropZone(content fyne.CanvasObject, browser *FileBrowser, ddm *DragDropManager) *DropZone {
	dz := &DropZone{
		content:     content,
		highlightBg: canvas.NewRectangle(color.NRGBA{R: 0, G: 150, B: 255, A: 50}),
		ddm:         ddm,
		browser:     browser,
	}
	dz.highlightBg.Hide()
	dz.ExtendBaseWidget(dz)
	return dz
}

// SetOnDrop sets the callback when files are dropped.
func (dz *DropZone) SetOnDrop(fn func()) {
	dz.onDrop = fn
}

// SetHighlighted sets the highlight state.
func (dz *DropZone) SetHighlighted(highlighted bool) {
	dz.highlighted = highlighted
	if highlighted {
		dz.highlightBg.Show()
	} else {
		dz.highlightBg.Hide()
	}
	dz.Refresh()
}

// CreateRenderer implements fyne.Widget.
func (dz *DropZone) CreateRenderer() fyne.WidgetRenderer {
	return &dropZoneRenderer{
		dz:      dz,
		objects: []fyne.CanvasObject{dz.highlightBg, dz.content},
	}
}

// MouseIn implements desktop.Hoverable.
func (dz *DropZone) MouseIn(e *desktop.MouseEvent) {
	if dz.ddm != nil && dz.ddm.IsDragging() && dz.ddm.GetDragSource() != dz.browser {
		dz.SetHighlighted(true)
	}
}

// MouseMoved implements desktop.Hoverable.
func (dz *DropZone) MouseMoved(e *desktop.MouseEvent) {}

// MouseOut implements desktop.Hoverable.
func (dz *DropZone) MouseOut() {
	dz.SetHighlighted(false)
}

// Tapped implements fyne.Tappable - handles drop on tap up.
func (dz *DropZone) Tapped(e *fyne.PointEvent) {
	if dz.ddm != nil && dz.ddm.IsDragging() && dz.ddm.GetDragSource() != dz.browser {
		dz.ddm.Drop(dz.browser)
		if dz.onDrop != nil {
			dz.onDrop()
		}
	}
}

type dropZoneRenderer struct {
	dz      *DropZone
	objects []fyne.CanvasObject
}

func (r *dropZoneRenderer) Layout(size fyne.Size) {
	r.dz.highlightBg.Resize(size)
	r.dz.content.Resize(size)
}

func (r *dropZoneRenderer) MinSize() fyne.Size {
	return r.dz.content.MinSize()
}

func (r *dropZoneRenderer) Refresh() {
	r.dz.highlightBg.Refresh()
	r.dz.content.Refresh()
}

func (r *dropZoneRenderer) Objects() []fyne.CanvasObject {
	return r.objects
}

func (r *dropZoneRenderer) Destroy() {}

// DraggableItem represents a draggable list item.
type DraggableItem struct {
	widget.BaseWidget
	content  fyne.CanvasObject
	item     *FileItem
	browser  *FileBrowser
	ddm      *DragDropManager
	dragging bool
	startPos fyne.Position
}

// NewDraggableItem creates a new draggable list item.
func NewDraggableItem(content fyne.CanvasObject, item *FileItem, browser *FileBrowser, ddm *DragDropManager) *DraggableItem {
	di := &DraggableItem{
		content: content,
		item:    item,
		browser: browser,
		ddm:     ddm,
	}
	di.ExtendBaseWidget(di)
	return di
}

// Dragged implements fyne.Draggable.
func (di *DraggableItem) Dragged(e *fyne.DragEvent) {
	if !di.dragging && di.item != nil && !di.item.IsDir && di.item.Name != ".." {
		di.dragging = true
		di.startPos = e.Position

		// Start drag with selected items or just this item
		items := di.browser.GetSelectedItems()
		if len(items) == 0 {
			items = []FileItem{*di.item}
		}
		di.ddm.StartDrag(di.browser, items)
	}
}

// DragEnd implements fyne.Draggable.
func (di *DraggableItem) DragEnd() {
	di.dragging = false
	// Drop is handled by DropZone.Tapped
}

// CreateRenderer implements fyne.Widget.
func (di *DraggableItem) CreateRenderer() fyne.WidgetRenderer {
	return widget.NewSimpleRenderer(di.content)
}
