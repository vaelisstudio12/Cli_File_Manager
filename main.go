package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/progress"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

const (
	ModeNormal = iota
	ModeRename
	ModeNewFolder
	ModeNewFile
	ModeEditor
	ModeSearch
)

const (
	OpNormal = iota
	OpUndo
	OpRedo
)

type UndoAction struct {
	ActionType string
	Source     string
	Target     string
	IsFolder   bool
}

type FileItem struct {
	Name  string
	IsDir bool
	Size  int64
}

type Panel struct {
	Path          string
	AllItems      []FileItem
	FilteredItems []FileItem
	Cursor        int
	ScrollOffset  int
	SearchQuery   string
}

type progressTickMsg float64
type asyncOpCompleteMsg struct {
	action UndoAction
	err    error
	opType int
}

type model struct {
	width  int
	height int

	mode        int
	activePanel int
	left        Panel
	right       Panel

	inputRunes  []rune
	inputCursor int

	editContent []rune
	editCursor  int
	editScrollX int
	editScrollY int

	message   string
	undoStack []UndoAction
	redoStack []UndoAction

	isOperating  bool
	progressChan chan float64
	progressBar  progress.Model
}

func formatSize(b int64) string {
	if b < 1024 {
		return fmt.Sprintf("%d B", b)
	}
	if b < 1024*1024 {
		return fmt.Sprintf("%.1f KB", float64(b)/1024)
	}
	if b < 1024*1024*1024 {
		return fmt.Sprintf("%.1f MB", float64(b)/1024/1024)
	}
	return fmt.Sprintf("%.1f GB", float64(b)/1024/1024/1024)
}

func getPathSize(path string) int64 {
	var size int64
	filepath.Walk(path, func(_ string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() {
			size += info.Size()
		}
		return nil
	})
	return size
}

func copyRecursiveAsync(srcPath, dstPath string, totalBytes int64, currentBytes *int64, progChan chan float64) error {
	info, err := os.Stat(srcPath)
	if err != nil {
		return err
	}

	if info.IsDir() {
		os.MkdirAll(dstPath, info.Mode())
		entries, err := os.ReadDir(srcPath)
		if err != nil {
			return err
		}
		for _, entry := range entries {
			err := copyRecursiveAsync(filepath.Join(srcPath, entry.Name()), filepath.Join(dstPath, entry.Name()), totalBytes, currentBytes, progChan)
			if err != nil {
				return err
			}
		}
	} else {
		src, err := os.Open(srcPath)
		if err != nil {
			return err
		}
		defer src.Close()
		dst, err := os.Create(dstPath)
		if err != nil {
			return err
		}
		defer dst.Close()

		buf := make([]byte, 32*1024)
		for {
			n, readErr := src.Read(buf)
			if n > 0 {
				_, writeErr := dst.Write(buf[:n])
				if writeErr != nil {
					return writeErr
				}
				*currentBytes += int64(n)
				if totalBytes > 0 && progChan != nil {
					progChan <- float64(*currentBytes) / float64(totalBytes)
				}
			}
			if readErr == io.EOF {
				break
			}
			if readErr != nil {
				return readErr
			}
		}
	}
	return nil
}

func safeMoveAsync(src, dst string, progChan chan float64) error {
	err := os.Rename(src, dst)
	if err == nil {
		if progChan != nil {
			progChan <- 1.0
		}
		return nil
	}

	total := getPathSize(src)
	var current int64 = 0
	err = copyRecursiveAsync(src, dst, total, &current, progChan)
	if err != nil {
		return err
	}
	return os.RemoveAll(src)
}

func readDir(path string) ([]FileItem, error) {
	entries, err := os.ReadDir(path)
	var list []FileItem
	list = append(list, FileItem{Name: "..", IsDir: true})

	if err != nil {
		return list, err
	}

	for _, v := range entries {
		info, err := v.Info()
		size := int64(0)
		if err == nil && !v.IsDir() {
			size = info.Size()
		}
		list = append(list, FileItem{Name: v.Name(), IsDir: v.IsDir(), Size: size})
	}

	return list, nil
}

