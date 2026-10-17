package main

import "github.com/charmbracelet/lipgloss"

// One Dark Pro Darker palette.
const (
	colBg     = "#23272e"
	colPanel  = "#1e2227"
	colSel    = "#2c313a"
	colFg     = "#abb2bf"
	colMuted  = "#7f848e"
	colGutter = "#495162"
	colRed    = "#e06c75"
	colOrange = "#d19a66"
	colYellow = "#e5c07b"
	colGreen  = "#98c379"
	colCyan   = "#56b6c2"
	colBlue   = "#61afef"
	colPurple = "#c678dd"
)

var (
	bg          = lipgloss.NewStyle().Background(lipgloss.Color(colBg))
	sepStyle    = bg.Foreground(lipgloss.Color(colGutter))
	hintStyle   = bg.Foreground(lipgloss.Color(colMuted))
	errStyle    = bg.Foreground(lipgloss.Color(colRed))
	headerStyle = lipgloss.NewStyle().Background(lipgloss.Color(colPanel)).Foreground(lipgloss.Color(colFg))
	titleStyle  = headerStyle.Foreground(lipgloss.Color(colBlue)).Bold(true)
	mutedHeader = headerStyle.Foreground(lipgloss.Color(colMuted))
	gutterStyle = bg.Foreground(lipgloss.Color(colGutter))
)
