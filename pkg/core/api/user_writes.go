package core

import (
	"net/http"

	"ctoup.com/coreapp/api/helpers"
	core "ctoup.com/coreapp/api/openapi/core"
	"ctoup.com/coreapp/pkg/core/db/repository"
	"ctoup.com/coreapp/pkg/shared/auth"
	"ctoup.com/coreapp/pkg/shared/util"
	"github.com/gin-gonic/gin"
)

// The write and destructive operations, written once. See user_scope.go for
// the seam, and the guard table below for what survived the merge.
//
// This is the cluster the audit was actually about. DeleteUser was 87 lines and
// DeleteUserFromSuperAdmin was 26, and the difference was not compression: the
// admin path refused self-deletion and refused acting on somebody who outranks
// you, and the super-admin path did neither — while HardDeleteUserFromSuperAdmin,
// one function away in the same file, already refused self-deletion. Those
// guards were aligned first, on their own, with a test per path, so that this
// merge has a behaviour to preserve rather than one to invent.
//
//	guard                   admin   super-admin   why
//	─────────────────────────────────────────────────────────────────────────
//	IsAllowedToManageTenant    —        yes       "may you act on THIS tenant?"
//	                                              — the admin path answers it
//	                                              by having no choice of tenant
//	HasAdminPrivileges        yes       yes       redundant with the path gate;
//	                                              kept as defence in depth, the
//	                                              trade this repo already makes
//	                                              for tenant filters in SQL
//	rank (HasRightsForRoles)  yes       yes       NOT vacuous on the super-admin
//	                                              path: resellers reach it, and
//	                                              must not act on someone above
//	                                              them
//	self                      yes       yes       removing yourself from the
//	                                              tenant you administer is the
//	                                              same footgun at either level

func (o *userOps) addUser(c *gin.Context, scope TenantScope) {
	logger := util.GetLoggerFromCtx(c.Request.Context())

	target, ok := resolve(c, scope)
	if !ok {
		return
	}

	var req core.AddUserJSONRequestBody
	if err := c.ShouldBindJSON(&req); err != nil {
		logger.Err(err).Msg("Failed to bind JSON")
		c.JSON(http.StatusBadRequest, helpers.ErrorResponse(err))
		return
	}
	if err := auth.HasRightsForRoles(c, req.Roles); err != nil {
		logger.Err(err).Msg("User does not have rights for the requested roles")
		c.JSON(http.StatusUnauthorized, helpers.ErrorResponse(err))
		return
	}

	client, ok := target.client(c)
	if !ok {
		return
	}

	silent := req.Silent != nil && *req.Silent
	MarkSilent(c, silent)

	user, err := o.userService.CreateUser(c, client, target.id, req, nil)
	if err != nil {
		logger.Err(err).Msg("Failed to add user")
		// A duplicate email is a 409 with a code the frontend can act on. The
		// super-admin copy of this handler answered 500 — same mistake, same
		// operation, unusable error. Nobody decided that; it is what a fork
		// costs.
		if auth.IsEmailAlreadyExists(err) {
			c.JSON(http.StatusConflict, gin.H{
				"code":    auth.ErrorCodeEmailAlreadyExists,
				"message": "A user with this email address already exists.",
			})
			return
		}
		c.JSON(http.StatusInternalServerError, helpers.ErrorResponse(err))
		return
	}

	if !silent {
		url, err := getWelcomeEmailURL(c, target.subdomain)
		if err != nil {
			logger.Err(err).Msg("Failed to get welcome email URL")
			c.JSON(http.StatusInternalServerError, helpers.ErrorResponse(err))
			return
		}
		if err := sendWelcomeEmail(c, client, url, req.Email); err != nil {
			logger.Err(err).Msg("Failed to send welcome email")
			c.JSON(http.StatusInternalServerError, helpers.ErrorResponse(err))
			return
		}
	}

	c.JSON(http.StatusCreated, user)
}

func (o *userOps) updateUser(c *gin.Context, scope TenantScope, userID string) {
	logger := util.GetLoggerFromCtx(c.Request.Context())

	target, ok := resolve(c, scope)
	if !ok {
		return
	}

	var req core.UpdateUserJSONRequestBody
	if err := c.ShouldBindJSON(&req); err != nil {
		logger.Err(err).Msg("Failed to bind JSON")
		c.JSON(http.StatusBadRequest, helpers.ErrorResponse(err))
		return
	}
	if err := auth.HasRightsForRoles(c, req.Roles); err != nil {
		logger.Err(err).Msg("User does not have rights for the requested roles")
		c.JSON(http.StatusUnauthorized, helpers.ErrorResponse(err))
		return
	}

	client, ok := target.client(c)
	if !ok {
		return
	}
	if err := o.userService.UpdateUser(c, client, target.id, userID, req); err != nil {
		logger.Err(err).Msg("Failed to update user")
		c.JSON(http.StatusInternalServerError, helpers.ErrorResponse(err))
		return
	}
	c.Status(http.StatusNoContent)
}

