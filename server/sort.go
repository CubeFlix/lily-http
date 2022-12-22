// server/sort.go
// Alphabetical sorting.

package server

import "unicode"

// Stolen from https://stackoverflow.com/questions/35076109/in-golang-how-can-i-sort-a-list-of-strings-alphabetically-without-completely-ig.

type ByCase []DirItem

func (s ByCase) Len() int      { return len(s) }
func (s ByCase) Swap(i, j int) { s[i], s[j] = s[j], s[i] }

func (s ByCase) Less(i, j int) bool {
	iRunes := []rune(s[i].Name)
	jRunes := []rune(s[j].Name)

	max := len(iRunes)
	if max > len(jRunes) {
		max = len(jRunes)
	}

	for idx := 0; idx < max; idx++ {
		ir := iRunes[idx]
		jr := jRunes[idx]

		lir := unicode.ToLower(ir)
		ljr := unicode.ToLower(jr)

		if lir != ljr {
			return lir < ljr
		}

		// the lowercase runes are the same, so compare the original
		if ir != jr {
			return ir < jr
		}
	}

	// If the strings are the same up to the length of the shortest string,
	// the shorter string comes first
	return len(iRunes) < len(jRunes)
}
