package cmd

import (
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"
	"github.com/kvzhao/netwatch/internal/ui"
)

var (
	iface       string
	thresholdGB float64
)

var rootCmd = &cobra.Command{
	Use:   "netwatch",
	Short: "Monitor network traffic to avoid HiNet throttling",
	Long:  "A TUI tool that monitors daily network traffic and warns before exceeding ISP thresholds.",
	RunE: func(cmd *cobra.Command, args []string) error {
		m := ui.NewModel(iface, thresholdGB)
		p := tea.NewProgram(m, tea.WithAltScreen())
		if _, err := p.Run(); err != nil {
			return fmt.Errorf("failed to start dashboard: %w", err)
		}
		return nil
	},
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func init() {
	rootCmd.PersistentFlags().StringVar(&iface, "iface", "en0", "Network interface to monitor")
	rootCmd.PersistentFlags().Float64Var(&thresholdGB, "threshold", 150, "Daily traffic threshold in GB")
}
