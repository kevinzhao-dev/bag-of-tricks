package cmd

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/kvzhao/netwatch/internal/storage"
	"github.com/kvzhao/netwatch/internal/tracker"
)

var historyDays int

var historyCmd = &cobra.Command{
	Use:   "history",
	Short: "Show recent daily traffic history",
	RunE: func(cmd *cobra.Command, args []string) error {
		store, err := storage.New()
		if err != nil {
			return fmt.Errorf("failed to open storage: %w", err)
		}

		records, err := store.Recent(historyDays)
		if err != nil {
			return fmt.Errorf("failed to load history: %w", err)
		}

		if len(records) == 0 {
			fmt.Println("No history records found.")
			fmt.Println("Run `netwatch` first to start collecting data.")
			return nil
		}

		limitBytes := uint64(thresholdGB * float64(tracker.GB))
		gb := float64(tracker.GB)

		// Header
		fmt.Println()
		fmt.Printf("  %-12s %10s %10s %10s  %s\n", "Date", "Download", "Upload", "Total", "Status")
		fmt.Printf("  %s\n", strings.Repeat("─", 60))

		for _, r := range records {
			total := r.TotalBytes()
			rx := float64(r.RxBytes) / gb
			tx := float64(r.TxBytes) / gb
			totalGB := float64(total) / gb

			status := "✓"
			if total > limitBytes {
				status = "⚠ OVER"
			}

			fmt.Printf("  %-12s %8.2f GB %8.2f GB %8.2f GB  %s\n",
				r.Date, rx, tx, totalGB, status)
		}

		fmt.Println()

		// Consecutive days
		days, dates, _ := store.ConsecutiveDaysOver(limitBytes)
		if days > 0 {
			fmt.Printf("  Consecutive days over %.0f GB: %d (%s)\n",
				thresholdGB, days, strings.Join(dates, ", "))
			remaining := 3 - days
			if remaining > 0 {
				fmt.Printf("  ⚠ 降速風險：再連續超標 %d 天觸發\n", remaining)
			} else {
				fmt.Printf("  ⚠ 已觸發降速條件！\n")
			}
			fmt.Println()
		}

		return nil
	},
}

func init() {
	historyCmd.Flags().IntVarP(&historyDays, "days", "n", 7, "Number of days to show")
	rootCmd.AddCommand(historyCmd)
}
