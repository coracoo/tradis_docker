package main

import (
	"context"
	"dockerpanel/backend/pkg/background"
	"errors"
	"net"
	"net/http"
	"testing"
	"time"
)

func TestShutdownClientCancelsSettingsTaskBeforeDatabaseClose(t *testing.T) {
	root, cancelBackground := context.WithCancel(context.Background())
	runner := background.New(root)
	started := make(chan struct{})
	finished := make(chan struct{})
	if !runner.Submit("settings.container-discovery", func(ctx context.Context) error {
		close(started)
		<-ctx.Done()
		close(finished)
		return ctx.Err()
	}) {
		t.Fatal("settings task was rejected")
	}
	<-started

	databaseClosed := false
	shutdownClient(&http.Server{}, cancelBackground, runner, time.Second, func() error {
		select {
		case <-finished:
			databaseClosed = true
			return nil
		default:
			t.Fatal("database close ran before the settings task finished")
			return nil
		}
	})
	if !databaseClosed {
		t.Fatal("database close callback was not called")
	}
}

func TestShutdownClientBoundsUncooperativeRunnerWithoutClosingDatabase(t *testing.T) {
	root, cancelBackground := context.WithCancel(context.Background())
	runner := background.New(root)
	started := make(chan struct{})
	canceled := make(chan struct{})
	release := make(chan struct{})
	if !runner.Submit("settings.container-discovery", func(ctx context.Context) error {
		close(started)
		<-ctx.Done()
		close(canceled)
		<-release
		return ctx.Err()
	}) {
		t.Fatal("settings task was rejected")
	}
	<-started

	timeoutObserved := make(chan struct{}, 1)
	databaseClosed := make(chan struct{}, 1)
	shutdownDone := make(chan struct{})
	go func() {
		shutdownClientWithTimeoutObserver(&http.Server{}, cancelBackground, runner, time.Millisecond, func() error {
			databaseClosed <- struct{}{}
			return nil
		}, func(component string) {
			if component == "settings_background" {
				timeoutObserved <- struct{}{}
			}
		})
		close(shutdownDone)
	}()

	<-canceled
	<-timeoutObserved
	select {
	case <-databaseClosed:
		t.Fatal("database close ran after the runner timeout but before task completion")
	default:
	}

	select {
	case <-shutdownDone:
	case <-time.After(time.Second):
		close(release)
		<-shutdownDone
		t.Fatal("shutdown waited beyond its deadline")
	}
	close(release)
	if err := runner.Wait(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestShutdownClientBoundsHTTPHandlerWithoutClosingDatabase(t *testing.T) {
	handlerStarted := make(chan struct{})
	releaseHandler := make(chan struct{})
	server := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		close(handlerStarted)
		<-releaseHandler
		w.WriteHeader(http.StatusNoContent)
	})}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	serveDone := make(chan error, 1)
	go func() { serveDone <- server.Serve(listener) }()

	responseDone := make(chan error, 1)
	go func() {
		response, requestErr := http.Get("http://" + listener.Addr().String())
		if response != nil {
			_ = response.Body.Close()
		}
		responseDone <- requestErr
	}()
	<-handlerStarted

	timeoutObserved := make(chan struct{}, 1)
	databaseClosed := make(chan struct{}, 1)
	shutdownDone := make(chan struct{})
	go func() {
		shutdownClientWithTimeoutObserver(server, func() {}, background.New(context.Background()), time.Millisecond, func() error {
			databaseClosed <- struct{}{}
			return nil
		}, func(component string) {
			if component == "http" {
				timeoutObserved <- struct{}{}
			}
		})
		close(shutdownDone)
	}()

	<-timeoutObserved
	select {
	case <-databaseClosed:
		t.Fatal("database close ran after the HTTP timeout but before the handler completed")
	default:
	}

	select {
	case <-shutdownDone:
	case <-time.After(time.Second):
		close(releaseHandler)
		<-shutdownDone
		t.Fatal("shutdown waited beyond its deadline")
	}
	close(releaseHandler)
	<-responseDone
	if err := <-serveDone; !errors.Is(err, http.ErrServerClosed) {
		t.Fatalf("server.Serve() error = %v, want ErrServerClosed", err)
	}
	select {
	case <-databaseClosed:
		t.Fatal("database closed with a timed-out handler")
	default:
	}
}
