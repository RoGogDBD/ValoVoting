package twitch

import "strings"

// CompetitiveMaps is the current Valorant competitive map pool.
var CompetitiveMaps = []string{
	"Abyss",
	"Ascent",
	"Bind",
	"Haven",
	"Icebox",
	"Lotus",
	"Pearl",
	"Split",
	"Sunset",
}

// matchMap returns the canonical map name for the user-supplied string
// using case-insensitive prefix matching. Returns "" if no match.
func matchMap(input string) string {
	input = strings.ToLower(strings.TrimSpace(input))
	if input == "" {
		return ""
	}
	for _, m := range CompetitiveMaps {
		if strings.HasPrefix(strings.ToLower(m), input) {
			return m
		}
	}
	return ""
}
