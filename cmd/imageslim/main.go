// GM TUI — a terminal UI for batch image resize and compression using GraphicsMagick.
//
// Run with: go run .
// Requires the "gm" binary (GraphicsMagick) to be installed:
//
//	brew install graphicsmagick   # macOS
//	apt install graphicsmagick    # Debian/Ubuntu
package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/brunovpinheiro/ImageSlim/internal/gm"
)

// ---------------------------------------------------------------------------
// Application state enum
// ---------------------------------------------------------------------------

// appState represents which screen the TUI is currently showing.
type appState int

const (
	stateForm    appState = iota // Configuration form
	stateRunning                 // GraphicsMagick is running
	stateDone                    // Command completed successfully
	stateError                   // Command failed
)

// ---------------------------------------------------------------------------
// Form focus positions
// ---------------------------------------------------------------------------

// Focus indices for the form screen.  0–2 are the text inputs; 3 is the
// output-mode selector (which uses arrow keys instead of text entry).
const (
	focusDir     = 0
	focusResize  = 1
	focusQuality = 2
	focusMode    = 3
	maxFocus     = 3
)

// ---------------------------------------------------------------------------
// Output mode options
// ---------------------------------------------------------------------------

const (
	modePreserve  = 0
	modeOverwrite = 1
)

var modeLabels = []string{
	"Preserve originals  →  write to output/ folder",
	"Overwrite files in-place  →  gm mogrify",
}

// ---------------------------------------------------------------------------
// Lipgloss styles
// ---------------------------------------------------------------------------

const (
	accentColor  = "#7D56F4"
	successColor = "#04B575"
	errorColor   = "#FF4672"
	mutedColor   = "#626262"
	warnColor    = "#FFAA00"
	dimColor     = "#3D3D3D"
)

var (
	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color(accentColor))

	subtitleStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color(mutedColor))

	labelStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color(mutedColor))

	focusedLabelStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color(accentColor))

	// Left-border highlight for the focused text input.
	focusedInputStyle = lipgloss.NewStyle().
				Border(lipgloss.NormalBorder(), false, false, false, true).
				BorderForeground(lipgloss.Color(accentColor)).
				PaddingLeft(1)

	// Dim left border for blurred text inputs.
	blurredInputStyle = lipgloss.NewStyle().
				Border(lipgloss.NormalBorder(), false, false, false, true).
				BorderForeground(lipgloss.Color(dimColor)).
				PaddingLeft(1)

	selectedModeStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color(accentColor))

	unselectedModeStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color(mutedColor))

	successStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color(successColor))

	errorStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color(errorColor))

	warningStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color(warnColor))

	helpStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color(mutedColor))

	cmdStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color(mutedColor)).
			Italic(true)
)

// ---------------------------------------------------------------------------
// Bubble Tea message types
// ---------------------------------------------------------------------------

// resultMsg carries the gm.Result back to the Update loop once the background
// command finishes.
type resultMsg gm.Result

// ---------------------------------------------------------------------------
// Model
// ---------------------------------------------------------------------------

// model is the single Bubble Tea application model.  It holds state for all
// screens; only the fields relevant to the current appState are meaningful.
type model struct {
	state      appState
	inputs     []textinput.Model // form inputs: dir, resize, quality
	focus      int               // which form element is focused (0–3)
	outputMode int               // 0 = preserve, 1 = overwrite
	result     gm.Result         // populated after command finishes
	spinner    spinner.Model     // animated spinner shown during running state
	viewport   viewport.Model   // scrollable output shown in done/error states
	vpReady    bool              // true once viewport has been initialised
	width      int               // terminal width (updated via WindowSizeMsg)
	height     int               // terminal height (updated via WindowSizeMsg)
	gmFound    bool              // whether "gm" binary was found in PATH
}

// ---------------------------------------------------------------------------
// Initialisation
// ---------------------------------------------------------------------------

