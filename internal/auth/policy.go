package auth

import (
	pbcore "ganigeorgiev.com/pocketbase/pocketbase/core"
	"ganigeorgiev.com/pocketbase/pocketbase/tools/hook"
)

// registerAuthPolicy applies application eligibility to PocketBase's native
// issuance pipeline too. An empty auth method is token refresh, which already
// proves possession of a token issued after the application's second factor.
func (o *Auth) registerAuthPolicy() {
	// The application reset flow owns eligibility, localized mail and login UX.
	o.pb.OnRecordRequestPasswordResetRequest(o.authCol).Bind(&hook.Handler[*pbcore.RecordRequestPasswordResetRequestEvent]{Id: "listpocket-reset-request-policy", Func: func(e *pbcore.RecordRequestPasswordResetRequestEvent) error {
		return e.ForbiddenError("Use the application password reset flow.", nil)
	}})
	o.pb.OnRecordConfirmPasswordResetRequest(o.authCol).Bind(&hook.Handler[*pbcore.RecordConfirmPasswordResetRequestEvent]{Id: "listpocket-reset-confirm-policy", Func: func(e *pbcore.RecordConfirmPasswordResetRequestEvent) error {
		return e.ForbiddenError("Use the application password reset flow.", nil)
	}})

	o.pb.OnRecordAuthRequest(o.authCol).Bind(&hook.Handler[*pbcore.RecordAuthRequestEvent]{
		Id: "listpocket-auth-policy",
		Func: func(e *pbcore.RecordAuthRequestEvent) error {
			if e.Record.GetString("status") != UserStatusEnabled || e.Record.GetString("user_type") != UserTypeUser {
				return e.ForbiddenError("User is not eligible to authenticate.", nil)
			}
			if e.AuthMethod != "" {
				if e.Record.GetString("twofa_type") != "" || (e.AuthMethod == pbcore.MFAMethodPassword && !e.Record.GetBool("password_login")) {
					return e.ForbiddenError("Use the application login flow.", nil)
				}
			}
			return e.Next()
		},
	})
}