func (p *Panel) applyFilter() {
	if p.SearchQuery == "" {
		p.FilteredItems = p.AllItems
	} else {
		var list []FileItem
		query := strings.ToLower(p.SearchQuery)
		for _, item := range p.AllItems {
			if item.Name == ".." || strings.Contains(strings.ToLower(item.Name), query) {
				list = append(list, item)
			}
		}
		p.FilteredItems = list
	}

	if p.Cursor >= len(p.FilteredItems) {
		p.Cursor = max(0, len(p.FilteredItems)-1)
	}
	p.ScrollOffset = 0
}

func (p *Panel) reload() error {
	items, err := readDir(p.Path)
	p.AllItems = items
	p.applyFilter()
	return err
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func initialModel() model {
	cwd, _ := os.Getwd()
	leftPanel := Panel{Path: cwd, SearchQuery: ""}
	leftErr := leftPanel.reload()

	rightPanel := Panel{Path: cwd, SearchQuery: ""}
	rightPanel.reload()

	pg := progress.New(progress.WithScaledGradient("#FF7CCB", "#FDFF8C"))

	msg := "Ready. Press F3/Tab to switch panels | Ctrl+F to Search"
	if leftErr != nil {
		msg = "Error reading directory: " + leftErr.Error()
	}

	return model{
		mode:        ModeNormal,
		activePanel: 0,
		left:        leftPanel,
		right:       rightPanel,
		message:     msg,
		undoStack:   []UndoAction{},
		redoStack:   []UndoAction{},
		progressBar: pg,
	}
}

func (m model) Init() tea.Cmd { return nil }

func (m *model) listenProgress() tea.Cmd {
	return func() tea.Msg {
		if m.progressChan == nil {
			return nil
		}
		val, ok := <-m.progressChan
		if !ok {
			return nil
		}
		return progressTickMsg(val)
	}
}

func (m *model) runAsyncOp(action UndoAction, sysOp string, opType int) tea.Cmd {
	m.isOperating = true
	m.progressChan = make(chan float64)

	c := m.progressChan

	return tea.Batch(
		m.listenProgress(),
		func() tea.Msg {
			defer close(c)
			var err error

			switch sysOp {
			case "COPY":
				total := getPathSize(action.Source)
				var current int64 = 0
				err = copyRecursiveAsync(action.Source, action.Target, total, &current, c)
			case "MOVE", "DELETE", "RENAME":
				err = safeMoveAsync(action.Source, action.Target, c)
			case "REVERSE_MOVE":
				err = safeMoveAsync(action.Target, action.Source, c)
			}

			return asyncOpCompleteMsg{action: action, err: err, opType: opType}
		},
	)
}

func (p *Panel) adjustScroll(panelHeight int) {
	visibleItems := panelHeight - 4
	if visibleItems < 1 {
		visibleItems = 1
	}

	if p.Cursor < p.ScrollOffset {
		p.ScrollOffset = p.Cursor
	} else if p.Cursor >= p.ScrollOffset+visibleItems {
		p.ScrollOffset = p.Cursor - visibleItems + 1
	}
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.progressBar.Width = max(10, int(float64(m.width)*0.40)-6)

	case progressTickMsg:
		cmd := m.progressBar.SetPercent(float64(msg))
		return m, tea.Batch(cmd, m.listenProgress())

	case asyncOpCompleteMsg:
		m.isOperating = false
		m.progressChan = nil

		errL := m.left.reload()
		errR := m.right.reload()

		if msg.err != nil {
			m.message = "Operation failed: " + msg.err.Error()
		} else if errL != nil || errR != nil {
			m.message = "Operation completed, but failed to reload directories."
		} else {
			if msg.opType == OpNormal {
				m.undoStack = append(m.undoStack, msg.action)
				m.redoStack = nil
				m.message = "Operation completed successfully."
			} else if msg.opType == OpUndo {
				m.redoStack = append(m.redoStack, msg.action)
				m.message = "Undo completed."
			} else if msg.opType == OpRedo {
				m.undoStack = append(m.undoStack, msg.action)
				m.message = "Redo completed."
			}
		}
		return m, nil

	case tea.KeyMsg:
		if m.isOperating {
			return m, nil
		}

		active := &m.left
		passive := &m.right
		if m.activePanel == 1 {
			active = &m.right
			passive = &m.left
		}

		if m.mode == ModeEditor {
			getRowCol := func() (int, int) {
				r, c := 0, 0
				for i := 0; i < m.editCursor; i++ {
					if m.editContent[i] == '\n' {
						r++
						c = 0
					} else {
						c++
					}
				}
				return r, c
			}

			switch msg.String() {
			case "esc":
				if m.mode == ModeSearch {
					active.SearchQuery = ""
					active.applyFilter()
				}
				m.mode = ModeNormal
			case "ctrl+s":
				selected := active.FilteredItems[active.Cursor]
				err := os.WriteFile(filepath.Join(active.Path, selected.Name), []byte(string(m.editContent)), 0644)
				m.mode = ModeNormal
				if err != nil {
					m.message = "Failed to save: " + err.Error()
				} else {
					m.message = "File saved successfully."
				}
			case "left":
				if m.editCursor > 0 {
					m.editCursor--
				}
			case "right":
				if m.editCursor < len(m.editContent) {
					m.editCursor++
				}
			case "up":
				r, c := getRowCol()
				if r > 0 {
					currStart := m.editCursor - c
					prevNL := currStart - 1

					prevStart := prevNL
					for prevStart > 0 && m.editContent[prevStart-1] != '\n' {
						prevStart--
					}

					prevLen := prevNL - prevStart
					newCol := c
					if newCol > prevLen {
						newCol = prevLen
					}
					m.editCursor = prevStart + newCol
				}
			case "down":
				_, c := getRowCol() // r kullanılmadığı için _ yaptık
				nextNL := m.editCursor
				for nextNL < len(m.editContent) && m.editContent[nextNL] != '\n' {
					nextNL++
				}

				if nextNL < len(m.editContent) {
					nextStart := nextNL + 1
					nextEnd := nextStart
					for nextEnd < len(m.editContent) && m.editContent[nextEnd] != '\n' {
						nextEnd++
					}

					nextLen := nextEnd - nextStart
					newCol := c
					if newCol > nextLen {
						newCol = nextLen
					}
					m.editCursor = nextStart + newCol
				}
			case "backspace":
				if m.editCursor > 0 {
					leftPart := append([]rune{}, m.editContent[:m.editCursor-1]...)
					rightPart := m.editContent[m.editCursor:]
					m.editContent = append(leftPart, rightPart...)
					m.editCursor--
				}
			case "enter":
				leftPart := append([]rune{}, m.editContent[:m.editCursor]...)
				rightPart := m.editContent[m.editCursor:]
				m.editContent = append(append(leftPart, '\n'), rightPart...)
				m.editCursor++
			default:
				if msg.Type == tea.KeyRunes || msg.Type == tea.KeySpace {
					newChars := []rune(msg.String())
					leftPart := append([]rune{}, m.editContent[:m.editCursor]...)
					rightPart := m.editContent[m.editCursor:]
					m.editContent = append(append(leftPart, newChars...), rightPart...)
					m.editCursor += len(newChars)
				}
			}

			wInfoText := int(float64(m.width)*0.40) - 4
			hInfoText := m.height - 9

			if wInfoText < 1 {
				wInfoText = 1
			}
			if hInfoText < 1 {
				hInfoText = 1
			}

			r, c := getRowCol()

			if r < m.editScrollY {
				m.editScrollY = r
			} else if r >= m.editScrollY+hInfoText {
				m.editScrollY = r - hInfoText + 1
			}

			if c < m.editScrollX {
				m.editScrollX = c
			} else if c >= m.editScrollX+wInfoText {
				m.editScrollX = c - wInfoText + 1
			}

			return m, nil
		}

		if m.mode == ModeRename || m.mode == ModeNewFolder || m.mode == ModeNewFile || m.mode == ModeSearch {
			switch msg.String() {
			case "esc":
				if m.mode == ModeSearch {
					active.SearchQuery = ""
					active.applyFilter()
				}
				m.mode = ModeNormal
			case "left":
				if m.inputCursor > 0 {
					m.inputCursor--
				}
			case "right":
				if m.inputCursor < len(m.inputRunes) {
					m.inputCursor++
				}
			case "backspace":
				if m.inputCursor > 0 {
					leftPart := append([]rune{}, m.inputRunes[:m.inputCursor-1]...)
					rightPart := m.inputRunes[m.inputCursor:]
					m.inputRunes = append(leftPart, rightPart...)
					m.inputCursor--
				}
			case "enter":
				if m.mode == ModeSearch {
					m.mode = ModeNormal
					return m, nil
				}

				if len(m.inputRunes) > 0 {
					name := string(m.inputRunes)
					targetPath := filepath.Join(active.Path, name)

					if m.mode == ModeRename {
						selected := active.FilteredItems[active.Cursor]
						oldPath := filepath.Join(active.Path, selected.Name)
						action := UndoAction{ActionType: "RENAME", Source: oldPath, Target: targetPath, IsFolder: selected.IsDir}
						cmds = append(cmds, m.runAsyncOp(action, "RENAME", OpNormal))
					} else if m.mode == ModeNewFolder {
						err := os.Mkdir(targetPath, 0755)
						if err != nil {
							m.message = "Failed to create folder: " + err.Error()
						} else {
							m.undoStack = append(m.undoStack, UndoAction{ActionType: "NEW_FOLDER", Target: targetPath, IsFolder: true})
							m.redoStack = nil
							m.message = "Folder created."
							active.reload()
							passive.reload()
						}
					} else if m.mode == ModeNewFile {
						err := os.WriteFile(targetPath, []byte(""), 0644)
						if err != nil {
							m.message = "Failed to create file: " + err.Error()
						} else {
							m.undoStack = append(m.undoStack, UndoAction{ActionType: "NEW_FILE", Target: targetPath, IsFolder: false})
							m.redoStack = nil
							m.message = "File created."
							active.reload()
							passive.reload()
						}
					}
				}
				m.mode = ModeNormal

			default:
				if msg.Type == tea.KeyRunes || msg.Type == tea.KeySpace {
					newChars := []rune(msg.String())
					leftPart := append([]rune{}, m.inputRunes[:m.inputCursor]...)
					rightPart := m.inputRunes[m.inputCursor:]
					m.inputRunes = append(append(leftPart, newChars...), rightPart...)
					m.inputCursor += len(newChars)
				}
			}

			if m.mode == ModeSearch {
				active.SearchQuery = string(m.inputRunes)
				active.applyFilter()
			}
			return m, tea.Batch(cmds...)
		}

		switch msg.String() {
		case "ctrl+c":
			return m, tea.Quit
		case "tab", "f3":
			m.activePanel = (m.activePanel + 1) % 2
		case "up", "k":
			if active.Cursor > 0 {
				active.Cursor--
			}
		case "down", "j":
			if active.Cursor < len(active.FilteredItems)-1 {
				active.Cursor++
			}
		case "ctrl+f":
			m.mode = ModeSearch
			m.inputRunes = []rune(active.SearchQuery)
			m.inputCursor = len(m.inputRunes)
			m.message = "Search Mode: Type to filter files (Esc to cancel, Enter to confirm)"

		case "ctrl+b":
			m.mode = ModeNewFile
			m.inputRunes = []rune{}
			m.inputCursor = 0
		case "ctrl+n":
			m.mode = ModeNewFolder
			m.inputRunes = []rune{}
			m.inputCursor = 0

		case "f2":
			if len(active.FilteredItems) > 0 {
				selected := active.FilteredItems[active.Cursor]
				if selected.Name != ".." {
					m.mode = ModeRename
					m.inputRunes = []rune(selected.Name)
					m.inputCursor = len(m.inputRunes)
				}
			}

		case "enter", "right", "l":
			if len(active.FilteredItems) > 0 {
				selected := active.FilteredItems[active.Cursor]
				targetPath := filepath.Join(active.Path, selected.Name)

				if selected.IsDir {
					oldPath := active.Path
					active.Path = filepath.Clean(targetPath)
					err := active.reload()
					if err != nil {
						active.Path = oldPath
						active.reload()
						m.message = "Access denied or error: " + err.Error()
					} else {
						active.SearchQuery = ""
						active.Cursor = 0
						active.ScrollOffset = 0
					}
				} else {
					info, err := os.Stat(targetPath)
					if err == nil && info.Size() < 500*1024 {
						data, readErr := os.ReadFile(targetPath)
						if readErr != nil {
							m.message = "Cannot read file: " + readErr.Error()
						} else {
							m.editContent = []rune(string(data))
							m.editCursor = len(m.editContent)
							m.mode = ModeEditor
						}
					} else {
						m.message = "File too large for internal editor (>500KB) or unreadable."
					}
				}
			}

		case "backspace", "left", "h":
			oldPath := active.Path
			active.Path = filepath.Dir(active.Path)
			err := active.reload()
			if err != nil {
				active.Path = oldPath
				active.reload()
				m.message = "Cannot access parent directory: " + err.Error()
			} else {
				active.SearchQuery = ""
				active.Cursor = 0
				active.ScrollOffset = 0
			}

		case "f5":
			if active.Path == passive.Path {
				m.message = "Error: Cannot copy to the same directory!"
				return m, nil
			}
			if len(active.FilteredItems) > 0 {
				selected := active.FilteredItems[active.Cursor]
				if selected.Name != ".." {
					src := filepath.Join(active.Path, selected.Name)
					dst := filepath.Join(passive.Path, selected.Name)
					action := UndoAction{ActionType: "COPY", Source: src, Target: dst, IsFolder: selected.IsDir}
					cmds = append(cmds, m.runAsyncOp(action, "COPY", OpNormal))
				}
			}

		case "f6":
			if active.Path == passive.Path {
				m.message = "Error: Cannot move to the same directory!"
				return m, nil
			}
			if len(active.FilteredItems) > 0 {
				selected := active.FilteredItems[active.Cursor]
				if selected.Name != ".." {
					src := filepath.Join(active.Path, selected.Name)
					dst := filepath.Join(passive.Path, selected.Name)
					action := UndoAction{ActionType: "MOVE", Source: src, Target: dst, IsFolder: selected.IsDir}
					cmds = append(cmds, m.runAsyncOp(action, "MOVE", OpNormal))
				}
			}

		case "delete":
			if len(active.FilteredItems) > 0 {
				selected := active.FilteredItems[active.Cursor]
				if selected.Name != ".." {
					src := filepath.Join(active.Path, selected.Name)
					trashPath := filepath.Join(os.TempDir(), fmt.Sprintf("trash_%d_%s", time.Now().UnixNano(), selected.Name))
					action := UndoAction{ActionType: "DELETE", Source: src, Target: trashPath, IsFolder: selected.IsDir}
					cmds = append(cmds, m.runAsyncOp(action, "DELETE", OpNormal))
				}
			}

		case "ctrl+z":
			if len(m.undoStack) == 0 {
				m.message = "Nothing to undo."
			} else {
				lastAction := m.undoStack[len(m.undoStack)-1]
				m.undoStack = m.undoStack[:len(m.undoStack)-1]

				switch lastAction.ActionType {
				case "NEW_FOLDER", "NEW_FILE", "COPY":
					os.RemoveAll(lastAction.Target)
					m.message = "Undone: Action reverted."
					m.redoStack = append(m.redoStack, lastAction)
					active.reload()
					passive.reload()
				case "MOVE", "RENAME", "DELETE":
					cmds = append(cmds, m.runAsyncOp(lastAction, "REVERSE_MOVE", OpUndo))
				}
			}

		case "ctrl+y":
			if len(m.redoStack) == 0 {
				m.message = "Nothing to redo."
			} else {
				lastAction := m.redoStack[len(m.redoStack)-1]
				m.redoStack = m.redoStack[:len(m.redoStack)-1]

				switch lastAction.ActionType {
				case "NEW_FOLDER":
					os.MkdirAll(lastAction.Target, 0755)
					m.undoStack = append(m.undoStack, lastAction)
					m.message = "Redo: Folder recreated."
					active.reload()
					passive.reload()
				case "NEW_FILE":
					os.WriteFile(lastAction.Target, []byte(""), 0644)
					m.undoStack = append(m.undoStack, lastAction)
					m.message = "Redo: File recreated."
					active.reload()
					passive.reload()
				case "COPY":
					cmds = append(cmds, m.runAsyncOp(lastAction, "COPY", OpRedo))
				case "MOVE", "RENAME", "DELETE":
					cmds = append(cmds, m.runAsyncOp(lastAction, "MOVE", OpRedo))
				}
			}
		}

		active.adjustScroll(m.height - 4)
	}

	return m, tea.Batch(cmds...)
}

