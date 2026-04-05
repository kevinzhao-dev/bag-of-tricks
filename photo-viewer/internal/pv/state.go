package pv

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type gridConfig struct {
	Rows, Cols int
}

var gridConfigs = []gridConfig{
	{1, 1}, // level 0: 1 image
	{1, 2}, // level 1: 2 images
	{2, 2}, // level 2: 4 images
	{2, 3}, // level 3: 6 images
	{3, 3}, // level 4: 9 images
	{3, 4}, // level 5: 12 images
	{4, 4}, // level 6: 16 images
}

// PaneState tracks which image a single pane is showing.
type PaneState struct {
	ImageIndex int
	ChunkStart int // speed mode: first image index (inclusive)
	ChunkEnd   int // speed mode: last image index (exclusive)
}

// ViewerState holds the full viewer state.
type ViewerState struct {
	mu          sync.RWMutex
	images      []string
	dirStarts   []int    // indices where each directory group begins
	dirNames    []string // directory display names for each group
	gridLevel   int
	focusedPane int
	panes       []PaneState
	speedMode   bool
}

func NewViewerState(images []string) *ViewerState {
	dirStarts, dirNames := buildDirBoundaries(images)
	vs := &ViewerState{
		images:    images,
		dirStarts: dirStarts,
		dirNames:  dirNames,
		gridLevel: 0,
		panes:     []PaneState{{ImageIndex: 0}},
	}
	return vs
}

// buildDirBoundaries computes directory group boundaries from the sorted image list.
func buildDirBoundaries(images []string) (starts []int, names []string) {
	if len(images) == 0 {
		return nil, nil
	}
	prevDir := ""
	for i, img := range images {
		dir := filepath.Dir(img)
		if dir != prevDir {
			starts = append(starts, i)
			names = append(names, filepath.Base(dir))
			prevDir = dir
		}
	}
	return
}

// dirNameForIndex returns the directory display name for a given image index.
func (vs *ViewerState) dirNameForIndex(idx int) string {
	for i := len(vs.dirStarts) - 1; i >= 0; i-- {
		if idx >= vs.dirStarts[i] {
			return vs.dirNames[i]
		}
	}
	return ""
}

func (vs *ViewerState) clampIndex(idx int) int {
	n := len(vs.images)
	if n == 0 {
		return 0
	}
	if idx < 0 {
		return 0
	}
	if idx >= n {
		return n - 1
	}
	return idx
}

func (vs *ViewerState) paneCount() int {
	g := gridConfigs[vs.gridLevel]
	return g.Rows * g.Cols
}

// clampPane clamps an index within a pane's allowed range.
// In speed mode, respects chunk bounds; otherwise uses global range.
func (vs *ViewerState) clampPane(p *PaneState, idx int) int {
	if vs.speedMode {
		if idx < p.ChunkStart {
			return p.ChunkStart
		}
		if idx >= p.ChunkEnd {
			return p.ChunkEnd - 1
		}
		return idx
	}
	return vs.clampIndex(idx)
}

// NextInPane advances the focused pane's image.
func (vs *ViewerState) NextInPane() {
	vs.mu.Lock()
	defer vs.mu.Unlock()
	p := &vs.panes[vs.focusedPane]
	p.ImageIndex = vs.clampPane(p, p.ImageIndex+1)
}

// PrevInPane goes back one image in the focused pane.
func (vs *ViewerState) PrevInPane() {
	vs.mu.Lock()
	defer vs.mu.Unlock()
	p := &vs.panes[vs.focusedPane]
	p.ImageIndex = vs.clampPane(p, p.ImageIndex-1)
}

// JumpInPane jumps by delta images in the focused pane.
func (vs *ViewerState) JumpInPane(delta int) {
	vs.mu.Lock()
	defer vs.mu.Unlock()
	p := &vs.panes[vs.focusedPane]
	p.ImageIndex = vs.clampPane(p, p.ImageIndex+delta)
}

