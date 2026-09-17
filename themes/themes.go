// Package themes embeds the bundled glamour themes so they can be referenced
// by name from any presentation, not only from inside a repository checkout.
package themes

import (
	"embed"
	"io/fs"
	"sort"
	"strings"
)

//go:embed *.json
var files embed.FS

// Lookup returns the bundled theme with the given name (e.g. "kontrolplane").
// A ".json" suffix is accepted and ignored.
func Lookup(name string) ([]byte, bool) {
	name = strings.TrimSuffix(name, ".json")
	if name == "" || strings.ContainsAny(name, `/\.`) {
		return nil, false
	}
	data, err := files.ReadFile(name + ".json")
	if err != nil {
		return nil, false
	}
	return data, true
}

// Names lists the bundled theme names.
func Names() []string {
	entries, err := fs.Glob(files, "*.json")
	if err != nil {
		return nil
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		names = append(names, strings.TrimSuffix(e, ".json"))
	}
	sort.Strings(names)
	return names
}
