package calendar

import (
	"errors"
	"fmt"
	"net/http"
	"strings"

	"golang.org/x/oauth2"
	"google.golang.org/api/googleapi"
)

// ErrNeedsReauth marks an account whose refresh token Google no longer accepts.
var ErrNeedsReauth = errors.New("google rejected the stored credentials; reconnect the account on /setup")

// Explain rewrites common Google API failures into actionable messages while
// keeping the original error in the chain.
func Explain(err error) error {
	if err == nil {
		return nil
	}
	var re *oauth2.RetrieveError
	if errors.As(err, &re) && strings.Contains(re.ErrorCode, "invalid_grant") {
		return fmt.Errorf("%w (%v)", ErrNeedsReauth, err)
	}
	var ge *googleapi.Error
	if !errors.As(err, &ge) {
		return err
	}
	switch ge.Code {
	case http.StatusNotFound:
		return fmt.Errorf("not found (check the event or calendar id): %w", err)
	case http.StatusForbidden:
		if strings.Contains(ge.Message, "has not been used") || strings.Contains(ge.Message, "disabled") {
			return fmt.Errorf("the Google Calendar API is not enabled for this OAuth project: %w", err)
		}
		return fmt.Errorf("permission denied for this calendar: %w", err)
	case http.StatusGone:
		return fmt.Errorf("event was already deleted: %w", err)
	}
	return err
}
