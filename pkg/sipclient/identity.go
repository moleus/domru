package sipclient

import (
	"fmt"
	"os"
	"strings"

	"github.com/google/uuid"
	"github.com/moleus/domru/pkg/atomicfile"
)

func InstallationID(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err == nil {
		id := strings.TrimSpace(string(data))
		if _, err = uuid.Parse(id); err != nil {
			return "", fmt.Errorf("invalid installation ID file")
		}
		return id, nil
	}
	if !os.IsNotExist(err) {
		return "", err
	}
	id := uuid.NewString()
	return id, atomicfile.Write(path, []byte(id+"\n"))
}