// initialModel builds the starting model with sensible defaults.
func initialModel() model {
	// Detect whether the gm binary is installed.
	_, err := exec.LookPath("gm")
	gmFound := err == nil

	// Default base directory: wherever the user opened the terminal.
	defaultDir, err := os.Getwd()
	if err != nil {
		defaultDir = "."
	}

	// --- text inputs ---

	dir := textinput.New()
	dir.Placeholder = "e.g. ~/Pictures or /srv/images"
	dir.SetValue(defaultDir)
	dir.Width = 52
	dir.Focus()

	resize := textinput.New()
	resize.Placeholder = "e.g. 1200x1200"
	resize.SetValue("1200x1200")
	resize.Width = 20

	quality := textinput.New()
	quality.Placeholder = "1–100"
	quality.SetValue("80")
	quality.CharLimit = 3
	quality.Width = 10

	// --- spinner ---

	sp := spinner.New()
	sp.Spinner = spinner.Spinner{
		Frames: []string{"⣾", "⣽", "⣻", "⢿", "⡿", "⣟", "⣯", "⣷"},
		FPS:    time.Second / 10,
	}
	sp.Style = lipgloss.NewStyle().Foreground(lipgloss.Color(accentColor))

	return model{
		state:   stateForm,
		inputs:  []textinput.Model{dir, resize, quality},
		focus:   focusDir,
		spinner: sp,
		gmFound: gmFound,
	}
}

// ---------------------------------------------------------------------------
// Init
// ---------------------------------------------------------------------------

func (m model) Init() tea.Cmd {
	return textinput.Blink
}

// ---------------------------------------------------------------------------
// Update
// ---------------------------------------------------------------------------

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {

	// Keep track of terminal dimensions so the viewport can be sized correctly.
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		if m.vpReady {
			m.viewport.Width = viewportWidth(m.width)
			m.viewport.Height = viewportHeight(m.height)
		}
		return m, nil

	// The background gm command has finished; switch to done or error screen.
	case resultMsg:
		m.result = gm.Result(msg)
		if m.result.Err != nil {
			m.state = stateError
		} else {
			m.state = stateDone
		}
		// Initialise the scrollable viewport with the combined command output.
		content := buildOutputContent(m.result)
		vp := viewport.New(viewportWidth(m.width), viewportHeight(m.height))
		vp.SetContent(content)
		m.viewport = vp
		m.vpReady = true
		return m, nil

	// Spinner tick: keep the spinner running while processing.
	case spinner.TickMsg:
		if m.state == stateRunning {
			var cmd tea.Cmd
			m.spinner, cmd = m.spinner.Update(msg)
			return m, cmd
		}
		return m, nil

	// Key events are routed to the active screen's handler.
	case tea.KeyMsg:
		// Ctrl+C always quits, regardless of which screen is active.
		if msg.Type == tea.KeyCtrlC {
			return m, tea.Quit
		}
		switch m.state {
		case stateForm:
			return m.updateForm(msg)
		case stateRunning:
			return m.updateRunning(msg)
		case stateDone, stateError:
			return m.updateDoneOrError(msg)
		}
	}

	// Forward non-key messages to the viewport so mouse-wheel scrolling works.
	if (m.state == stateDone || m.state == stateError) && m.vpReady {
		var cmd tea.Cmd
		m.viewport, cmd = m.viewport.Update(msg)
		return m, cmd
	}

	return m, nil
}

// updateForm handles key events on the configuration form screen.
func (m model) updateForm(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {

	case tea.KeyEsc:
		return m, tea.Quit

	// Tab / Shift+Tab cycle focus through the four form elements.
	case tea.KeyTab, tea.KeyShiftTab:
		if msg.Type == tea.KeyShiftTab {
			m.focus--
			if m.focus < 0 {
				m.focus = maxFocus
			}
		} else {
			m.focus++
			if m.focus > maxFocus {
				m.focus = 0
			}
		}
		var cmds []tea.Cmd
		for i := range m.inputs {
			if i == m.focus {
				cmds = append(cmds, m.inputs[i].Focus())
			} else {
				m.inputs[i].Blur()
			}
		}
		return m, tea.Batch(cmds...)

	// Enter starts processing from any focus position.
	case tea.KeyEnter:
		m.state = stateRunning
		return m, tea.Batch(
			runCmd(m.buildOptions()),
			m.spinner.Tick,
		)

	// Arrow keys change the output mode when the mode selector is focused.
	case tea.KeyUp:
		if m.focus == focusMode && m.outputMode > 0 {
			m.outputMode--
		}
		return m, nil

	case tea.KeyDown:
		if m.focus == focusMode && m.outputMode < len(modeLabels)-1 {
			m.outputMode++
		}
		return m, nil

	case tea.KeyRunes:
		// 'q' quits only when the mode selector is focused, because text
		// inputs capture all rune keys for normal editing.
		if string(msg.Runes) == "q" && m.focus == focusMode {
			return m, tea.Quit
		}
	}

	// All other key events go to the currently focused text input.
	if m.focus < focusMode {
		var cmd tea.Cmd
		m.inputs[m.focus], cmd = m.inputs[m.focus].Update(msg)
		return m, cmd
	}

	return m, nil
}

