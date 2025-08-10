package client

import "github.com/charmbracelet/lipgloss"

// UI 样式定义
var (
	statusBarStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FFF7DB")).
			Background(lipgloss.Color("#3C3836")).
			Width(100).
			Padding(0, 1)

	chatHeaderStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FABD2F")).
			Bold(true).
			Padding(0, 1).
			MarginBottom(1)

	messagesStyle = lipgloss.NewStyle().
			Height(20).
			Padding(1, 1, 0, 1)

	myMessageStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#83A598")).
			Align(lipgloss.Right).
			Width(60).
			BorderLeft(true).
			Padding(0, 1).
			BorderForeground(lipgloss.Color("#3c3836"))

	otherMessageStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#FABD2F")).
				Align(lipgloss.Left).
				Width(60).
				BorderLeft(true).
				Padding(0, 1).
				BorderForeground(lipgloss.Color("#3c3836"))

	userListStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#458588")).
			Padding(0, 1)

	userListContainerStyle = lipgloss.NewStyle().
				Width(30).
				Border(lipgloss.RoundedBorder(), true, true, true, true).
				BorderForeground(lipgloss.Color("#458588")).
				Padding(0, 1)

	helpStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#888888")).
			Padding(0, 1).
			MarginTop(1)

	appStyle = lipgloss.NewStyle().
			Margin(1, 2)
)
