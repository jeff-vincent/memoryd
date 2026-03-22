package eval

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

// Report writes a human-readable comparison report.
func Report(w io.Writer, results []ScenarioResult) {
	fmt.Fprintf(w, "\n%s\n", strings.Repeat("=", 72))
	fmt.Fprintf(w, "  memoryd eval — bare Claude vs Claude + memoryd\n")
	fmt.Fprintf(w, "%s\n\n", strings.Repeat("=", 72))

	var totalBare, totalAug, totalCriteria int

	for _, r := range results {
		fmt.Fprintf(w, "--- %s ---\n\n", r.Scenario)

		for _, s := range r.Scores {
			delta := s.AugScore - s.BareScore
			marker := " "
			if delta > 0 {
				marker = "+"
			} else if delta < 0 {
				marker = "-"
			}
			fmt.Fprintf(w, "  [%s] %-60s  bare=%d  aug=%d\n", marker, s.Criterion, s.BareScore, s.AugScore)
			fmt.Fprintf(w, "      %s\n", s.Explanation)
		}

		fmt.Fprintf(w, "\n  TOTAL: bare=%d  aug=%d  delta=%+d\n\n", r.BareTotal, r.AugTotal, r.Delta)

		totalBare += r.BareTotal
		totalAug += r.AugTotal
		totalCriteria += len(r.Scores)
	}

	fmt.Fprintf(w, "%s\n", strings.Repeat("=", 72))
	fmt.Fprintf(w, "  AGGREGATE (%d scenarios, %d criteria)\n", len(results), totalCriteria)
	fmt.Fprintf(w, "    Bare total:      %d\n", totalBare)
	fmt.Fprintf(w, "    Augmented total: %d\n", totalAug)
	fmt.Fprintf(w, "    Delta:           %+d\n", totalAug-totalBare)
	if totalBare > 0 {
		pct := float64(totalAug-totalBare) / float64(totalBare) * 100
		fmt.Fprintf(w, "    Improvement:     %.1f%%\n", pct)
	}
	fmt.Fprintf(w, "%s\n\n", strings.Repeat("=", 72))
}

// ReportJSON writes machine-readable JSON output.
func ReportJSON(w io.Writer, results []ScenarioResult) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(results)
}

// LoadScenarios reads scenarios from JSON.
func LoadScenarios(data []byte) ([]Scenario, error) {
	var scenarios []Scenario
	if err := json.Unmarshal(data, &scenarios); err != nil {
		return nil, err
	}
	return scenarios, nil
}