// updateRunning handles key events while GraphicsMagick is processing.
// The user can only quit; all other input is ignored.
func (m model) updateRunning(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if msg.String() == "q" || msg.Type == tea.KeyEsc {
		return m, tea.Quit
	}
	return m, nil
}

// updateDoneOrError handles key events on the done and error screens.
func (m model) updateDoneOrError(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEsc, tea.KeyEnter:
		return m, tea.Quit
	case tea.KeyRunes:
		switch string(msg.Runes) {
		case "q":
			return m, tea.Quit
		case "r":
			// Return to the form so the user can run another job.
			nm := initialModel()
			nm.width, nm.height = m.width, m.height
			return nm, textinput.Blink
		}
	}
	// Forward other keys to the viewport (arrow keys, page-up/down, etc.).
	var cmd tea.Cmd
	m.viewport, cmd = m.viewport.Update(msg)
	return m, cmd
}

// ---------------------------------------------------------------------------
// View
// ---------------------------------------------------------------------------

func (m model) View() string {
	switch m.state {
	case stateForm:
		return m.viewForm()
	case stateRunning:
		return m.viewRunning()
	case stateDone:
		return m.viewDone()
	case stateError:
		return m.viewError()
	}
	return ""
}

// viewForm renders the configuration screen.
func (m model) viewForm() string {
	var b strings.Builder

	b.WriteString(titleStyle.Render("GM TUI — Batch Image Resize & Compress"))
	b.WriteString("\n")
	b.WriteString(subtitleStyle.Render("Powered by GraphicsMagick"))
	b.WriteString("\n\n")

	// Show a warning banner if gm is not installed.
	if !m.gmFound {
		b.WriteString(warningStyle.Render("⚠  'gm' not found in PATH — install GraphicsMagick first"))
		b.WriteString("\n")
		b.WriteString(warningStyle.Render("   macOS: brew install graphicsmagick"))
		b.WriteString("\n\n")
	}

	b.WriteString(m.renderTextField(focusDir, "Base directory"))
	b.WriteString("\n\n")
	b.WriteString(m.renderTextField(focusResize, "Resize  (W×H)"))
	b.WriteString("\n\n")
	b.WriteString(m.renderTextField(focusQuality, "JPEG quality  (1–100)"))
	b.WriteString("\n\n")
	b.WriteString(m.renderModeSelector())
	b.WriteString("\n")
	b.WriteString(helpStyle.Render("[Tab] next field   [↑↓] change mode   [Enter] run   [Ctrl+C / q] quit"))

	return b.String()
}

// renderTextField renders a labelled text input, highlighting it when focused.
func (m model) renderTextField(idx int, label string) string {
	var b strings.Builder

	lbl := labelStyle.Render(label)
	if m.focus == idx {
		lbl = focusedLabelStyle.Render(label)
	}
	b.WriteString(lbl)
	b.WriteString("\n")

	inp := m.inputs[idx].View()
	if m.focus == idx {
		b.WriteString(focusedInputStyle.Render(inp))
	} else {
		b.WriteString(blurredInputStyle.Render(inp))
	}

	return b.String()
}

// renderModeSelector renders the output-mode radio buttons.
func (m model) renderModeSelector() string {
	var b strings.Builder

	lbl := labelStyle.Render("Output mode")
	if m.focus == focusMode {
		lbl = focusedLabelStyle.Render("Output mode")
	}
	b.WriteString(lbl)
	b.WriteString("\n")

	for i, label := range modeLabels {
		radio := "○"
		if i == m.outputMode {
			radio = "●"
		}
		line := fmt.Sprintf("  %s  %s", radio, label)

		if m.focus == focusMode {
			if i == m.outputMode {
				b.WriteString(selectedModeStyle.Render(line))
			} else {
				b.WriteString(unselectedModeStyle.Render(line))
			}
		} else {
			if i == m.outputMode {
				b.WriteString(lipgloss.NewStyle().Bold(true).Render(line))
			} else {
				b.WriteString(unselectedModeStyle.Render(line))
			}
		}
		b.WriteString("\n")
	}

	return b.String()
}

