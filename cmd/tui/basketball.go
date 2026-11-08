package main

import (
	"fmt"
	"math"
	"path/filepath"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

const (
	shotWidth  = 64
	shotHeight = 19
	ballX      = 8
	ballY      = 15
	shotLeft   = 2 // View's left padding.
	shotTop    = 5 // Title, blank line, shot heading, blank line, and top padding.
)

var (
	courtStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("34"))
	hoopStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("208"))
	boardStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("51"))
	netStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	ballStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("214"))
	aimStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("42")).Bold(true)
	pathStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("99"))
)

type shotState struct {
	aimX, aimY int
	x, y       int
	frame      int
	id         int
	dragging   bool
	flying     bool
	scored     bool
	status     string
}

type shotTickMsg int

func newShotState() shotState {
	return shotState{aimX: 42, aimY: 5, x: ballX, y: ballY}
}

func (m model) shotClick(mouse tea.MouseClickMsg) (tea.Model, tea.Cmd) {
	x, y := mouse.X-shotLeft, mouse.Y-shotTop
	if !m.shot.flying && m.fields[0].input.Value() != "" && x >= ballX-1 && x <= ballX+1 && y >= ballY-1 && y <= ballY+1 {
		m.shot.dragging = true
		m.shot.status = ""
	} else if m.fields[0].input.Value() == "" {
		m.shot.status = "Drop or paste a photo path first."
	}
	return m, nil
}

func (m model) shotMotion(mouse tea.MouseMotionMsg) (tea.Model, tea.Cmd) {
	if m.shot.dragging {
		m.shot.aimX = max(0, min(shotWidth-1, mouse.X-shotLeft))
		m.shot.aimY = max(0, min(shotHeight-2, mouse.Y-shotTop))
	}
	return m, nil
}

func (m model) shotRelease(mouse tea.MouseReleaseMsg) (tea.Model, tea.Cmd) {
	if !m.shot.dragging {
		return m, nil
	}
	m.shot.dragging = false
	m.shot.aimX = max(0, min(shotWidth-1, mouse.X-shotLeft))
	m.shot.aimY = max(0, min(shotHeight-2, mouse.Y-shotTop))
	return m.launchShot()
}

func (m model) shotKey(key tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if key.String() == "esc" {
		m.shot.dragging = false
		m.shot.flying = false
		m.screen = actionScreen
		return m, nil
	}
	if m.shot.flying {
		return m, nil
	}
	switch key.String() {
	case "left":
		m.shot.aimX = max(0, m.shot.aimX-1)
	case "right":
		m.shot.aimX = min(shotWidth-1, m.shot.aimX+1)
	case "up":
		m.shot.aimY = max(0, m.shot.aimY-1)
	case "down":
		m.shot.aimY = min(shotHeight-2, m.shot.aimY+1)
	case " ", "space", "enter":
		if m.fields[0].input.Value() == "" {
			m.shot.status = "Drop or paste a photo path first."
			return m, nil
		}
		return m.launchShot()
	}
	return m, nil
}

func (m model) launchShot() (tea.Model, tea.Cmd) {
	if m.fields[0].input.Value() == "" {
		m.shot.status = "Drop or paste a photo path first."
		return m, nil
	}
	m.shot.flying = true
	m.shot.aimX, m.shot.aimY, m.shot.scored = shotTarget(m.shot.aimX, m.shot.aimY)
	m.shot.frame = 0
	m.shot.id++
	m.shot.status = ""
	m.shot.x, m.shot.y = ballX, ballY
	return m, shotDelay(m.shot.id)
}

func shotTarget(x, y int) (int, int, bool) {
	if x >= 43 && x <= 56 && y >= 4 && y <= 10 {
		return 49, 6, true
	}
	return x, y, false
}

func shotDelay(id int) tea.Cmd {
	return tea.Tick(50*time.Millisecond, func(time.Time) tea.Msg { return shotTickMsg(id) })
}

