package server

import (
	"testing"

	"github.com/stretchr/testify/require"
	"heckel.io/ntfy/v2/client"
)

func TestErrors_AdminCodesMatchClient(t *testing.T) {
	codes := map[int]*errHTTP{
		client.AdminErrCodeNotFound:               errHTTPNotFound,
		client.AdminErrCodeUnauthorized:           errHTTPUnauthorized,
		client.AdminErrCodeForbidden:              errHTTPForbidden,
		client.AdminErrCodeAdminSocketOnly:        errHTTPForbiddenAdminSocketOnly,
		client.AdminErrCodeUserNotFound:           errHTTPBadRequestUserNotFound,
		client.AdminErrCodeUserExists:             errHTTPConflictUserExists,
		client.AdminErrCodeTierNotFound:           errHTTPBadRequestTierInvalid,
		client.AdminErrCodeTierExists:             errHTTPConflictTierExists,
		client.AdminErrCodeTierInUse:              errHTTPConflictTierInUse,
		client.AdminErrCodeTierCodeInvalid:        errHTTPBadRequestTierCodeInvalid,
		client.AdminErrCodeTierReservationsLimit:  errHTTPBadRequestTierReservationsLimit,
		client.AdminErrCodeAccessAdminUser:        errHTTPBadRequestAccessAdminUser,
		client.AdminErrCodeTokenNotFound:          errHTTPBadRequestTokenNotFound,
		client.AdminErrCodeRoleInvalid:            errHTTPBadRequestRoleInvalid,
		client.AdminErrCodeUserNoPrimaryEmail:     errHTTPBadRequestUserNoPrimaryEmail,
		client.AdminErrCodeBaseURLNotConfigured:   errHTTPBadRequestBaseURLNotConfigured,
		client.AdminErrCodePasswordHashInvalid:    errHTTPBadRequestPasswordHashInvalid,
		client.AdminErrCodePermissionInvalid:      errHTTPBadRequestPermissionInvalid,
		client.AdminErrCodeProvisionedUserChange:  errHTTPConflictProvisionedUserChange,
		client.AdminErrCodeProvisionedTokenChange: errHTTPConflictProvisionedTokenChange,
	}
	for code, err := range codes {
		require.Equal(t, err.Code, code, err.Message)
	}
}