// viewRunning renders the "processing" screen with a live spinner.
func (m model) viewRunning() string {
	var b strings.Builder

	b.WriteString(titleStyle.Render("Processing…"))
	b.WriteString("\n\n")
	b.WriteString(m.spinner.View())
	b.WriteString("  ")
	b.WriteString(subtitleStyle.Render("Running GraphicsMagick — please wait…"))
	b.WriteString("\n\n")
	b.WriteString(helpStyle.Render("[q / Ctrl+C] cancel"))

	return b.String()
}

// viewDone renders the success screen with scrollable command output.
func (m model) viewDone() string {
	var b strings.Builder

	b.WriteString(successStyle.Render("✓  Done!"))
	b.WriteString("\n")
	b.WriteString(subtitleStyle.Render("All files processed successfully."))
	b.WriteString("\n\n")

	if m.vpReady {
		b.WriteString(m.viewport.View())
		b.WriteString("\n")
		b.WriteString(helpStyle.Render(scrollHint(m.viewport)))
		b.WriteString("\n")
	}

	b.WriteString(helpStyle.Render("[r] run again   [Enter / q] quit"))

	return b.String()
}

// viewError renders the error screen with scrollable command output.
func (m model) viewError() string {
	var b strings.Builder

	b.WriteString(errorStyle.Render("✗  Error"))
	b.WriteString("\n")
	if m.result.Err != nil {
		b.WriteString(subtitleStyle.Render(m.result.Err.Error()))
	}
	b.WriteString("\n\n")

	if m.vpReady {
		b.WriteString(m.viewport.View())
		b.WriteString("\n")
		b.WriteString(helpStyle.Render(scrollHint(m.viewport)))
		b.WriteString("\n")
	}

	b.WriteString(helpStyle.Render("[r] try again   [Enter / q] quit"))

	return b.String()
}

// scrollHint returns a "X% scrolled" hint when the viewport has overflow.
func scrollHint(vp viewport.Model) string {
	if vp.AtBottom() && vp.AtTop() {
		return "" // all content fits, no hint needed
	}
	return fmt.Sprintf("↑↓ / PgUp PgDn to scroll   %d%%", int(vp.ScrollPercent()*100))
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// buildOptions assembles a gm.Options from the current form field values.
// Invalid or empty fields fall back to their defaults.
func (m model) buildOptions() gm.Options {
	dir := expandHome(strings.TrimSpace(m.inputs[focusDir].Value()))
	if dir == "" {
		dir = "."
	}

	resize := strings.TrimSpace(m.inputs[focusResize].Value())
	if resize == "" {
		resize = "1200x1200"
	}

	quality, err := strconv.Atoi(strings.TrimSpace(m.inputs[focusQuality].Value()))
	if err != nil || quality < 1 || quality > 100 {
		quality = 80
	}

	return gm.Options{
		Dir:       dir,
		Pattern:   "*.jpg",
		Resize:    resize,
		Quality:   quality,
		Overwrite: m.outputMode == modeOverwrite,
	}
}

// expandHome replaces a leading "~/" with the user's actual home directory.
func expandHome(path string) string {
	if strings.HasPrefix(path, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, path[2:])
		}
	}
	return path
}

// runCmd returns a Bubble Tea command that executes gm.Run in a goroutine and
// sends the result back to the Update loop as a resultMsg.
func runCmd(opts gm.Options) tea.Cmd {
	return func() tea.Msg {
		return resultMsg(gm.Run(opts))
	}
}

// buildOutputContent formats the gm.Result for display inside the viewport.
func buildOutputContent(result gm.Result) string {
	var b strings.Builder

	b.WriteString(cmdStyle.Render(result.Command))
	b.WriteString("\n\n")

	if strings.TrimSpace(result.Output) != "" {
		b.WriteString(result.Output)
	} else {
		b.WriteString(subtitleStyle.Render("(no output)"))
	}

	return b.String()
}

// viewportWidth returns the content width for the viewport, leaving a small
// margin so borders and padding don't cause wrapping artefacts.
func viewportWidth(termWidth int) int {
	w := termWidth - 4
	if w < 40 {
		return 40
	}
	return w
}

// viewportHeight returns the number of visible lines for the viewport,
// reserving space for the surrounding UI elements (title, help text, etc.).
func viewportHeight(termHeight int) int {
	h := termHeight - 10
	if h < 5 {
		return 5
	}
	return h
}

// ---------------------------------------------------------------------------
// Entry point
// ---------------------------------------------------------------------------

func main() {
	// tea.WithAltScreen() takes over the full terminal and restores it on exit.
	p := tea.NewProgram(initialModel(), tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error running gm-tui: %v\n", err)
		os.Exit(1)
	}
}
