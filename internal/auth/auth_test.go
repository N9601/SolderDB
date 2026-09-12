package auth

import (
	"fmt"
	"sync"
	"testing"

	"solderdb/internal/collections"
	"solderdb/internal/engine"
)

func newTestService(t *testing.T) *Service {
	t.Helper()
	dir := t.TempDir()
	db, err := engine.Open(engine.Options{DataDir: dir})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	svc, err := New(db, collections.New(db), dir)
	if err != nil {
		t.Fatalf("auth: %v", err)
	}
	return svc
}

func TestConcurrentRegistrationsCreateOneAdmin(t *testing.T) {
	svc := newTestService(t)

	const n = 4
	roles := make([]Role, n)
	errs := make([]error, n)
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			sess, err := svc.Register(fmt.Sprintf("user%d@test.local", i), "supersecret")
			roles[i], errs[i] = sess.User.Role, err
		}(i)
	}
	wg.Wait()

	admins := 0
	for i := 0; i < n; i++ {
		if errs[i] != nil {
			t.Fatalf("register %d: %v", i, errs[i])
		}
		if roles[i] == RoleAdmin {
			admins++
		}
	}
	if admins != 1 {
		t.Fatalf("expected exactly one admin from concurrent first registrations, got %d (%v)", admins, roles)
	}
}

func TestConcurrentRegistrationsOfOneEmail(t *testing.T) {
	svc := newTestService(t)

	const n = 3
	errs := make([]error, n)
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, errs[i] = svc.Register("same@test.local", "supersecret")
		}(i)
	}
	wg.Wait()

	ok := 0
	for _, err := range errs {
		if err == nil {
			ok++
		}
	}
	if ok != 1 {
		t.Fatalf("expected one successful registration for a single email, got %d (%v)", ok, errs)
	}
}