func renderPanel(title string, p Panel, isActive bool, width int, height int) string {
	borderColor := lipgloss.Color("240")
	titleStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Bold(true)
	if isActive {
		borderColor = lipgloss.Color("205")
		titleStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("205")).Bold(true)
	}

	style := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(borderColor).Width(width).Height(height)

	header := fmt.Sprintf(" %s [%s]", titleStyle.Render(title), filepath.Base(p.Path))
	if p.SearchQuery != "" {
		header += lipgloss.NewStyle().Foreground(lipgloss.Color("220")).Render(fmt.Sprintf(" (Search: %s)", p.SearchQuery))
	}
	content := header + "\n\n"

	if len(p.FilteredItems) == 0 {
		content += "  (Empty or no matches)\n"
	}

	visibleHeight := height - 4
	startIdx := p.ScrollOffset
	endIdx := startIdx + visibleHeight

	if endIdx > len(p.FilteredItems) {
		endIdx = len(p.FilteredItems)
	}

	for i := startIdx; i < endIdx; i++ {
		d := p.FilteredItems[i]
		icon := "📄"
		if d.IsDir {
			icon = "📁"
		}

		sizeStr := ""
		if !d.IsDir {
			sizeStr = lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Render(fmt.Sprintf(" %s", formatSize(d.Size)))
		}

		line := fmt.Sprintf("%s %s%s", icon, d.Name, sizeStr)

		if len(line) > width-6 {
			line = line[:width-9] + "..."
		}

		if i == p.Cursor && isActive {
			content += lipgloss.NewStyle().Foreground(lipgloss.Color("0")).Background(lipgloss.Color("205")).Bold(true).Render("> "+line) + "\n"
		} else if i == p.Cursor {
			content += lipgloss.NewStyle().Foreground(lipgloss.Color("205")).Render("* "+line) + "\n"
		} else {
			content += "  " + line + "\n"
		}
	}
	return style.Render(content)
}

