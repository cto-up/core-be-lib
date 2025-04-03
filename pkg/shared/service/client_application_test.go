package service

import (
	"testing"

	commontestutils "ctoup.com/coreapp/internal/testutils"
	"ctoup.com/coreapp/pkg/core/db"
	"ctoup.com/coreapp/pkg/core/db/repository"

	"ctoup.com/coreapp/pkg/core/db/testutils"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func setupTestClientApplicationService(t *testing.T) (*ClientApplicationService, *db.Store) {
	store := testutils.NewTestStore(t)
	service := NewClientApplicationService(store)
	return service, store
}

func createTestClientApplication(t *testing.T, service *ClientApplicationService) repository.CoreClientApplication {
	ctx := &gin.Context{}
	tenantID := commontestutils.RandomString(10) // Add random tenant ID
	name := commontestutils.RandomString(10)
	description := commontestutils.RandomString(20)
	createdBy := commontestutils.RandomString(10)

	app, err := service.CreateClientApplication(ctx, tenantID, name, description, createdBy)
	require.NoError(t, err)
	require.NotNil(t, app)

	return app
}

func TestCreateClientApplication(t *testing.T) {
	service, _ := setupTestClientApplicationService(t)
	ctx := &gin.Context{}
	tenantID := commontestutils.RandomString(10) // Add random tenant ID

	t.Run("successful creation", func(t *testing.T) {
		name := commontestutils.RandomString(10)
		description := commontestutils.RandomString(20)
		createdBy := commontestutils.RandomString(10)

		app, err := service.CreateClientApplication(ctx, tenantID, name, description, createdBy)
		require.NoError(t, err)
		require.NotNil(t, app)
		require.Equal(t, name, app.Name)
		require.Equal(t, description, app.Description.String)
		require.Equal(t, createdBy, app.CreatedBy)
		require.True(t, app.Active)
	})
}

func TestGetClientApplication(t *testing.T) {
	service, _ := setupTestClientApplicationService(t)
	ctx := &gin.Context{}

	t.Run("existing application", func(t *testing.T) {
		app := createTestClientApplication(t, service)

		found, err := service.GetClientApplicationByID(ctx, app.ID, app.TenantID.String)
		require.NoError(t, err)
		require.NotNil(t, found)
		require.Equal(t, app.ID, found.ID)
		require.Equal(t, app.Name, found.Name)
	})

	tenantID := commontestutils.RandomString(10) // Add random tenant ID

	t.Run("non-existing application", func(t *testing.T) {
		_, err := service.GetClientApplicationByID(ctx, uuid.New(), tenantID)
		require.Error(t, err)
	})
}

func TestUpdateClientApplication(t *testing.T) {
	service, _ := setupTestClientApplicationService(t)
	ctx := &gin.Context{}

	t.Run("successful update", func(t *testing.T) {
		app := createTestClientApplication(t, service)

		newName := commontestutils.RandomString(10)
		newDescription := commontestutils.RandomString(20)

		updated, err := service.UpdateClientApplication(ctx, app.ID, app.TenantID.String, newName, newDescription, true)
		require.NoError(t, err)
		require.Equal(t, newName, updated.Name)
		require.Equal(t, newDescription, updated.Description.String)
	})
	tenantID := commontestutils.RandomString(10) // Add random tenant ID

	t.Run("non-existing application", func(t *testing.T) {
		_, err := service.UpdateClientApplication(ctx, uuid.New(), tenantID, "name", "description", true)
		require.Error(t, err)
	})
}

func TestDeactivateClientApplication(t *testing.T) {
	service, _ := setupTestClientApplicationService(t)
	ctx := &gin.Context{}

	t.Run("successful deactivation", func(t *testing.T) {
		app := createTestClientApplication(t, service)

		err := service.DeactivateClientApplication(ctx, app.ID, app.TenantID.String)

		require.NoError(t, err)

	})
	tenantID := commontestutils.RandomString(10) // Add random tenant ID

	t.Run("non-existing application", func(t *testing.T) {
		err := service.DeactivateClientApplication(ctx, uuid.New(), tenantID)
		require.Error(t, err)
	})
}
