package localav

import (
	"errors"
	"fmt"
)

// ErrUnitialized is returned when Player methods are called before Init.
var ErrUnitialized = errors.New("localav player not initialized")

func errorf(format string, args ...any) error {
	return fmt.Errorf(format, args...)
}
