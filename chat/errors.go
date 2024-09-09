package chat

import (
	"fmt"
)

type ErrSessionExpired struct {
	serverMsg string
}

func (err *ErrSessionExpired) Error() string {
	return err.serverMsg
}

type errSocketClosed struct {
	sock *sock
}

func (e *errSocketClosed) Error() string {
	return "Socket is closed."
}

type errLogWriterClosed struct {
	fname string
}

func (e *errLogWriterClosed) Error() string {
	return fmt.Sprintf("Log writer for %s is closed.", e.fname)
}

type errReadTimedOut struct {
	room uint
}

func (e *errReadTimedOut) Error() string {
	return fmt.Sprintf("Msg read timed out in room %d.", e.room)
}
