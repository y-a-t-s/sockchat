package socket

import (
	"os"
	"testing"

	"github.com/joho/godotenv"
)

func TestSocket(t *testing.T) {
	if err := godotenv.Load(); err != nil {
		t.Error(err)
	}
	os.Setenv("SC_HOST", "kiwifarms.st")
	os.Setenv("SC_PORT", "9443")
	os.Setenv("SC_DEF_ROOM", "1")
	sock, err := NewSocket()
	if err != nil {
		t.Error(err)
	}

	err = sock.connect()
	if err != nil {
		t.Error(err)
	}

	sock.CloseAll()
}