// GotoInPane sets the focused pane to a specific image index.
func (vs *ViewerState) GotoInPane(index int) {
	vs.mu.Lock()
	defer vs.mu.Unlock()
	p := &vs.panes[vs.focusedPane]
	p.ImageIndex = vs.clampPane(p, index)
}

// GridUp increases the grid level (more panes). Exits speed mode.
func (vs *ViewerState) GridUp() {
	vs.mu.Lock()
	defer vs.mu.Unlock()
	if vs.gridLevel >= len(gridConfigs)-1 {
		return
	}
	vs.speedMode = false
	vs.gridLevel++
	vs.adjustPanes()
}

// GridDown decreases the grid level (fewer panes). Exits speed mode.
func (vs *ViewerState) GridDown() {
	vs.mu.Lock()
	defer vs.mu.Unlock()
	if vs.gridLevel <= 0 {
		return
	}
	vs.speedMode = false
	vs.gridLevel--
	vs.adjustPanes()
}

// adjustPanes grows or shrinks the pane list to match the current grid.
// New panes get sequential images from the last pane's position.
func (vs *ViewerState) adjustPanes() {
	need := vs.paneCount()
	for len(vs.panes) < need {
		lastIdx := vs.panes[len(vs.panes)-1].ImageIndex
		nextIdx := vs.clampIndex(lastIdx + 1)
		vs.panes = append(vs.panes, PaneState{ImageIndex: nextIdx})
	}
	if len(vs.panes) > need {
		vs.panes = vs.panes[:need]
	}
	if vs.focusedPane >= need {
		vs.focusedPane = need - 1
	}
}

// NextDir jumps the focused pane to the first image in the next directory.
func (vs *ViewerState) NextDir() {
	vs.mu.Lock()
	defer vs.mu.Unlock()
	p := &vs.panes[vs.focusedPane]
	for _, start := range vs.dirStarts {
		if start > p.ImageIndex {
			p.ImageIndex = start
			return
		}
	}
	// Already in last dir, stay at last image
}

// PrevDir jumps the focused pane to the first image in the previous directory.
func (vs *ViewerState) PrevDir() {
	vs.mu.Lock()
	defer vs.mu.Unlock()
	p := &vs.panes[vs.focusedPane]
	for i := len(vs.dirStarts) - 1; i >= 0; i-- {
		if vs.dirStarts[i] < p.ImageIndex {
			p.ImageIndex = vs.dirStarts[i]
			return
		}
	}
}

// AdvanceAll advances every pane by 1 image within its allowed range.
func (vs *ViewerState) AdvanceAll() {
	vs.mu.Lock()
	defer vs.mu.Unlock()
	for i := range vs.panes {
		p := &vs.panes[i]
		p.ImageIndex = vs.clampPane(p, p.ImageIndex+1)
	}
}

// RetreatAll goes back 1 image in every pane within its allowed range.
func (vs *ViewerState) RetreatAll() {
	vs.mu.Lock()
	defer vs.mu.Unlock()
	for i := range vs.panes {
		p := &vs.panes[i]
		p.ImageIndex = vs.clampPane(p, p.ImageIndex-1)
	}
}

// EnterSpeedMode splits images evenly across current panes.
// Each pane gets its own chunk to browse independently.
func (vs *ViewerState) EnterSpeedMode() {
	vs.mu.Lock()
	defer vs.mu.Unlock()
	if vs.speedMode {
		return
	}
	n := len(vs.images)
	numPanes := len(vs.panes)
	if numPanes <= 1 || n == 0 {
		return // speed mode only makes sense with multiple panes
	}
	vs.speedMode = true
	chunkSize := n / numPanes
	for i := range vs.panes {
		start := i * chunkSize
		end := (i + 1) * chunkSize
		if i == numPanes-1 {
			end = n // last pane gets remainder
		}
		vs.panes[i] = PaneState{
			ImageIndex: start,
			ChunkStart: start,
			ChunkEnd:   end,
		}
	}
}

