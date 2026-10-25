package tasks

import (
	"errors"
	"fmt"
	"net/http"
	"strings"

	"golang.org/x/oauth2"
	"google.golang.org/api/googleapi"

	"github.com/tr4m0ryp/oido-google-calander/internal/calendar"
)

// ErrScopeMissing marks an account whose grant lacks the Google Tasks scope,
// typically because it was connected before Tasks support existed.
var ErrScopeMissing = errors.New("this account has not granted Google Tasks access")

// MissingScope builds the error tools return for such an account, telling
// the user exactly how to fix it.
func MissingScope(email, setupURL string) error {
	if setupURL == "" {
		setupURL = "the /setup page of this server"
	}
	return fmt.Errorf("%w: %s was connected without the Google Tasks permission. Reconnect it once: "+
		"open %s, sign in, click \"Reconnect to enable Tasks\" on %s and approve the Tasks permission "+
		"(the account is updated in place)", ErrScopeMissing, email, setupURL, email)
}

// Explain rewrites common Google API failures into actionable messages while
// keeping the original error in the chain.
func Explain(err error) error {
	if err == nil {
		return nil
	}
	var re *oauth2.RetrieveError
	if errors.As(err, &re) && strings.Contains(re.ErrorCode, "invalid_grant") {
		return fmt.Errorf("%w (%v)", calendar.ErrNeedsReauth, err)
	}
	var ge *googleapi.Error
	if !errors.As(err, &ge) {
		return err
	}
	switch ge.Code {
	case http.StatusNotFound:
		return fmt.Errorf("not found (check the task id and task list id): %w", err)
	case http.StatusForbidden, http.StatusUnauthorized:
		if insufficientScope(ge) {
			return fmt.Errorf("%w: reconnect the account on /setup (%v)", ErrScopeMissing, err)
		}
		if strings.Contains(ge.Message, "has not been used") || strings.Contains(ge.Message, "disabled") {
			return fmt.Errorf("the Google Tasks API is not enabled for this OAuth project "+
				"(gcloud services enable tasks.googleapis.com): %w", err)
		}
		return fmt.Errorf("permission denied for this task list: %w", err)
	}
	return err
}

func insufficientScope(ge *googleapi.Error) bool {
	for _, item := range ge.Errors {
		if item.Reason == "insufficientPermissions" {
			return true
		}
	}
	msg := strings.ToLower(ge.Message)
	return strings.Contains(msg, "insufficient authentication scopes") ||
		strings.Contains(msg, "access_token_scope_insufficient")
}
