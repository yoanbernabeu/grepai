package cli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/yoanbernabeu/grepai/indexer"
	"github.com/yoanbernabeu/grepai/store"
	"github.com/yoanbernabeu/grepai/trace"
	"github.com/yoanbernabeu/grepai/watcher"
)

func applyDirectoryAction(action directoryAction, scanner *indexer.Scanner, forgetIndex func(string), dispatch func(watcher.FileEvent)) error {
	fileEvent := action.event
	if action.kind == directoryActionForgetIndex || ((fileEvent.Type == watcher.EventCreate || fileEvent.Type == watcher.EventModify) && !scanner.ShouldIndexPath(fileEvent.Path)) {
		forgetIndex(fileEvent.Path)
		return nil
	}
	if fileEvent.Type == watcher.EventCreate || fileEvent.Type == watcher.EventModify {
		fileInfo, err := scanner.ScanFile(fileEvent.Path)
		if err != nil {
			return err
		}
		if fileInfo == nil {
			forgetIndex(fileEvent.Path)
			return nil
		}
	}
	dispatch(fileEvent)
	return nil
}

func reconcilePolicyDirectory(ctx context.Context, projectRoot, directory string, scanner *indexer.Scanner, vectorStore store.VectorStore, symbolStore trace.SymbolStore, dispatch func(directoryAction)) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	directory = filepath.Clean(directory)
	if filepath.IsAbs(directory) || directory == ".." || strings.HasPrefix(directory, ".."+string(filepath.Separator)) {
		return fmt.Errorf("policy reconciliation scope %s is outside project root", directory)
	}
	if err := validateWatchProjectRoot(projectRoot, os.Lstat); err != nil {
		return err
	}
	if directory != "." {
		info, err := os.Lstat(filepath.Join(projectRoot, directory))
		if err != nil {
			return fmt.Errorf("stat policy reconciliation scope %s: %w", directory, err)
		}
		if !info.IsDir() {
			return fmt.Errorf("policy reconciliation scope %s is not a directory", directory)
		}
	}

	paths, err := indexedPathsUnderDirectory(ctx, directory, vectorStore, symbolStore)
	if err != nil {
		return err
	}
	files, _, err := scanner.ScanMetadataScope(directory)
	if err != nil {
		return fmt.Errorf("scan policy reconciliation scope %s: %w", directory, err)
	}
	actions := make([]directoryAction, 0, len(paths)+len(files))
	seen := make(map[string]struct{}, len(paths))
	for _, path := range paths {
		if err := ctx.Err(); err != nil {
			return err
		}
		if !scanner.ShouldIndexPath(path) {
			seen[filepath.Clean(path)] = struct{}{}
			actions = append(actions, directoryAction{kind: directoryActionForgetIndex, event: watcher.FileEvent{Type: watcher.EventDelete, Path: path}})
			continue
		}
		seen[filepath.Clean(path)] = struct{}{}
		info, err := os.Lstat(filepath.Join(projectRoot, path))
		switch {
		case err == nil && info.Mode().IsRegular():
			actions = append(actions, directoryAction{kind: directoryActionDispatch, event: watcher.FileEvent{Type: watcher.EventModify, Path: path}})
		case err == nil:
			actions = append(actions, directoryAction{kind: directoryActionForgetIndex, event: watcher.FileEvent{Type: watcher.EventDelete, Path: path}})
		case watchPathAbsent(err):
			actions = append(actions, directoryAction{kind: directoryActionDispatch, event: watcher.FileEvent{Type: watcher.EventDelete, Path: path}})
		default:
			return fmt.Errorf("stat indexed path %s: %w", path, err)
		}
	}
	for _, file := range files {
		path := filepath.Clean(file.Path)
		if _, ok := seen[path]; ok {
			continue
		}
		actions = append(actions, directoryAction{kind: directoryActionDispatch, event: watcher.FileEvent{Type: watcher.EventCreate, Path: path}})
	}
	for _, action := range actions {
		if err := ctx.Err(); err != nil {
			return err
		}
		dispatch(action)
	}
	return ctx.Err()
}