func shotPosition(aimX, aimY, frame int) (int, int) {
	t := float64(frame) / 12
	x := float64(ballX) + float64(aimX-ballX)*t
	y := float64(ballY) + float64(aimY-ballY)*t - 20*t*(1-t)
	return int(math.Round(x)), int(math.Round(y))
}

func (m model) shotTick(msg shotTickMsg) (tea.Model, tea.Cmd) {
	if !m.shot.flying || int(msg) != m.shot.id {
		return m, nil
	}
	m.shot.frame++
	m.shot.x, m.shot.y = shotPosition(m.shot.aimX, m.shot.aimY, m.shot.frame)
	if m.shot.frame > 12 && m.shot.scored {
		m.shot.x = m.shot.aimX
		if m.shot.frame == 15 {
			values, err := m.values()
			if err != nil {
				m.shot.status = err.Error()
				m.shot.flying = false
				return m, nil
			}
			m.screen = uploadingScreen
			return m, tea.Batch(m.spinner.Tick, upload(m.provider, m.profile, values))
		}
	} else if !m.shot.scored && m.shot.y >= 17 {
		m.shot.x, m.shot.y = ballX, ballY
		m.shot.flying = false
		m.shot.status = "Missed. Drag the ball and try again."
		return m, nil
	}
	return m, shotDelay(m.shot.id)
}

func (m model) shotView() string {
	grid := make([][]rune, shotHeight)
	colors := make([][]lipgloss.Style, shotHeight)
	for y := range grid {
		grid[y] = []rune(strings.Repeat(" ", shotWidth))
		colors[y] = make([]lipgloss.Style, shotWidth)
	}
	put := func(x, y int, char rune, style lipgloss.Style) {
		if x >= 0 && x < shotWidth && y >= 0 && y < shotHeight {
			grid[y][x] = char
			colors[y][x] = style
		}
	}
	for x := 0; x < shotWidth; x++ {
		put(x, 17, '─', courtStyle)
	}
	for y := 3; y <= 7; y++ {
		put(53, y, '║', boardStyle)
	}
	for x := 46; x <= 52; x++ {
		put(x, 7, '═', hoopStyle)
	}
	put(47, 8, '╲', netStyle)
	put(51, 8, '╱', netStyle)
	put(48, 9, '╲', netStyle)
	put(50, 9, '╱', netStyle)
	put(49, 10, '∨', netStyle)
	if !m.shot.flying {
		targetX, targetY, onTarget := shotTarget(m.shot.aimX, m.shot.aimY)
		for frame := 2; frame <= 10; frame += 2 {
			x, y := shotPosition(targetX, targetY, frame)
			put(x, y, '·', pathStyle)
		}
		if onTarget {
			put(m.shot.aimX, m.shot.aimY, '*', aimStyle)
		} else {
			put(m.shot.aimX, m.shot.aimY, '+', hoopStyle)
		}
	}
	put(m.shot.x, m.shot.y, '⬤', ballStyle)
	var out strings.Builder
	out.WriteString("Take the shot\n\n")
	for y, row := range grid {
		for x, char := range row {
			out.WriteString(colors[y][x].Render(string(char)))
		}
		out.WriteByte('\n')
	}
	name := m.fields[0].input.Value()
	if name == "" {
		out.WriteString(mutedStyle.Render("Drop a photo here or paste its path to begin\n"))
	} else {
		fmt.Fprintf(&out, "Photo: %.48s\n", filepath.Base(name))
	}
	if m.shot.status != "" {
		out.WriteString(m.shot.status + "\n")
	} else if !m.shot.flying {
		_, _, onTarget := shotTarget(m.shot.aimX, m.shot.aimY)
		if onTarget {
			out.WriteString(successStyle.Render("On target — release to shoot") + "\n")
		}
	}
	out.WriteString(selectedStyle.Render("drop/paste a photo • drag the ball to aim • release to shoot\narrows aim • space shoots • esc back"))
	return out.String()
}