func (m model) View() string {
	if m.width == 0 {
		return "Initializing UI..."
	}

	wLeft := int(float64(m.width)*0.30) - 2
	wRight := int(float64(m.width)*0.30) - 2
	wInfo := int(float64(m.width)*0.40) - 2
	h := m.height - 4

	leftView := renderPanel("LEFT PANE", m.left, m.activePanel == 0, wLeft, h)
	rightView := renderPanel("RIGHT PANE", m.right, m.activePanel == 1, wRight, h)

	infoColor := lipgloss.Color("42")
	infoTitle := "=== INFO & LOG ==="
	infoBody := m.message

	if m.isOperating {
		infoColor = lipgloss.Color("220")
		infoTitle = "=== PROCESSING ==="
		infoBody = "Please wait...\n\n" + m.progressBar.View()
	} else if m.mode == ModeEditor {
		infoColor = lipgloss.Color("196")
		infoTitle = "=== TEXT EDITOR ==="

		leftPart := m.editContent[:m.editCursor]
		rightPart := m.editContent[m.editCursor:]

		var contentWithCursor []rune
		contentWithCursor = append(contentWithCursor, leftPart...)
		contentWithCursor = append(contentWithCursor, '█')
		contentWithCursor = append(contentWithCursor, rightPart...)

		lines := strings.Split(string(contentWithCursor), "\n")
		var visibleLines []string

		wInfoText := wInfo - 2
		hInfoText := h - 5

		if wInfoText < 1 {
			wInfoText = 1
		}
		if hInfoText < 1 {
			hInfoText = 1
		}

		startY := m.editScrollY
		endY := startY + hInfoText
		if endY > len(lines) {
			endY = len(lines)
		}

		for i := startY; i < endY; i++ {
			lineRunes := []rune(lines[i])

			if m.editScrollX < len(lineRunes) {
				lineRunes = lineRunes[m.editScrollX:]
			} else {
				lineRunes = []rune{}
			}

			if len(lineRunes) > wInfoText {
				lineRunes = lineRunes[:wInfoText]
			}

			visibleLines = append(visibleLines, string(lineRunes))
		}

		infoBody = strings.Join(visibleLines, "\n")
	}

	infoStyle := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(infoColor).Width(wInfo).Height(h)
	infoView := infoStyle.Render(lipgloss.NewStyle().Bold(true).Foreground(infoColor).Render(infoTitle) + "\n\n" + infoBody)

	footer := ""
	if m.mode == ModeRename || m.mode == ModeNewFolder || m.mode == ModeNewFile || m.mode == ModeSearch {
		leftInput := string(m.inputRunes[:m.inputCursor])
		rightInput := string(m.inputRunes[m.inputCursor:])
		label := "INPUT:"
		if m.mode == ModeSearch {
			label = "SEARCH:"
		}
		footer = fmt.Sprintf("%s %s█%s   (Enter: Confirm | Esc: Cancel)", label, leftInput, rightInput)
	} else if m.mode == ModeNormal {
		footer = "[Tab] Switch | [Ctrl+F] Search | [Ctrl+Z] Undo | [Ctrl+Y] Redo | [F5] Copy | [F6] Move | [Del] Delete"
	} else if m.mode == ModeEditor {
		footer = "[Editor] | [Ctrl+S] Save | [Esc] Exit | [Arrows] Navigate"
	}

	footerStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("0")).Background(lipgloss.Color("42")).Padding(0, 1)

	if m.mode == ModeEditor || m.mode == ModeSearch || m.mode == ModeRename || m.mode == ModeNewFolder || m.mode == ModeNewFile {
		footerStyle = footerStyle.Background(lipgloss.Color("205"))
	}

	return lipgloss.JoinHorizontal(lipgloss.Top, leftView, rightView, infoView) + "\n\n " + footerStyle.Render(footer)
}

func main() {
	p := tea.NewProgram(initialModel(), tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Println("Application Error:", err)
		os.Exit(1)
	}
}
