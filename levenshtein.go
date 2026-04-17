package cmd

import "slices"

// levenshtein distance algorithm
func levenshtein(s1, s2 string) int {
	if len(s1) == 0 {
		return len(s2)
	}
	if len(s2) == 0 {
		return len(s1)
	}

	if len(s1) < len(s2) {
		s1, s2 = s2, s1
	}

	n := len(s1)
	m := len(s2)

	row := make([]int, m+1)
	for i := 0; i <= m; i++ {
		row[i] = i
	}

	for i := 1; i <= n; i++ {
		prev := i
		for j := 1; j <= m; j++ {
			var val int
			if s1[i-1] == s2[j-1] {
				val = row[j-1]
			} else {
				val = 1 + min(row[j-1], row[j], prev)
			}
			row[j-1] = prev
			prev = val
		}
		row[m] = prev
	}

	return row[m]
}

// suggestCommand returns the most similar command names from the list
func suggestCommand(name string, cmds Commands) []string {
	type suggestion struct {
		name string
		dist int
	}
	var suggestions []suggestion

	for _, cmd := range cmds {
		if cmd.Hidden {
			continue
		}
		d := levenshtein(name, cmd.Name)
		if d <= 3 { // Threshold for similarity
			suggestions = append(suggestions, suggestion{cmd.Name, d})
		}
		for _, alias := range cmd.Aliases {
			d := levenshtein(name, alias)
			if d <= 3 {
				suggestions = append(suggestions, suggestion{alias, d})
			}
		}
	}

	slices.SortFunc(suggestions, func(a, b suggestion) int {
		return a.dist - b.dist
	})

	var result []string
	for _, s := range suggestions {
		result = append(result, s.name)
	}

	if len(result) > 3 {
		result = result[:3]
	}

	return result
}