// ExitSpeedMode returns to compare mode. Panes keep their current positions.
func (vs *ViewerState) ExitSpeedMode() {
	vs.mu.Lock()
	defer vs.mu.Unlock()
	vs.speedMode = false
	// Clear chunk bounds
	for i := range vs.panes {
		vs.panes[i].ChunkStart = 0
		vs.panes[i].ChunkEnd = 0
	}
}

// SetFocus sets focus to a specific pane (0-based).
func (vs *ViewerState) SetFocus(pane int) {
	vs.mu.Lock()
	defer vs.mu.Unlock()
	if pane >= 0 && pane < len(vs.panes) {
		vs.focusedPane = pane
	}
}

// FocusNext cycles focus to the next pane.
func (vs *ViewerState) FocusNext() {
	vs.mu.Lock()
	defer vs.mu.Unlock()
	vs.focusedPane = (vs.focusedPane + 1) % len(vs.panes)
}

// FocusPrev cycles focus to the previous pane.
func (vs *ViewerState) FocusPrev() {
	vs.mu.Lock()
	defer vs.mu.Unlock()
	vs.focusedPane = (vs.focusedPane - 1 + len(vs.panes)) % len(vs.panes)
}

// PaneSnapshot is the JSON-serializable state for one pane.
type PaneSnapshot struct {
	ImageURL   string `json:"image_url"`
	ImageIndex int    `json:"image_index"`
	ImageName  string `json:"image_name"`
	ImageDate  string `json:"image_date"`
	DirName    string `json:"dir_name"`
	Focused    bool   `json:"focused"`
	ChunkStart int    `json:"chunk_start"` // speed mode chunk range
	ChunkEnd   int    `json:"chunk_end"`
	ChunkDone  bool   `json:"chunk_done"` // reached end of chunk
}

// StateSnapshot is the full JSON-serializable state sent to the browser.
type StateSnapshot struct {
	GridRows    int            `json:"grid_rows"`
	GridCols    int            `json:"grid_cols"`
	Panes       []PaneSnapshot `json:"panes"`
	TotalImages int            `json:"total_images"`
	FocusedPane int            `json:"focused_pane"`
	DirCount    int            `json:"dir_count"`
	SpeedMode   bool           `json:"speed_mode"`
}

// Snapshot returns a serializable copy of the current state.
func (vs *ViewerState) Snapshot() StateSnapshot {
	vs.mu.RLock()
	defer vs.mu.RUnlock()
	g := gridConfigs[vs.gridLevel]
	snap := StateSnapshot{
		GridRows:    g.Rows,
		GridCols:    g.Cols,
		TotalImages: len(vs.images),
		FocusedPane: vs.focusedPane,
		DirCount:    len(vs.dirStarts),
		SpeedMode:   vs.speedMode,
	}
	for i, p := range vs.panes {
		name := ""
		dateStr := ""
		if p.ImageIndex < len(vs.images) {
			imgPath := vs.images[p.ImageIndex]
			name = filepath.Base(imgPath)
			if info, err := os.Stat(imgPath); err == nil {
				dateStr = info.ModTime().Format(time.DateTime)
			}
		}
		chunkDone := vs.speedMode && p.ImageIndex >= p.ChunkEnd-1
		snap.Panes = append(snap.Panes, PaneSnapshot{
			ImageURL:   fmt.Sprintf("/image?idx=%d", p.ImageIndex),
			ImageIndex: p.ImageIndex,
			ImageName:  name,
			ImageDate:  dateStr,
			DirName:    vs.dirNameForIndex(p.ImageIndex),
			Focused:    i == vs.focusedPane,
			ChunkStart: p.ChunkStart,
			ChunkEnd:   p.ChunkEnd,
			ChunkDone:  chunkDone,
		})
	}
	return snap
}
