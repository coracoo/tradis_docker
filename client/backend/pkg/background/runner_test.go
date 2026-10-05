package background

import (
	"context"
	"errors"
	"io"
	"os"
	"strings"
	"testing"

	"dockerpanel/backend/pkg/logging"
	"dockerpanel/backend/pkg/secrets"
)

func TestRunnerCoalescesTrimmedTaskKeys(t *testing.T) {
	runner := New(context.Background())
	started := make(chan struct{})
	release := make(chan struct{})
	if !runner.Submit(" settings.discovery ", func(context.Context) error {
		close(started)
		<-release
		return nil
	}) {
		t.Fatal("first task was rejected")
	}
	<-started
	if runner.Submit("settings.discovery", func(context.Context) error { return nil }) {
		t.Fatal("trimmed duplicate task was accepted")
	}
	close(release)
	if err := runner.Wait(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestRunnerRejectsBlankKeysAndNilTasks(t *testing.T) {
	runner := New(context.Background())
	for _, key := range []string{"", " \t\n "} {
		if runner.Submit(key, func(context.Context) error { return nil }) {
			t.Fatalf("blank key %q was accepted", key)
		}
	}
	if runner.Submit("settings.discovery", nil) {
		t.Fatal("nil task was accepted")
	}
}

func TestRunnerAllowsSameTrimmedKeyAfterCompletion(t *testing.T) {
	runner := New(context.Background())
	if !runner.Submit(" settings.discovery ", func(context.Context) error { return nil }) {
		t.Fatal("first task was rejected")
	}
	if err := runner.Wait(context.Background()); err != nil {
		t.Fatal(err)
	}
	if !runner.Submit("settings.discovery", func(context.Context) error { return nil }) {
		t.Fatal("completed key was not released")
	}
	if err := runner.Wait(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestRunnerShutdownCancelsTasksAndWaitsForCompletion(t *testing.T) {
	runner := New(context.Background())
	started := make(chan struct{})
	canceled := make(chan struct{})
	release := make(chan struct{})
	if !runner.Submit("settings.discovery", func(ctx context.Context) error {
		close(started)
		<-ctx.Done()
		close(canceled)
		<-release
		return ctx.Err()
	}) {
		t.Fatal("task was rejected")
	}
	<-started

	shutdown := make(chan error, 1)
	go func() {
		shutdown <- runner.Shutdown(context.Background())
	}()
	<-canceled
	select {
	case err := <-shutdown:
		t.Fatalf("shutdown returned before task completed: %v", err)
	default:
	}
	close(release)
	if err := <-shutdown; err != nil {
		t.Fatal(err)
	}
}

func TestRunnerRejectsConcurrentSubmissionAfterShutdownStarts(t *testing.T) {
	runner := New(context.Background())
	canceled := make(chan struct{})
	release := make(chan struct{})
	if !runner.Submit("settings.discovery", func(ctx context.Context) error {
		<-ctx.Done()
		close(canceled)
		<-release
		return nil
	}) {
		t.Fatal("task was rejected")
	}

	shutdown := make(chan error, 1)
	go func() {
		shutdown <- runner.Shutdown(context.Background())
	}()
	<-canceled
	if runner.Submit("settings.refresh", func(context.Context) error { return nil }) {
		t.Fatal("task was accepted while shutdown was waiting")
	}
	close(release)
	if err := <-shutdown; err != nil {
		t.Fatal(err)
	}
}

func TestRunnerWaitsForConcurrentSubmission(t *testing.T) {
	runner := New(context.Background())
	firstStarted := make(chan struct{})
	firstRelease := make(chan struct{})
	firstCompleted := make(chan struct{})
	if !runner.Submit("settings.discovery", func(context.Context) error {
		close(firstStarted)
		<-firstRelease
		close(firstCompleted)
		return nil
	}) {
		t.Fatal("first task was rejected")
	}
	<-firstStarted
	runner.mu.Lock()
	generation := runner.idle
	runner.mu.Unlock()

	waitCtx, cancelWait := context.WithCancel(context.Background())
	defer cancelWait()
	waitStarted := make(chan struct{})
	waited := make(chan error, 1)
	go func() {
		close(waitStarted)
		waited <- runner.Wait(waitCtx)
	}()
	<-waitStarted

	secondStarted := make(chan struct{})
	secondRelease := make(chan struct{})
	if !runner.Submit("settings.refresh", func(context.Context) error {
		close(secondStarted)
		<-secondRelease
		return nil
	}) {
		t.Fatal("concurrent task was rejected")
	}
	<-secondStarted
	close(firstRelease)
	<-firstCompleted
	select {
	case <-generation:
		t.Fatal("generation closed while concurrent task was running")
	default:
	}
	select {
	case err := <-waited:
		t.Fatalf("wait returned while concurrent task was running: %v", err)
	default:
	}
	cancelWait()
	if err := <-waited; !errors.Is(err, context.Canceled) {
		t.Fatalf("wait error = %v, want context canceled", err)
	}
	close(secondRelease)
	<-generation
	if err := runner.Wait(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestRunnerWaitHonorsCallerCancellation(t *testing.T) {
	runner := New(context.Background())
	started := make(chan struct{})
	if !runner.Submit("settings.discovery", func(ctx context.Context) error {
		close(started)
		<-ctx.Done()
		return ctx.Err()
	}) {
		t.Fatal("task was rejected")
	}
	<-started

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := runner.Wait(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("wait error = %v, want context canceled", err)
	}
	if err := runner.Shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestRunnerRecoversTaskPanicsAndReleasesSameKey(t *testing.T) {
	runner := New(context.Background())
	if !runner.Submit(" settings.discovery ", func(context.Context) error {
		panic("unexpected task panic")
	}) {
		t.Fatal("panic task was rejected")
	}
	if err := runner.Wait(context.Background()); err != nil {
		t.Fatal(err)
	}

	completed := make(chan struct{})
	if !runner.Submit("settings.discovery", func(context.Context) error {
		close(completed)
		return nil
	}) {
		t.Fatal("panic key was not released")
	}
	if err := runner.Wait(context.Background()); err != nil {
		t.Fatal(err)
	}
	select {
	case <-completed:
	default:
		t.Fatal("task after panic did not run")
	}
}

func TestRunnerRedactsTaskErrorAndPanicLogs(t *testing.T) {
	logs := captureLogs(t, func() {
		runner := New(context.Background())
		if !runner.Submit("settings.error", func(context.Context) error {
			return errors.New("TOKEN=error-token password=error-password dotenv: TOKEN=dotenv-error")
		}) {
			t.Fatal("error task was rejected")
		}
		if err := runner.Wait(context.Background()); err != nil {
			t.Fatal(err)
		}
		if !runner.Submit("settings.panic", func(context.Context) error {
			panic("TOKEN=panic-token password=panic-password dotenv: TOKEN=dotenv-panic")
		}) {
			t.Fatal("panic task was rejected")
		}
		if err := runner.Wait(context.Background()); err != nil {
			t.Fatal(err)
		}
	})

	for _, secret := range []string{
		"error-token",
		"error-password",
		"dotenv-error",
		"panic-token",
		"panic-password",
		"dotenv-panic",
	} {
		if strings.Contains(logs, secret) {
			t.Fatalf("log leaked %q: %s", secret, logs)
		}
	}
	if !strings.Contains(logs, secrets.RedactedValue) {
		t.Fatalf("log did not contain redacted marker: %s", logs)
	}
}

func TestCaptureLogsRestoresGlobalsWhenCallbackStopsTest(t *testing.T) {
	previousStdout := os.Stdout
	completed := t.Run("callback stops", func(t *testing.T) {
		captureLogs(t, func() {
			t.SkipNow()
		})
	})
	if !completed {
		t.Fatal("skipped callback subtest did not complete")
	}
	if os.Stdout != previousStdout {
		t.Fatal("captureLogs did not restore stdout after callback stopped the test")
	}
}

func captureLogs(t *testing.T, run func()) string {
	t.Helper()
	t.Setenv("LOG_LEVEL", "debug")
	previousStdout := os.Stdout
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	writerClosed := false
	readerClosed := false
	closeWriter := func() error {
		if writerClosed {
			return nil
		}
		writerClosed = true
		return writer.Close()
	}
	closeReader := func() error {
		if readerClosed {
			return nil
		}
		readerClosed = true
		return reader.Close()
	}
	t.Cleanup(func() {
		os.Stdout = previousStdout
		logging.Configure()
		_ = closeWriter()
		_ = closeReader()
	})
	os.Stdout = writer
	logging.Configure()
	run()
	if err := closeWriter(); err != nil {
		t.Fatal(err)
	}
	os.Stdout = previousStdout
	logging.Configure()
	output, err := io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}
	if err := closeReader(); err != nil {
		t.Fatal(err)
	}
	return string(output)
}