// deleteUser removes a user from the scoped tenant — or, on the root domain
// where there is no tenant, deletes them outright.
//
// That branch is the reason two endpoints named "delete" delete different
// amounts, and it is preserved rather than tidied: the /admin-api endpoint has
// always meant "delete globally" when a SUPER_ADMIN invokes it with no tenant,
// and there is no tenantless super-admin route to move that to. Making both
// endpoints mean "remove from tenant" would silently turn an existing global
// delete into a membership removal.
func (o *userOps) deleteUser(c *gin.Context, scope TenantScope, userID string) {
	logger := util.GetLoggerFromCtx(c.Request.Context())

	// FIRST, ahead of the tenant lookup: this needs nothing but the caller's own
	// identity, so refusing here saves a round trip on a request that was never
	// going to succeed — and it makes the guard testable without a database.
	if !o.refuseSelf(c, userID, "Cannot delete self") {
		return
	}
	if !o.requireAdminPrivileges(c, "delete user") {
		return
	}

	target, ok := resolve(c, scope)
	if !ok {
		return
	}

	if target.id == "" {
		o.deleteUserGlobally(c, userID)
		return
	}

	if !o.refuseIfTargetOutranks(c, target.id, userID) {
		return
	}
	client, ok := target.client(c)
	if !ok {
		return
	}
	if err := o.userService.RemoveUserFromTenant(c, client, target.id, userID); err != nil {
		if helpers.AbortIfReferenced(c, err, "USER_IN_USE",
			"user is referenced by other records and cannot be deleted") {
			return
		}
		logger.Err(err).Msg("Failed to delete user")
		c.JSON(http.StatusInternalServerError, helpers.ErrorResponse(err))
		return
	}
	c.Status(http.StatusNoContent)
}

// deleteUserGlobally is the root-domain branch: no tenant, so no membership to
// remove and no tenant-scoped roles to rank against. Super admins only.
func (o *userOps) deleteUserGlobally(c *gin.Context, userID string) {
	logger := util.GetLoggerFromCtx(c.Request.Context())

	if !auth.IsSuperAdmin(c) {
		logger.Error().Msg("Only SUPER_ADMIN can delete user without tenant")
		c.JSON(http.StatusUnauthorized, helpers.ErrorStringResponse("Only SUPER_ADMIN can delete user without tenant"))
		return
	}
	user, err := o.userService.GetUserByID(c, userID)
	if err != nil {
		logger.Err(err).Msg("failed to get user by ID")
		c.JSON(http.StatusInternalServerError, helpers.ErrorResponse(err))
		return
	}
	// Global roles, because there is no tenant to scope them to.
	if err := auth.HasRightsForRoles(c, user.Roles); err != nil {
		logger.Err(err).Msg("user does not have rights to be deleted")
		c.JSON(http.StatusUnauthorized, helpers.ErrorResponse(err))
		return
	}

	subdomain, err := util.GetSubdomain(c)
	if err != nil {
		logger.Err(err).Msg("Failed to get subdomain")
		c.JSON(http.StatusBadRequest, helpers.ErrorResponse(err))
		return
	}
	client, err := o.authProvider.GetAuthClientForSubdomain(c, subdomain)
	if err != nil {
		logger.Err(err).Msg("Failed to get auth client for subdomain")
		c.JSON(http.StatusBadRequest, helpers.ErrorResponse(err))
		return
	}
	if err := o.userService.DeleteUser(c, client, userID); err != nil {
		if helpers.AbortIfReferenced(c, err, "USER_IN_USE",
			"user is referenced by other records and cannot be deleted") {
			return
		}
		logger.Err(err).Msg("Failed to delete user")
		c.JSON(http.StatusInternalServerError, helpers.ErrorResponse(err))
		return
	}
	c.Status(http.StatusNoContent)
}

// removeUserFromTenant deletes the membership and nothing else. Unlike
// deleteUser it has no root-domain branch — there is no membership to remove
// when there is no tenant — so an absent tenant is a 404 rather than a global
// delete.
func (o *userOps) removeUserFromTenant(c *gin.Context, scope TenantScope, userID string) {
	logger := util.GetLoggerFromCtx(c.Request.Context())

	if !o.refuseSelf(c, userID, "Cannot remove self from tenant") {
		return
	}
	if !o.requireAdminPrivileges(c, "remove user from tenant") {
		return
	}

	target, ok := resolve(c, scope)
	if !ok {
		return
	}

	isMember, err := o.store.IsUserMemberOfTenant(c, repository.IsUserMemberOfTenantParams{
		UserID:   userID,
		TenantID: target.id,
	})
	if err != nil || !isMember {
		logger.Err(err).Msg("failed to check user membership")
		c.JSON(http.StatusNotFound, helpers.ErrorStringResponse("user not found in this tenant"))
		return
	}
	if !o.refuseIfTargetOutranks(c, target.id, userID) {
		return
	}
	client, ok := target.client(c)
	if !ok {
		return
	}
	if err := o.userService.RemoveUserFromTenant(c, client, target.id, userID); err != nil {
		logger.Err(err).Msg("Failed to remove user from tenant")
		c.JSON(http.StatusInternalServerError, helpers.ErrorResponse(err))
		return
	}
	c.Status(http.StatusNoContent)
}

