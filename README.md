# Go TUI File Manager

A modern terminal-based file manager developed using Go and the Charmbracelet ecosystem, featuring dual panes, asynchronous file operations, an advanced undo/redo mechanism, and a built-in text editor.


![Demo GIF](https://github.com/vaelisstudio12/Cli_File_Manager/blob/main/animation.gif) 
## 🚀 Features

* **Dual-Pane:** Fast switching between left and right panels with independent directory management.
* **Asynchronous Operations:** Background architecture with a `progress bar` to prevent UI freezing during large file copies and moves.
* **Undo/Redo:** Safely revert or repeat file operations (`Ctrl+Z` and `Ctrl+Y`).
* **Built-in Text Editor:** Open, edit, and save text files under 500 KB directly inside the terminal (`Ctrl+S`).
* **Live Search & Filtering:** Instantly search and filter files and folders in the current directory (`Ctrl+F`).

## ⌨️ Keyboard Shortcuts

| Shortcut | Action |
| :--- | :--- |
| `Tab` / `F3` | Switch between panels |
| `↑` / `↓` or `j` / `k` | Navigate up/down in the list |
| `Enter` / `→` / `l` | Open directory or open file in editor |
| `Backspace` / `←` / `h` | Go to parent directory |
| `F2` | Rename selected item |
| `F5` | Copy active item to passive panel |
| `F6` | Move active item to passive panel |
| `Delete` | Delete selected item |
| `Ctrl+B` | Create a new empty file |
| `Ctrl+N` | Create a new folder |
| `Ctrl+F` | Start search mode |
| `Ctrl+Z` | Undo last operation |
| `Ctrl+Y` | Redo undone operation |
| `Ctrl+C` | Quit application |

* **In Editor Mode:** `Ctrl+S` (Save), `Esc` (Exit), `Arrow Keys` (Cursor Movement)
* **In Search/Input Mode:** `Enter` (Confirm), `Esc` (Cancel)

## 🛠️ Installation

Follow these steps to run the project in your local environment:

1. Ensure **Go (1.18 or higher)** is installed on your system.
2. Initialize and download the required dependencies in your project directory:

```bash
go mod init file-manager
go get [github.com/charmbracelet/bubbletea](https://github.com/charmbracelet/bubbletea)
go get [github.com/charmbracelet/bubbles/progress](https://github.com/charmbracelet/bubbles/progress)
go get [github.com/charmbracelet/lipgloss](https://github.com/charmbracelet/lipgloss)
```

## Running

```bash
go run main.go
```

## ☕ Donate
If you like this project, consider buying me a coffee with Monero (XMR):

`45qNiHzBpi83ojK88ppgAS4cQSHRFThqY3JpXaNoQFB8Ap6hK6gFZ64SnTFqajeinjAqff3xjNy918ubRADX53bg2ZDPHUo`