package deck

import (
	"context"
	"path/filepath"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/kontrolplane/lekture/internal/meta"
)

// debounce coalesces the burst of events a single editor save produces.
const debounce = 75 * time.Millisecond

// Watch calls onChange whenever the deck's file changes on disk, until ctx is
// cancelled. onErr reports problems that disable or interrupt watching.
//
// The parent directory is watched rather than the file itself: editors such as
// vim save by writing a temporary file and renaming it over the original,
// which destroys the watched inode and would otherwise silently end live
// reload for the rest of the session.
func Watch(ctx context.Context, path string, base meta.Meta, onChange func(*Deck), onErr func(error)) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		onErr(err)
		return
	}
	defer watcher.Close()

	if err := watcher.Add(filepath.Dir(path)); err != nil {
		onErr(err)
		return
	}

	const relevant = fsnotify.Write | fsnotify.Create | fsnotify.Rename | fsnotify.Remove

	var timer <-chan time.Time
	for {
		select {
		case <-ctx.Done():
			return

		case event, ok := <-watcher.Events:
			if !ok {
				return
			}
			if filepath.Clean(event.Name) != path {
				continue
			}
			if event.Op&relevant == 0 {
				continue
			}
			timer = time.After(debounce)

		case <-timer:
			timer = nil
			d, err := Load(path, base)
			if err != nil {
				// The file may be mid-rename; the next event will retry.
				continue
			}
			onChange(d)

		case err, ok := <-watcher.Errors:
			if !ok {
				return
			}
			onErr(err)
		}
	}
}
