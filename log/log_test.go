package log_test

import (
	"heckel.io/ntfy/log"
	"net/http"
	"testing"
)

const tagPay = "PAY"

type visitor struct {
	UserID string
	IP     string
}

func (v *visitor) Context() map[string]any {
	return map[string]any{
		"user_id": v.UserID,
		"ip":      v.IP,
	}
}

func TestEvent_Info(t *testing.T) {
	/*
		log-level: INFO, user_id:u_abc=DEBUG
		log-level-overrides:
			- user_id=u_abc: DEBUG
		log-filter =

	*/
	v := &visitor{
		UserID: "u_abc",
		IP:     "1.2.3.4",
	}
	stripeCtx := log.NewCtx(map[string]any{
		"tag": "pay",
	})
	log.SetLevel(log.InfoLevel)
	//log.SetFormat(log.JSONFormat)
	//log.SetLevelOverride("user_id", "u_abc", log.DebugLevel)
	log.SetLevelOverride("tag", "pay", log.DebugLevel)
	mlog := log.Field("tag", "manager")
	mlog.Field("one", 1).Info("this is one")
	mlog.Err(http.ErrHandlerTimeout).Field("two", 2).Info("this is two")
	log.Info("somebody did something")
	log.
		Context(stripeCtx, v).
		Fields(map[string]any{
			"tier":    "ti_abc",
			"user_id": "u_abc",
		}).
		Debug("Somebody paid something for $%d", 10)
	log.
		Field("tag", "account").
		Field("user_id", "u_abc").
		Debug("User logged in")
}
