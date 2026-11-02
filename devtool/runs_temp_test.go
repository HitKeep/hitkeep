package devtool

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
)

func TestStartRunIsolatesWorkerTemporaryAndGoCaches(t *testing.T) {
	root := initTestRepository(t)
	stateRoot := filepath.Join(t.TempDir(), "state")
	t.Setenv("HK_STATE_DIR", stateRoot)
	t.Setenv("TMPDIR", filepath.Join(t.TempDir(), "global-tmp"))
	t.Setenv("GOTMPDIR", filepath.Join(t.TempDir(), "global-go-tmp"))
	t.Setenv("GOCACHE", filepath.Join(t.TempDir(), "global-go-cache"))
	t.Setenv("HITKEEP_PREVIOUS_IMAGE", "ghcr.io/pascalebeier/hitkeep:2.12.0@sha256:test")
	original := map[string]string{"TMPDIR": os.Getenv("TMPDIR"), "GOTMPDIR": os.Getenv("GOTMPDIR"), "GOCACHE": os.Getenv("GOCACHE")}

	worker := filepath.Join(t.TempDir(), "hk-worker")
	if err := os.WriteFile(worker, []byte("#!/bin/sh\nenv > \"$HK_RUN_TEMP_ROOT/worker-env\"\nwhile :; do sleep 1; done\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	app, err := NewApp(root)
	if err != nil {
		t.Fatal(err)
	}
	app.executable = worker
	start, err := app.StartRun(context.Background(), RunRequest{Kind: "setup"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		run, getErr := app.GetRun(start.RunID)
		if getErr == nil && run.PID > 0 {
			_ = syscall.Kill(-run.PID, syscall.SIGKILL)
		}
		_ = app.removeRunTempRoot(start.RunID)
	})

	tempRoot := app.runTempRoot(start.RunID)
	workerEnvironment := waitForWorkerEnvironment(t, filepath.Join(tempRoot, "worker-env"))
	for key, want := range map[string]string{
		"HK_RUN_TEMP_ROOT":       tempRoot,
		"HITKEEP_PREVIOUS_IMAGE": "ghcr.io/pascalebeier/hitkeep:2.12.0@sha256:test",
		"TMPDIR":                 filepath.Join(tempRoot, "tmp"),
		"GOTMPDIR":               filepath.Join(tempRoot, "go-tmp"),
		"GOCACHE":                filepath.Join(tempRoot, "go-cache"),
	} {
		if got := workerEnvironmentValue(workerEnvironment, key); got != want {
			t.Fatalf("worker %s = %q, want %q", key, got, want)
		}
	}
	for key, want := range original {
		if got := os.Getenv(key); got != want {
			t.Fatalf("global %s changed to %q, want %q", key, got, want)
		}
	}
}

func TestCommandEnvironmentPreservesValidatedRunIsolation(t *testing.T) {
	root := initTestRepository(t)
	t.Setenv("HK_STATE_DIR", filepath.Join(t.TempDir(), "state"))
	app, err := NewApp(root)
	if err != nil {
		t.Fatal(err)
	}
	const runID = "20260827T010000-abcd1234"
	if _, err := app.prepareRunTempEnvironment(runID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = app.removeRunTempRoot(runID) })
	t.Setenv("HK_RUN_TEMP_ROOT", app.runTempRoot(runID))

	environment := environmentValues(app.commandEnvironment(nil))
	if got, want := environment["TMPDIR"], filepath.Join(app.runTempRoot(runID), "tmp"); got != want {
		t.Fatalf("TMPDIR = %q, want %q", got, want)
	}
	if got, want := environment["GOTMPDIR"], filepath.Join(app.runTempRoot(runID), "go-tmp"); got != want {
		t.Fatalf("GOTMPDIR = %q, want %q", got, want)
	}
	if got, want := environment["GOCACHE"], filepath.Join(app.runTempRoot(runID), "go-cache"); got != want {
		t.Fatalf("GOCACHE = %q, want %q", got, want)
	}
}

func TestPruneTerminalRunTempRootsLeavesActiveAndUnknownRoots(t *testing.T) {
	root := initTestRepository(t)
	t.Setenv("HK_STATE_DIR", filepath.Join(t.TempDir(), "state"))
	app, err := NewApp(root)
	if err != nil {
		t.Fatal(err)
	}
	terminalID := "20260827T010000-deadbeef"
	activeID := "20260827T010001-cafe1234"
	unknownID := "20260827T010002-feed1234"
	for _, runID := range []string{terminalID, activeID, unknownID} {
		if _, err := app.prepareRunTempEnvironment(runID); err != nil {
			t.Fatal(err)
		}
	}
	started := time.Now().UTC().Add(-3 * time.Second)
	if err := app.writeRun(Run{ID: terminalID, WorkspaceID: app.workspace.ID, Request: RunRequest{Kind: "setup"}, Status: "running", PID: 1 << 30, StartedAt: started}); err != nil {
		t.Fatal(err)
	}
	if err := app.writeRun(Run{ID: activeID, WorkspaceID: app.workspace.ID, Request: RunRequest{Kind: "setup"}, Status: "running", PID: os.Getpid()}); err != nil {
		t.Fatal(err)
	}
	if err := app.pruneTerminalRunTempRoots(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(app.runTempRoot(terminalID)); !os.IsNotExist(err) {
		t.Fatalf("crashed worker root remains: %v", err)
	}
	for _, runID := range []string{activeID, unknownID} {
		if _, err := os.Stat(app.runTempRoot(runID)); err != nil {
			t.Fatalf("%s root was removed: %v", runID, err)
		}
	}
}

func waitForWorkerEnvironment(t *testing.T, path string) string {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		content, err := os.ReadFile(path)
		if err == nil && workerEnvironmentValue(string(content), "GOCACHE") != "" {
			return string(content)
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", path)
	return ""
}

func workerEnvironmentValue(environment, key string) string {
	return environmentValues(strings.Split(strings.TrimSpace(environment), "\n"))[key]
}

func environmentValues(environment []string) map[string]string {
	values := make(map[string]string, len(environment))
	for _, entry := range environment {
		key, value, found := strings.Cut(entry, "=")
		if found {
			values[key] = value
		}
	}
	return values
}

func TestExecuteRunPublishesTerminalAfterFinalization(t *testing.T) {
	app, run := prepareFinalizationRun(t)
	entered, release, done := make(chan struct{}), make(chan struct{}), make(chan struct{})
	unblock := sync.OnceFunc(func() { close(release) })
	app.beforeRunFinalize = func() { close(entered); <-release }
	var executeErr error
	t.Cleanup(func() { unblock(); <-done })
	go func() { defer close(done); executeErr = app.ExecuteRun(t.Context(), run.ID) }()
	select {
	case <-entered:
	case <-done:
		t.Fatalf("execution missed barrier: %v", executeErr)
	case <-t.Context().Done():
		t.Fatal(t.Context().Err())
	}
	current, err := app.GetRun(run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if isTerminal(current.Status) || current.PID != os.Getpid() {
		t.Fatalf("published terminal before finalization: %+v", current)
	}
	unblock()
	<-done
	if executeErr != nil {
		t.Fatal(executeErr)
	}
	current, err = app.GetRun(run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if current.Status != "passed" || current.PID != 0 {
		t.Fatalf("final run: %+v", current)
	}
	assertFinalizedRun(t, app, current)
}

func prepareFinalizationRun(t *testing.T) (*App, Run) {
	t.Helper()
	root := initTestRepository(t)
	writeTestToolchainConfig(t, root)
	t.Setenv("HK_STATE_DIR", filepath.Join(t.TempDir(), "state"))
	app, err := NewApp(root)
	if err != nil {
		t.Fatal(err)
	}
	paths, err := app.managedToolchainPaths()
	if err != nil {
		t.Fatal(err)
	}
	config, err := app.ToolchainConfig()
	if err != nil {
		t.Fatal(err)
	}
	for path, script := range map[string]string{
		paths.GoExecutable:   "echo 'go version go" + config.Go + " test/arch'",
		paths.NodeExecutable: "case \"${1:-}\" in *npm-cli.js) echo " + config.NPM + " ;; *) echo v" + config.Node + " ;; esac",
		paths.NPMExecutable:  "echo " + config.NPM,
		paths.NPXExecutable:  "echo final-action-output",
		paths.NPMCLI:         "exit 0",
	} {
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("#!/bin/sh\n"+script+"\n"), 0700); err != nil {
			t.Fatal(err)
		}
	}
	fakeBin := t.TempDir()
	if err := os.WriteFile(filepath.Join(fakeBin, "docker"), []byte("#!/bin/sh\necho 1.0.0\n"), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", fakeBin+string(os.PathListSeparator)+os.Getenv("PATH"))
	for key, value := range map[string]string{"HK_CHILD_RUN": "1", "HK_EXPECTED_SCHEMA": SchemaVersion, "HK_WORKER_PROTOCOL": "1", "HK_WORKSPACE_ID": app.workspace.ID, "HK_SOURCE_FINGERPRINT": "legacy"} {
		t.Setenv(key, value)
	}
	run := Run{ID: "finalization", WorkspaceID: app.workspace.ID, Request: RunRequest{Kind: "setup"}, Status: "running", PID: os.Getpid(), StartedAt: time.Now().UTC(), LogPath: filepath.Join(app.workspace.StateDir, "runs", "finalization.log")}
	if _, err := app.prepareRunTempEnvironment(run.ID); err != nil {
		t.Fatal(err)
	}
	if err := app.writeRun(run); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(app.runReadyPath(run.ID), nil, 0600); err != nil {
		t.Fatal(err)
	}
	return app, run
}

func assertFinalizedRun(t *testing.T, app *App, run Run) {
	t.Helper()
	for _, path := range []string{app.runTempRoot(run.ID), app.runReadyPath(run.ID), app.runCancellationPath(run.ID)} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Errorf("transient path remains %s: %v", path, err)
		}
	}
	log, err := os.ReadFile(run.LogPath)
	if err != nil || len(log) == 0 {
		t.Fatalf("retained log: %q, %v", log, err)
	}
	index, err := app.loadArtifactIndex()
	if err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(run.LogPath)
	if err != nil {
		t.Fatal(err)
	}
	digest, err := artifactDigest(run.LogPath, info)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range index.Entries {
		if entry.Path == run.LogPath && entry.Digest == digest {
			return
		}
	}
	t.Fatalf("final log digest missing from index: %+v", index)
}

func TestExecuteRunFinalizationErrors(t *testing.T) {
	for _, tc := range []struct{ name, wantStatus, wantError string }{
		{"action", "failed", "exit code 7"},
		{"log-open", "failed", "open run log"},
		{"artifact", "passed", ""},
		{"cleanup", "failed", "remove run marker"},
		{"cancelled-cleanup", "cancelled", "remove run marker"},
		{"maintenance", "passed", ""},
		{"terminal-write", "running", "replace state"},
		{"protocol", "running", "worker protocol mismatch"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			app, run := prepareFinalizationRun(t)
			switch tc.name {
			case "action":
				paths, err := app.managedToolchainPaths()
				if err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(paths.NPXExecutable, []byte("#!/bin/sh\necho action-failed\nexit 7\n"), 0700); err != nil {
					t.Fatal(err)
				}
			case "log-open":
				if err := os.Mkdir(run.LogPath, 0700); err != nil {
					t.Fatal(err)
				}
			case "protocol":
				t.Setenv("HK_WORKER_PROTOCOL", "invalid")
			}
			var injectErr error
			app.beforeRunFinalize = func() {
				switch tc.name {
				case "artifact":
					injectErr = os.MkdirAll(app.artifactIndexPath(), 0700)
				case "cleanup", "cancelled-cleanup":
					injectErr = os.Mkdir(app.runCancellationPath(run.ID), 0700)
					if injectErr == nil {
						injectErr = os.WriteFile(filepath.Join(app.runCancellationPath(run.ID), "keep"), nil, 0600)
					}
				case "maintenance":
					// A marker directory prevents the best-effort maintenance timestamp write.
					marker := filepath.Join(app.workspace.StateDir, "artifacts", ".last-maintenance")
					injectErr = os.MkdirAll(marker, 0700)
					if injectErr == nil {
						injectErr = os.Chtimes(marker, time.Time{}, time.Time{})
					}
				case "terminal-write":
					path := filepath.Join(app.workspace.StateDir, "runs", run.ID+".json")
					injectErr = os.Rename(path, path+".saved")
					if injectErr == nil {
						injectErr = os.Mkdir(path, 0700)
					}
				}
			}
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			if tc.name == "cancelled-cleanup" {
				cancel()
			}
			err := app.ExecuteRun(ctx, run.ID)
			if tc.name == "cancelled-cleanup" && !errors.Is(err, context.Canceled) {
				t.Fatalf("cleanup lost cancellation: %v", err)
			}
			if injectErr != nil {
				t.Fatal(injectErr)
			}
			if tc.wantError == "" {
				if err != nil {
					t.Fatal(err)
				}
			} else if err == nil || !strings.Contains(err.Error(), tc.wantError) {
				t.Fatalf("error = %v, want %q", err, tc.wantError)
			}
			if tc.name == "terminal-write" {
				path := filepath.Join(app.workspace.StateDir, "runs", run.ID+".json")
				if err := os.Remove(path); err != nil {
					t.Fatal(err)
				}
				if err := os.Rename(path+".saved", path); err != nil {
					t.Fatal(err)
				}
			}
			current, err := app.GetRun(run.ID)
			if err != nil {
				t.Fatal(err)
			}
			if current.Status != tc.wantStatus {
				t.Fatalf("run = %+v, want %s", current, tc.wantStatus)
			}
			if tc.name == "protocol" {
				if _, err := os.Stat(app.runReadyPath(run.ID)); err != nil {
					t.Fatalf("invalid worker mutated handshake: %v", err)
				}
				return
			}
			if _, err := os.Stat(app.runTempRoot(run.ID)); !os.IsNotExist(err) {
				t.Fatalf("temp root remains: %v", err)
			}
			if tc.name == "action" || tc.name == "maintenance" {
				assertFinalizedRun(t, app, current)
			}
		})
	}
}

func TestExecuteRunCancellationWhileActionBlocked(t *testing.T) {
	app, run := prepareFinalizationRun(t)
	enteredPath, releasePath := filepath.Join(t.TempDir(), "entered"), filepath.Join(t.TempDir(), "release")
	var pipes []*os.File
	for _, path := range []string{enteredPath, releasePath} {
		if err := syscall.Mkfifo(path, 0600); err != nil {
			t.Fatal(err)
		}
		f, err := os.OpenFile(path, os.O_RDWR, 0600)
		if err != nil {
			t.Fatal(err)
		}
		pipes = append(pipes, f)
		t.Cleanup(func() { _ = f.Close() })
	}
	paths, err := app.managedToolchainPaths()
	if err != nil {
		t.Fatal(err)
	}
	script := fmt.Sprintf("#!/bin/sh\nprintf 'x' > %q\nread release < %q\n", enteredPath, releasePath)
	if err := os.WriteFile(paths.NPXExecutable, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	done := make(chan struct{})
	var executeErr error
	t.Cleanup(func() { cancel(); <-done })
	go func() { defer close(done); executeErr = app.ExecuteRun(ctx, run.ID) }()
	entered := make(chan error, 1)
	go func() { var b [1]byte; _, err := pipes[0].Read(b[:]); entered <- err }()
	select {
	case err := <-entered:
		if err != nil {
			t.Fatal(err)
		}
	case <-done:
		t.Fatalf("action exited before blocking: %v", executeErr)
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	if _, err := app.CancelRun(run.ID); err != nil {
		t.Fatal(err)
	}
	<-done
	if !errors.Is(executeErr, context.Canceled) {
		t.Fatalf("execution error = %v", executeErr)
	}
	current, err := app.GetRun(run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if current.Status != "cancelled" || current.ExitCode == nil || *current.ExitCode != 130 {
		t.Fatalf("cancelled run: %+v", current)
	}
	assertFinalizedRun(t, app, current)
}

func TestExecuteRunLateCancellationDoesNotRegressTerminal(t *testing.T) {
	app, run := prepareFinalizationRun(t)
	var callers sync.WaitGroup
	start := make(chan struct{})
	app.beforeRunFinalize = func() {
		cancelled, err := app.CancelRun(run.ID)
		if err != nil || cancelled.Status != "cancelling" {
			t.Errorf("late cancellation: %+v, %v", cancelled, err)
		}
		for range 8 {
			callers.Go(func() {
				<-start
				if _, err := app.CancelRun(run.ID); err != nil {
					t.Errorf("cancel: %v", err)
				}
			})
		}
		close(start)
	}
	err := app.ExecuteRun(t.Context(), run.ID)
	callers.Wait()
	if err != nil {
		t.Fatal(err)
	}
	current, err := app.CancelRun(run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if current.Status != "passed" {
		t.Fatalf("completed action regressed: %+v", current)
	}
	assertFinalizedRun(t, app, current)
}
