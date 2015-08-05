package distsync

import (
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
	start := make(chan struct{})
	done := make(chan struct{})
	errCh := make(chan error)

	concurrentClients := 5

	for i := 0; i < concurrentClients; i++ {
		go func(i int) {
			mu := NewMutex("my-mutex", ":63790")

			// Try for 10 seconds (this should be more than the time needed for
			// all clients to get a lock.
			mu.Delay = 500 * time.Millisecond
			mu.Tries = 20

			<-start

			err := mu.Lock()
			if err != nil {
				t.Logf("error in client %d, err: %v", i, err)
				errCh <- err
				return
			}

			// Do work.
			time.Sleep(50 * time.Millisecond)

			mu.Unlock()

			done <- struct{}{}
		}(i)
	}

	close(start) // Start all goroutines at once to simulate lock contention.

	for i := 0; i < concurrentClients; i++ {
		select {
		case <-done:
			t.Logf("%d client(s) done", i+1)
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
