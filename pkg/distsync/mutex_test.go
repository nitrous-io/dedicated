package distsync

import (
	"math/rand"
	"os"
	"testing"
	"time"

	"github.com/stvp/tempredis"
)

var server *tempredis.Server

// Some extra setup and teardown is required here.
func TestMain(m *testing.M) {
	var err error
	server, err = tempredis.Start(tempredis.Config{
		"port": "63790",
	})
	if err != nil {
		panic(err)
	}
	defer server.Term()

	exitCode := m.Run()

	os.Exit(exitCode)
}

func TestMutex(t *testing.T) {
	done := make(chan struct{})
	errCh := make(chan error)

	concurrentClients := 4
	locksPerClient := 10

	for i := 0; i < concurrentClients; i++ {
		go func() {
			mu := NewMutex("my-mutex", ":63790")
			mu.Delay = 1 * time.Second

			for j := 0; j < locksPerClient; j++ {
				err := mu.Lock()
				if err != nil {
					errCh <- err
					return
				}

				// Give goroutine scheduler a chance to run other goroutines.
				time.Sleep(1 * time.Millisecond)

				mu.Unlock()

				// Wait a random amount of time before trying to lock again.
				time.Sleep(time.Duration(rand.Int31n(100)) * time.Millisecond)
			}

			done <- struct{}{}
		}()
	}

	for i := 0; i < concurrentClients; i++ {
		select {
		case <-done:
			t.Logf("client %d done", i)
		case err := <-errCh:
			t.Fatal(err)
		}
	}
}

func TestUnlockPanic(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatalf("expected unlock of unlocked to panic, but didn't")
		}
	}()

	mu := NewMutex("my-mutex", ":63790")

	if err := mu.Lock(); err != nil {
		t.Errorf("failed to acquire lock, err: %v", err)
	}

	mu.Unlock()
	mu.Unlock()
}
