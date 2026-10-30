package service

import (
	"context"
	"sync/atomic"
	"time"

	"ctoup.com/coreapp/pkg/core/db"
	"ctoup.com/coreapp/pkg/core/db/repository"
	"ctoup.com/coreapp/pkg/shared/auth"
	"ctoup.com/coreapp/pkg/shared/util"
	"github.com/jackc/pgx/v5/pgtype"
)

// PersonalDataAccess is one read of SubjectUserID's personal data by
// ActorUserID. Resource names the kind of record as "<module>.<kind>"
// ("agent.run"); ResourceID, when there is one, which record. Never put
// the content read here. TenantID defaults to the request's tenant when ctx is
// the *gin.Context.
type PersonalDataAccess struct {
	TenantID      string
	ActorUserID   string
	SubjectUserID string
	Resource      string
	ResourceID    string
}

var personalDataAccessStore atomic.Pointer[db.Store]

// SetPersonalDataAccessStore points RecordPersonalDataAccess at the core
// database. The core server config does it at boot.
func SetPersonalDataAccessStore(store *db.Store) {
	personalDataAccessStore.Store(store)
}

// RecordPersonalDataAccess logs a staff read of another person's data. A read
// of one's own data is not logged. The write happens off the request path and
// a failure is only logged: the log must never slow or fail the read it
// records.
func RecordPersonalDataAccess(ctx context.Context, a PersonalDataAccess) {
	if a.ActorUserID == "" || a.SubjectUserID == "" || a.ActorUserID == a.SubjectUserID {
		return
	}
	logger := util.GetLoggerFromCtx(ctx)
	store := personalDataAccessStore.Load()
	if store == nil {
		logger.Warn().Str("resource", a.Resource).Msg("personal-data access not recorded: no store configured")
		return
	}
	if a.TenantID == "" {
		a.TenantID, _ = ctx.Value(auth.AUTH_TENANT_ID_KEY).(string)
	}

	go func() {
		writeCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		err := store.InsertPersonalDataAccess(writeCtx, repository.InsertPersonalDataAccessParams{
			TenantID:      a.TenantID,
			ActorUserID:   a.ActorUserID,
			SubjectUserID: a.SubjectUserID,
			Resource:      a.Resource,
			ResourceID:    pgtype.Text{String: a.ResourceID, Valid: a.ResourceID != ""},
		})
		if err != nil {
			logger.Err(err).Str("actor_user_id", a.ActorUserID).Str("subject_user_id", a.SubjectUserID).
				Str("resource", a.Resource).Msg("personal-data access not recorded")
		}
	}()
}