// refuseSelf reports whether to continue.
func (o *userOps) refuseSelf(c *gin.Context, userID, message string) bool {
	if callerID := c.GetString(auth.AUTH_USER_ID); callerID != "" && callerID == userID {
		logger := util.GetLoggerFromCtx(c.Request.Context())
		logger.Error().Msg(message)
		c.JSON(http.StatusForbidden, helpers.ErrorStringResponse(message))
		return false
	}
	return true
}

func (o *userOps) requireAdminPrivileges(c *gin.Context, what string) bool {
	if auth.HasAdminPrivileges(c) {
		return true
	}
	message := "Only RESELLER, CUSTOMER_ADMIN, ADMIN or SUPER_ADMIN can " + what
	logger := util.GetLoggerFromCtx(c.Request.Context())
	logger.Error().Msg(message)
	c.JSON(http.StatusUnauthorized, helpers.ErrorStringResponse(message))
	return false
}

// refuseIfTargetOutranks reads the target's TENANT-SCOPED roles rather than
// their global ones. A user with no membership has no roles here and is not
// refused — the membership check, where one applies, is the operation's job.
func (o *userOps) refuseIfTargetOutranks(c *gin.Context, tenantID, userID string) bool {
	logger := util.GetLoggerFromCtx(c.Request.Context())

	roles, err := o.store.GetUserTenantRoles(c, repository.GetUserTenantRolesParams{
		UserID:   userID,
		TenantID: tenantID,
	})
	if err != nil {
		logger.Err(err).Msg("failed to get user roles")
		c.JSON(http.StatusInternalServerError, helpers.ErrorResponse(err))
		return false
	}
	if len(roles) == 0 {
		return true
	}
	target := make([]core.Role, len(roles))
	for i, r := range roles {
		target[i] = core.Role(r)
	}
	if err := auth.HasRightsForRoles(c, target); err != nil {
		logger.Err(err).Msg("caller does not have rights over this user")
		c.JSON(http.StatusForbidden, helpers.ErrorResponse(err))
		return false
	}
	return true
}

// addUserMembership adds an EXISTING user to the scoped tenant.
//
// The rank guard is applied on both paths now, where it lived only on the
// /admin-api one. That is the security-relevant half of this merge: resellers
// reach the super-admin route, and the super-admin copy checked nothing about
// the roles being granted — so a reseller could add a user to a tenant it
// resells with a role above its own. The /admin-api twin refused exactly that,
// twelve lines away in another file.
func (o *userOps) addUserMembership(c *gin.Context, scope TenantScope, userID string, roles []core.Role) {
	logger := util.GetLoggerFromCtx(c.Request.Context())

	byUserID := c.GetString(auth.AUTH_USER_ID)
	if byUserID == "" {
		logger.Error().Msg("ByUserID not found")
		c.JSON(http.StatusInternalServerError, helpers.ErrorStringResponse("ByUserID not found"))
		return
	}

	target, ok := resolve(c, scope)
	if !ok {
		return
	}
	if err := auth.HasRightsForRoles(c, roles); err != nil {
		logger.Err(err).Msg("User does not have rights for the requested roles")
		c.JSON(http.StatusUnauthorized, helpers.ErrorResponse(err))
		return
	}

	isMember, err := o.store.IsUserMemberOfTenant(c, repository.IsUserMemberOfTenantParams{
		UserID:   userID,
		TenantID: target.id,
	})
	if err != nil {
		logger.Err(err).Msg("Failed to check tenant membership")
		c.JSON(http.StatusInternalServerError, helpers.ErrorResponse(err))
		return
	}
	if isMember {
		logger.Error().Str("userID", userID).Str("tenantID", target.id).Msg("User is already a member of this tenant")
		c.JSON(http.StatusBadRequest, helpers.ErrorStringResponse("User is already a member of this tenant"))
		return
	}

	client, ok := target.client(c)
	if !ok {
		return
	}
	if err := o.userService.AddUserToTenant(c, client, target.id, userID, roles, byUserID); err != nil {
		logger.Err(err).Msg("Failed to add user to tenant")
		c.JSON(http.StatusInternalServerError, helpers.ErrorResponse(err))
		return
	}

	user, err := o.userService.GetUserByTenantIDByID(c, target.id, userID)
	if err != nil {
		logger.Err(err).Msg("Failed to get user after adding membership")
		c.JSON(http.StatusInternalServerError, helpers.ErrorResponse(err))
		return
	}

	// The notification is best-effort: the membership exists either way, and
	// failing the request would leave the caller unable to tell whether it did.
	if target.name != "" {
		// A sign-in link for the tenant they were just added to, which is what
		// the /admin-api path sent; the super-admin path sent the tenant root,
		// where an unauthenticated visitor lands on a sign-in page anyway.
		url, err := getResetPasswordURL(c, target.subdomain)
		if err != nil {
			logger.Err(err).Msg("Failed to get URL for notification")
		} else if err := sendTenantAddedEmail(c, client, url, user.Email, target.name); err != nil {
			logger.Err(err).Msg("Failed to send tenant added notification")
		}
	}

	c.JSON(http.StatusCreated, user)
}
