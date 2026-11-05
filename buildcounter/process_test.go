package buildcounter

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"sync"
	"testing"
	"time"
)

func TestCounterProcessWorker(t *testing.T) {
	path := os.Getenv("GO_VERSIONING_COUNTER_TEST_STORE")
	if path == "" {
		t.Skip("subprocess helper")
	}
	s, err := Open(path, "example-app")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	for i := 0; i < 5; i++ {
		if _, err := s.Reserve(ctx, request(fmt.Sprintf("process-%s-%d", os.Getenv("GO_VERSIONING_COUNTER_TEST_ID"), i))); err != nil {
			t.Fatal(err)
		}
	}
}
func TestSeparateProcessesAllocateUniquePersistentNumbers(t *testing.T) {
	s, path := newStore(t)
	const workers = 4
	failures := make(chan error, workers)
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			command := exec.Command(os.Args[0], "-test.run=^TestCounterProcessWorker$")
			command.Env = append(os.Environ(), "GO_VERSIONING_COUNTER_TEST_STORE="+path, fmt.Sprintf("GO_VERSIONING_COUNTER_TEST_ID=%d", i))
			if output, err := command.CombinedOutput(); err != nil {
				failures <- fmt.Errorf("worker: %w: %s", err, output)
			}
		}(i)
	}
	wg.Wait()
	close(failures)
	for err := range failures {
		t.Error(err)
	}
	status, err := s.Status(context.Background())
	if err != nil || status.LastNumber != workers*5 {
		t.Fatalf("persistent process sequence=%+v %v", status, err)
	}
}
