package testutils

import (
	"context"
	"fmt"
	"testing"

	"ctoup.com/coreapp/internal/testutils"
	"ctoup.com/coreapp/pkg/core/db/repository"
	"ctoup.com/coreapp/pkg/shared/repository/subentity"
	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/require"
)

func Test_CreateUser(t *testing.T) {
	params := repository.CreateUserParams{
		ID: testutils.RandomOwner(),
		Profile: subentity.UserProfile{
			About:     testutils.RandomAbout(),
			Interests: testutils.RandomInterests(1, 3),
		},
		TenantID: testutils.RandomTenant(),
	}

	user, err := testStore.CreateUser(context.Background(), params)
	require.NoError(t, err)
	require.NotEmpty(t, user)
	require.Equal(t, params.ID, user.ID)
	require.Equal(t, params.Profile.Interests, user.Profile.Interests)
	require.Equal(t, params.Profile.About, user.Profile.About)
	require.NotZero(t, user.ID)
	require.NotZero(t, user.CreatedAt)

}

func createRandomUser(tenantID string, t *testing.T) repository.CoreUser {
	params := repository.CreateUserParams{
		ID: testutils.RandomOwner(),
		Profile: subentity.UserProfile{
			About:     testutils.RandomAbout(),
			Interests: testutils.RandomInterests(1, 3),
		},
		TenantID: tenantID,
	}

	user, err := testStore.CreateUser(context.Background(), params)
	require.NoError(t, err)
	require.NotEmpty(t, user)
	require.Equal(t, params.ID, user.ID)
	require.Equal(t, params.Profile.Interests, user.Profile.Interests)
	require.Equal(t, params.Profile.About, user.Profile.About)
	require.NotZero(t, user.ID)
	require.NotZero(t, user.CreatedAt)
	return user
}

func Test_GetUser(t *testing.T) {
	tenantID := testutils.RandomTenant()
	user1 := createRandomUser(tenantID, t)
	user2, err := testStore.GetUserByID(context.Background(), user1.ID)
	require.NoError(t, err)
	require.NotEmpty(t, user2)
	require.Equal(t, user1.ID, user2.ID)
	require.Equal(t, user1.Profile.Interests, user2.Profile.Interests)
	require.Equal(t, user1.Profile.About, user2.Profile.About)
	require.Equal(t, user1.ID, user2.ID)
	require.Equal(t, user1.CreatedAt, user2.CreatedAt)
}
func Test_UpdateUser(t *testing.T) {
	tenantID := testutils.RandomTenant()
	user1 := createRandomUser(tenantID, t)

	params := repository.UpdateProfileParams{
		ID: user1.ID,
		//  to change
		Profile: subentity.UserProfile{
			About:     testutils.RandomAbout(),
			Interests: testutils.RandomInterests(1, 3),
		},
		TenantID: tenantID,
	}
	_, err := testStore.UpdateProfile(context.Background(), params)
	require.NoError(t, err)

	user2, err := testStore.GetUserByID(context.Background(), user1.ID)
	require.NoError(t, err)
	require.NotEmpty(t, user2)
	require.Equal(t, user1.ID, user2.ID)
	require.Equal(t, user1.ID, user2.ID)
	require.Equal(t, params.Profile.Interests, user2.Profile.Interests)
	require.Equal(t, params.Profile.About, user2.Profile.About)

	require.Equal(t, user1.CreatedAt, user2.CreatedAt)
}
func Test_DeleteUser(t *testing.T) {
	tenantID := testutils.RandomTenant()
	user := createRandomUser(tenantID, t)
	_, err := testStore.DeleteUser(context.Background(),
		repository.DeleteUserParams{
			ID:       user.ID,
			TenantID: tenantID,
		},
	)
	require.NoError(t, err)
	user2, err := testStore.GetUserByID(context.Background(), user.ID)
	require.EqualError(t, err, pgx.ErrNoRows.Error())
	require.Empty(t, user2)
}

func Test_DeleteUserFailsWhenNotTheUser(t *testing.T) {
	tenantID := testutils.RandomTenant()
	user := createRandomUser(tenantID, t)

	fmt.Println(user.ID)

	iid := testutils.RandomOwner()
	fmt.Println(iid)

	_, err := testStore.DeleteUser(context.Background(), repository.DeleteUserParams{
		ID:       iid,
		TenantID: tenantID,
	})

	require.EqualError(t, err, pgx.ErrNoRows.Error())

	user2, err := testStore.GetUserByID(context.Background(), user.ID)
	require.NoError(t, err)
	require.NotEmpty(t, user2)
}

func Test_GetListUser(t *testing.T) {
	t.Helper()
	tenantID := testutils.RandomTenant()
	for i := 0; i < 10; i++ {
		createRandomUser(tenantID, t)
	}
	params := repository.ListUsersParams{
		Limit:    5,
		Offset:   5,
		TenantID: tenantID,
	}
	users, err := testStore.ListUsers(context.Background(), params)
	require.NoError(t, err)
	require.Len(t, users, 5)

	for _, user := range users {
		require.NotEmpty(t, user)
	}
}
