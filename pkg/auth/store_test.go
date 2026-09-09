package auth

import (
	"github.com/stretchr/testify/require"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func TestCredentialsAtomicReplacement(t *testing.T) {
	file := filepath.Join(t.TempDir(), "accounts.json")
	s := NewFileCredentialsStore(file)
	require.NoError(t, s.SaveCredentials(Credentials{AccessToken: "longlonglong", RefreshToken: "refresh"}))
	require.NoError(t, s.SaveCredentials(Credentials{AccessToken: "x"}))
	value, err := s.LoadCredentials()
	require.NoError(t, err)
	require.Equal(t, "x", value.AccessToken)
	info, err := os.Stat(file)
	require.NoError(t, err)
	require.Equal(t, os.FileMode(0600), info.Mode().Perm())
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := s.SaveCredentials(Credentials{AccessToken: "new"}); err != nil {
				t.Error(err)
			}
			if _, err := s.LoadCredentials(); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
}
