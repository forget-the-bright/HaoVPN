package singleinstance_test

import (
	"errors"
	"testing"

	"haovpn/internal/singleinstance"
)

func TestAcquireClientExclusive(t *testing.T) {
	a, err := singleinstance.AcquireClient()
	if err != nil {
		t.Fatalf("first acquire: %v", err)
	}
	defer a.Release()

	_, err = singleinstance.AcquireClient()
	if !errors.Is(err, singleinstance.ErrAlreadyRunning) {
		t.Fatalf("second acquire want ErrAlreadyRunning, got %v", err)
	}
}

func TestAlreadyRunningMessage(t *testing.T) {
	msg := singleinstance.AlreadyRunningMessage()
	if msg == "" {
		t.Fatal("empty message")
	}
}
