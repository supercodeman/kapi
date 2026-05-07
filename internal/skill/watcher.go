package skill

import (
	"context"
	"log"
	"time"

	"github.com/fsnotify/fsnotify"
)

type Watcher struct {
	registry    *Registry
	fileDir     string
	dbInterval  time.Duration
	stopCh      chan struct{}
}

func NewWatcher(registry *Registry, fileDir string, dbInterval time.Duration) *Watcher {
	if dbInterval <= 0 {
		dbInterval = 10 * time.Second
	}
	return &Watcher{
		registry:   registry,
		fileDir:    fileDir,
		dbInterval: dbInterval,
		stopCh:     make(chan struct{}),
	}
}

func (w *Watcher) Start(ctx context.Context) {
	go func() {
		defer func() {
			if r := recover(); r != nil {
				log.Printf("watchFiles panic: %v", r)
			}
		}()
		w.watchFiles(ctx)
	}()
	go func() {
		defer func() {
			if r := recover(); r != nil {
				log.Printf("pollDB panic: %v", r)
			}
		}()
		w.pollDB(ctx)
	}()
}

func (w *Watcher) Stop() {
	close(w.stopCh)
}

func (w *Watcher) watchFiles(ctx context.Context) {
	if w.fileDir == "" {
		return
	}

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		log.Printf("failed to create file watcher: %v", err)
		return
	}
	defer watcher.Close()

	if err := watcher.Add(w.fileDir); err != nil {
		log.Printf("failed to watch skill dir %s: %v", w.fileDir, err)
		return
	}

	log.Printf("watching skill files in %s", w.fileDir)

	for {
		select {
		case event, ok := <-watcher.Events:
			if !ok {
				return
			}
			if event.Has(fsnotify.Write) || event.Has(fsnotify.Create) || event.Has(fsnotify.Remove) {
				log.Printf("skill file changed: %s, reloading...", event.Name)
				if err := w.registry.Reload(ctx); err != nil {
					log.Printf("failed to reload skills: %v", err)
				}
			}
		case err, ok := <-watcher.Errors:
			if !ok {
				return
			}
			log.Printf("file watcher error: %v", err)
		case <-w.stopCh:
			return
		case <-ctx.Done():
			return
		}
	}
}

func (w *Watcher) pollDB(ctx context.Context) {
	ticker := time.NewTicker(w.dbInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			if err := w.registry.Reload(ctx); err != nil {
				log.Printf("failed to reload skills from db: %v", err)
			}
		case <-w.stopCh:
			return
		case <-ctx.Done():
			return
		}
	}
}
