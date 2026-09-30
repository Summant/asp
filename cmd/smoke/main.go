package main

import (
	"fmt"

	"github.com/summant/asp/internal/source"
)

func main() {
	for _, s := range source.All(source.Default()...) {
		title := s.Opening
		if len(title) > 54 {
			title = title[:54] + "…"
		}
		fmt.Printf("%-7s %-8s %-56s %s\n", s.Agent.Label(), s.ID[:8], title, s.CWD)
	}
}
