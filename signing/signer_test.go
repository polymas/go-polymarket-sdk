package signing

import (
	"errors"
	"sync"
	"testing"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/polymas/go-polymarket-sdk/types"
)

const signerTestPrivateKey = "1111111111111111111111111111111111111111111111111111111111111111"

func TestSignerClearDisablesSigning(t *testing.T) {
	signer, err := NewSigner(signerTestPrivateKey, types.Polygon)
	if err != nil {
		t.Fatal(err)
	}
	digest := crypto.Keccak256Hash([]byte("before close"))
	if _, err := signer.SignWithRecovery(digest); err != nil {
		t.Fatalf("sign before Clear: %v", err)
	}

	signer.Clear()
	signer.Clear()

	if !signer.Closed() {
		t.Fatal("signer should report closed after Clear")
	}
	if _, err := signer.SignWithRecovery(digest); !errors.Is(err, ErrSignerClosed) {
		t.Fatalf("SignWithRecovery after Clear error = %v, want ErrSignerClosed", err)
	}
	if _, err := signer.Sign(digest); !errors.Is(err, ErrSignerClosed) {
		t.Fatalf("Sign after Clear error = %v, want ErrSignerClosed", err)
	}
}

func TestSignerConcurrentSignAndClear(t *testing.T) {
	signer, err := NewSigner(signerTestPrivateKey, types.Polygon)
	if err != nil {
		t.Fatal(err)
	}
	digest := crypto.Keccak256Hash([]byte("concurrent close"))

	var wg sync.WaitGroup
	errs := make(chan error, 32)
	for range 32 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, signErr := signer.SignWithRecovery(digest)
			if signErr != nil && !errors.Is(signErr, ErrSignerClosed) {
				errs <- signErr
			}
		}()
	}
	signer.Clear()
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatalf("unexpected concurrent signing error: %v", err)
	}
}

func TestNilSignerReturnsClosed(t *testing.T) {
	var signer *Signer
	if !signer.Closed() {
		t.Fatal("nil signer should report closed")
	}
	if _, err := signer.SignWithRecovery(crypto.Keccak256Hash(nil)); !errors.Is(err, ErrSignerClosed) {
		t.Fatalf("nil signer error = %v, want ErrSignerClosed", err)
	}
	signer.Clear()
}
