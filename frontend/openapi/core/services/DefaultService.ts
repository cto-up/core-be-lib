/* generated using openapi-typescript-codegen -- do no edit */
/* istanbul ignore file */
/* tslint:disable */
/* eslint-disable */
import type { APIToken } from '../models/APIToken';
import type { APITokenAuditLog } from '../models/APITokenAuditLog';
import type { APITokenCreated } from '../models/APITokenCreated';
import type { APITokenRevoke } from '../models/APITokenRevoke';
import type { ClientApplication } from '../models/ClientApplication';
import type { Config } from '../models/Config';
import type { NewAPIToken } from '../models/NewAPIToken';
import type { NewClientApplication } from '../models/NewClientApplication';
import type { NewConfig } from '../models/NewConfig';
import type { NewRole } from '../models/NewRole';
import type { NewTenant } from '../models/NewTenant';
import type { NewUser } from '../models/NewUser';
import type { PublicTenantSchema } from '../models/PublicTenantSchema';
import type { Role } from '../models/Role';
import type { Tenant } from '../models/Tenant';
import type { TenantFeatures } from '../models/TenantFeatures';
import type { TenantProfileSchema } from '../models/TenantProfileSchema';
import type { User } from '../models/User';
import type { UserActionSchema } from '../models/UserActionSchema';
import type { UserProfileSchema } from '../models/UserProfileSchema';

import type { CancelablePromise } from '../core/CancelablePromise';
import { OpenAPI } from '../core/OpenAPI';
import { request as __request } from '../core/request';

export class DefaultService {

    /**
     * Returns all global_configs from the system that the user has access to
     *
     * @param page page number
     * @param pageSize maximum number of results to return
     * @param sortBy field to sort by
     * @param order sort order
     * @param q starts with
     * @param detail basic or full
     * @returns Config global_config response
     * @throws ApiError
     */
    public static listGlobalConfigs(
        page?: number,
        pageSize?: number,
        sortBy?: string,
        order?: 'asc' | 'desc',
        q?: string,
        detail?: string,
    ): CancelablePromise<Array<Config>> {
        return __request(OpenAPI, {
            method: 'GET',
            url: '/superadmin-api/v1/configs/global-configs',
            query: {
                'page': page,
                'pageSize': pageSize,
                'sortBy': sortBy,
                'order': order,
                'q': q,
                'detail': detail,
            },
        });
    }

    /**
     * Creates a new global_config in the store. Duplicates are allowed
     * @param requestBody GlobalConfig to add to the store
     * @returns Config global_config response
     * @throws ApiError
     */
    public static addGlobalConfig(
        requestBody: NewConfig,
    ): CancelablePromise<Config> {
        return __request(OpenAPI, {
            method: 'POST',
            url: '/superadmin-api/v1/configs/global-configs',
            body: requestBody,
            mediaType: 'application/json',
        });
    }

    /**
     * Returns a global config based on a single ID, if the user does not have access to the global config
     * @param id ID of global config to fetch
     * @returns Config global config response
     * @throws ApiError
     */
    public static getGlobalConfigById(
        id: string,
    ): CancelablePromise<Config> {
        return __request(OpenAPI, {
            method: 'GET',
            url: '/superadmin-api/v1/configs/global-configs/{id}',
            path: {
                'id': id,
            },
        });
    }

    /**
     * Updates a global config in the store.
     * @param id ID of global config to fetch
     * @param requestBody Global config to add to the store
     * @returns Config global config response
     * @throws ApiError
     */
    public static updateGlobalConfig(
        id: string,
        requestBody: Config,
    ): CancelablePromise<Config> {
        return __request(OpenAPI, {
            method: 'PUT',
            url: '/superadmin-api/v1/configs/global-configs/{id}',
            path: {
                'id': id,
            },
            body: requestBody,
            mediaType: 'application/json',
        });
    }

    /**
     * deletes a single global config based on the ID supplied
     * @param id ID of global config to delete
     * @returns void
     * @throws ApiError
     */
    public static deleteGlobalConfig(
        id: string,
    ): CancelablePromise<void> {
        return __request(OpenAPI, {
            method: 'DELETE',
            url: '/superadmin-api/v1/configs/global-configs/{id}',
            path: {
                'id': id,
            },
        });
    }

    /**
     * Returns all tenant_configs from the system that the user has access to
     *
     * @param page page number
     * @param pageSize maximum number of results to return
     * @param sortBy field to sort by
     * @param order sort order
     * @param q starts with
     * @param detail basic or full
     * @returns Config tenant_config response
     * @throws ApiError
     */
    public static listTenantConfigs(
        page?: number,
        pageSize?: number,
        sortBy?: string,
        order?: 'asc' | 'desc',
        q?: string,
        detail?: string,
    ): CancelablePromise<Array<Config>> {
        return __request(OpenAPI, {
            method: 'GET',
            url: '/api/v1/configs/tenant-configs',
            query: {
                'page': page,
                'pageSize': pageSize,
                'sortBy': sortBy,
                'order': order,
                'q': q,
                'detail': detail,
            },
        });
    }

    /**
     * Creates a new tenant_config in the store. Duplicates are allowed
     * @param requestBody TenantConfig to add to the store
     * @returns Config tenant_config response
     * @throws ApiError
     */
    public static addTenantConfig(
        requestBody: NewConfig,
    ): CancelablePromise<Config> {
        return __request(OpenAPI, {
            method: 'POST',
            url: '/api/v1/configs/tenant-configs',
            body: requestBody,
            mediaType: 'application/json',
        });
    }

    /**
     * Returns a tenant config based on a single ID, if the user does not have access to the tenant config
     * @param id ID of tenant config to fetch
     * @returns Config tenant config response
     * @throws ApiError
     */
    public static getTenantConfigById(
        id: string,
    ): CancelablePromise<Config> {
        return __request(OpenAPI, {
            method: 'GET',
            url: '/api/v1/configs/tenant-configs/{id}',
            path: {
                'id': id,
            },
        });
    }

    /**
     * Updates a tenant config in the store.
     * @param id ID of tenant config to fetch
     * @param requestBody Tenant config to add to the store
     * @returns Config tenant config response
     * @throws ApiError
     */
    public static updateTenantConfig(
        id: string,
        requestBody: Config,
    ): CancelablePromise<Config> {
        return __request(OpenAPI, {
            method: 'PUT',
            url: '/api/v1/configs/tenant-configs/{id}',
            path: {
                'id': id,
            },
            body: requestBody,
            mediaType: 'application/json',
        });
    }

    /**
     * deletes a single tenant config based on the ID supplied
     * @param id ID of tenant config to delete
     * @returns void
     * @throws ApiError
     */
    public static deleteTenantConfig(
        id: string,
    ): CancelablePromise<void> {
        return __request(OpenAPI, {
            method: 'DELETE',
            url: '/api/v1/configs/tenant-configs/{id}',
            path: {
                'id': id,
            },
        });
    }

    /**
     * Returns a user based on a single ID, if the user does not have access to the user
     * @param userid ID of user to fetch
     * @returns User user response
     * @throws ApiError
     */
    public static getUserById(
        userid: string,
    ): CancelablePromise<User> {
        return __request(OpenAPI, {
            method: 'GET',
            url: '/api/v1/users/{userid}',
            path: {
                'userid': userid,
            },
        });
    }

    /**
     * Updates a new user in the store. Duplicates are allowed
     * @param userid ID of user to fetch
     * @param requestBody User to update to the store
     * @returns void
     * @throws ApiError
     */
    public static updateUser(
        userid: string,
        requestBody: User,
    ): CancelablePromise<void> {
        return __request(OpenAPI, {
            method: 'PUT',
            url: '/api/v1/users/{userid}',
            path: {
                'userid': userid,
            },
            body: requestBody,
            mediaType: 'application/json',
        });
    }

    /**
     * deletes a single user based on the ID supplied
     * @param userid ID of user to delete
     * @returns void
     * @throws ApiError
     */
    public static deleteUser(
        userid: string,
    ): CancelablePromise<void> {
        return __request(OpenAPI, {
            method: 'DELETE',
            url: '/api/v1/users/{userid}',
            path: {
                'userid': userid,
            },
        });
    }

    /**
     * Act on user
     * @param userid ID of user
     * @param requestBody Project to add to the store
     * @returns void
     * @throws ApiError
     */
    public static updateUserStatus(
        userid: string,
        requestBody: UserActionSchema,
    ): CancelablePromise<void> {
        return __request(OpenAPI, {
            method: 'POST',
            url: '/api/v1/users/{userid}/status',
            path: {
                'userid': userid,
            },
            body: requestBody,
            mediaType: 'application/json',
        });
    }

    /**
     * Assign a role to a user based
     * @param userid ID of user
     * @param roleid ID of role to assign
     * @returns void
     * @throws ApiError
     */
    public static assignRole(
        userid: string,
        roleid: string,
    ): CancelablePromise<void> {
        return __request(OpenAPI, {
            method: 'POST',
            url: '/api/v1/users/{userid}/roles/{roleid}/assign',
            path: {
                'userid': userid,
                'roleid': roleid,
            },
        });
    }

    /**
     * Unassign a role to a user based
     * @param userid ID of user
     * @param roleid ID of role to unassign
     * @returns void
     * @throws ApiError
     */
    public static unassignRole(
        userid: string,
        roleid: string,
    ): CancelablePromise<void> {
        return __request(OpenAPI, {
            method: 'POST',
            url: '/api/v1/users/{userid}/roles/{roleid}/unassign',
            path: {
                'userid': userid,
                'roleid': roleid,
            },
        });
    }

    /**
     * Returns all users from the system that the user has access to
     *
     * @param page page number
     * @param pageSize maximum number of results to return
     * @param sortBy field to sort by
     * @param order sort order
     * @param q starts with
     * @param detail basic or full (default to full)
     * @returns User user response
     * @throws ApiError
     */
    public static listUsers(
        page?: number,
        pageSize?: number,
        sortBy?: string,
        order?: 'asc' | 'desc',
        q?: string,
        detail?: string,
    ): CancelablePromise<Array<User>> {
        return __request(OpenAPI, {
            method: 'GET',
            url: '/api/v1/users',
            query: {
                'page': page,
                'pageSize': pageSize,
                'sortBy': sortBy,
                'order': order,
                'q': q,
                'detail': detail,
            },
        });
    }

    /**
     * Creates a new user in the store. Duplicates are not allowed
     * @param requestBody User to add to the store
     * @returns User user response
     * @throws ApiError
     */
    public static addUser(
        requestBody: NewUser,
    ): CancelablePromise<User> {
        return __request(OpenAPI, {
            method: 'POST',
            url: '/api/v1/users',
            body: requestBody,
            mediaType: 'application/json',
        });
    }

    /**
     * Returns a user based on a single ID, if the user does not have access to the user
     * @param userid ID of user to fetch
     * @param tenantid ID of tenant
     * @returns User user response
     * @throws ApiError
     */
    public static getUserByIdFromSuperAdmin(
        userid: string,
        tenantid: string,
    ): CancelablePromise<User> {
        return __request(OpenAPI, {
            method: 'GET',
            url: '/superadmin-api/v1/tenants/{tenantid}/users/{userid}',
            path: {
                'userid': userid,
                'tenantid': tenantid,
            },
        });
    }

    /**
     * Updates a new user in the store. Duplicates are allowed
     * @param userid ID of user to fetch
     * @param tenantid ID of tenant
     * @param requestBody User to update to the store
     * @returns void
     * @throws ApiError
     */
    public static updateUserFromSuperAdmin(
        userid: string,
        tenantid: string,
        requestBody: User,
    ): CancelablePromise<void> {
        return __request(OpenAPI, {
            method: 'PUT',
            url: '/superadmin-api/v1/tenants/{tenantid}/users/{userid}',
            path: {
                'userid': userid,
                'tenantid': tenantid,
            },
            body: requestBody,
            mediaType: 'application/json',
        });
    }

    /**
     * deletes a single user based on the ID supplied
     * @param userid ID of user to fetch
     * @param tenantid ID of tenant
     * @returns void
     * @throws ApiError
     */
    public static deleteUserFromSuperAdmin(
        userid: string,
        tenantid: string,
    ): CancelablePromise<void> {
        return __request(OpenAPI, {
            method: 'DELETE',
            url: '/superadmin-api/v1/tenants/{tenantid}/users/{userid}',
            path: {
                'userid': userid,
                'tenantid': tenantid,
            },
        });
    }

    /**
     * Act on user
     * @param userid ID of user to fetch
     * @param tenantid ID of tenant
     * @param requestBody Action to add to the store
     * @returns void
     * @throws ApiError
     */
    public static updateUserStatusFromSuperAdmin(
        userid: string,
        tenantid: string,
        requestBody: UserActionSchema,
    ): CancelablePromise<void> {
        return __request(OpenAPI, {
            method: 'POST',
            url: '/superadmin-api/v1/tenants/{tenantid}/users/{userid}/status',
            path: {
                'userid': userid,
                'tenantid': tenantid,
            },
            body: requestBody,
            mediaType: 'application/json',
        });
    }

    /**
     * Assign a role to a user based
     * @param userid ID of user to fetch
     * @param tenantid ID of tenant
     * @param roleid ID of role to assign
     * @returns void
     * @throws ApiError
     */
    public static assignRoleFromSuperAdmin(
        userid: string,
        tenantid: string,
        roleid: string,
    ): CancelablePromise<void> {
        return __request(OpenAPI, {
            method: 'POST',
            url: '/superadmin-api/v1/tenants/{tenantid}/users/{userid}/roles/{roleid}/assign',
            path: {
                'userid': userid,
                'tenantid': tenantid,
                'roleid': roleid,
            },
        });
    }

    /**
     * Unassign a role to a user based
     * @param userid ID of user to fetch
     * @param tenantid ID of tenant
     * @param roleid ID of role to unassign
     * @returns void
     * @throws ApiError
     */
    public static unassignRoleFromSuperAdmin(
        userid: string,
        tenantid: string,
        roleid: string,
    ): CancelablePromise<void> {
        return __request(OpenAPI, {
            method: 'POST',
            url: '/superadmin-api/v1/tenants/{tenantid}/users/{userid}/roles/{roleid}/unassign',
            path: {
                'userid': userid,
                'tenantid': tenantid,
                'roleid': roleid,
            },
        });
    }

    /**
     * Returns all users from the system that the user has access to
     *
     * @param tenantid ID of tenant
     * @param page page number
     * @param pageSize maximum number of results to return
     * @param sortBy field to sort by
     * @param order sort order
     * @param q starts with
     * @returns User user response
     * @throws ApiError
     */
    public static listUsersFromSuperAdmin(
        tenantid: string,
        page?: number,
        pageSize?: number,
        sortBy?: string,
        order?: 'asc' | 'desc',
        q?: string,
    ): CancelablePromise<Array<User>> {
        return __request(OpenAPI, {
            method: 'GET',
            url: '/superadmin-api/v1/tenants/{tenantid}/users',
            path: {
                'tenantid': tenantid,
            },
            query: {
                'page': page,
                'pageSize': pageSize,
                'sortBy': sortBy,
                'order': order,
                'q': q,
            },
        });
    }

    /**
     * Creates a new user in the store. Duplicates are not allowed
     * @param tenantid ID of tenant
     * @param requestBody User to add to the store
     * @returns User user response
     * @throws ApiError
     */
    public static addUserFromSuperAdmin(
        tenantid: string,
        requestBody: NewUser,
    ): CancelablePromise<User> {
        return __request(OpenAPI, {
            method: 'POST',
            url: '/superadmin-api/v1/tenants/{tenantid}/users',
            path: {
                'tenantid': tenantid,
            },
            body: requestBody,
            mediaType: 'application/json',
        });
    }

    /**
     * Creates a new empty user in the store. Duplicates are not allowed
     * @returns User user response
     * @throws ApiError
     */
    public static createMeUser(): CancelablePromise<User> {
        return __request(OpenAPI, {
            method: 'POST',
            url: '/api/v1/me',
        });
    }

    /**
     * Returns current user profile.
     *
     * @returns UserProfileSchema user response
     * @throws ApiError
     */
    public static getMeProfile(): CancelablePromise<UserProfileSchema> {
        return __request(OpenAPI, {
            method: 'GET',
            url: '/api/v1/me/profile',
        });
    }

    /**
     * Updates a user in the store.
     * @param requestBody User to add to the store
     * @returns UserProfileSchema user response
     * @throws ApiError
     */
    public static updateMeProfile(
        requestBody: UserProfileSchema,
    ): CancelablePromise<UserProfileSchema> {
        return __request(OpenAPI, {
            method: 'PUT',
            url: '/api/v1/me/profile',
            body: requestBody,
            mediaType: 'application/json',
        });
    }

    /**
     * Upload user profile picture
     * @param formData User to add to the store
     * @returns User user response
     * @throws ApiError
     */
    public static uploadProfilePicture(
        formData: {
            fileName?: Blob;
        },
    ): CancelablePromise<User> {
        return __request(OpenAPI, {
            method: 'POST',
            url: '/api/v1/me/profile/picture',
            formData: formData,
            mediaType: 'multipart/form-data',
        });
    }

    /**
     * Request a password reset
     * @param userid ID of user to fetch
     * @param requestBody email
     * @returns any level response
     * @throws ApiError
     */
    public static resetPasswordRequestByAdmin(
        userid: string,
        requestBody: {
            /**
             * email
             */
            email: string;
        },
    ): CancelablePromise<any> {
        return __request(OpenAPI, {
            method: 'POST',
            url: '/api/v1/users/{userid}/password-reset-request',
            path: {
                'userid': userid,
            },
            body: requestBody,
            mediaType: 'application/json',
        });
    }

    /**
     * Request a password reset
     * @param userid ID of user to fetch
     * @param tenantid ID of tenant
     * @param requestBody email
     * @returns any level response
     * @throws ApiError
     */
    public static resetPasswordRequestBySuperAdmin(
        userid: string,
        tenantid: string,
        requestBody: {
            /**
             * email
             */
            email: string;
        },
    ): CancelablePromise<any> {
        return __request(OpenAPI, {
            method: 'POST',
            url: '/superadmin-api/v1/tenants/{tenantid}/users/{userid}/password-reset-request',
            path: {
                'userid': userid,
                'tenantid': tenantid,
            },
            body: requestBody,
            mediaType: 'application/json',
        });
    }

    /**
     * Request a password reset
     * @param requestBody email
     * @returns any level response
     * @throws ApiError
     */
    public static resetPasswordRequest(
        requestBody: {
            /**
             * email
             */
            email: string;
        },
    ): CancelablePromise<any> {
        return __request(OpenAPI, {
            method: 'POST',
            url: '/public-api/v1/password-reset-request',
            body: requestBody,
            mediaType: 'application/json',
        });
    }

    /**
     * Returns a user based on a single ID, if the user does not have access to the user
     * @param userid ID of user to fetch
     * @returns any image file response
     * @throws ApiError
     */
    public static getProfilePicture(
        userid: string,
    ): CancelablePromise<any> {
        return __request(OpenAPI, {
            method: 'GET',
            url: '/public-api/v1/users/{userid}/profile/picture',
            path: {
                'userid': userid,
            },
        });
    }

    /**
     * Returns current tenant.
     *
     * @returns PublicTenantSchema tenant response
     * @throws ApiError
     */
    public static getPublicTenant(): CancelablePromise<PublicTenantSchema> {
        return __request(OpenAPI, {
            method: 'GET',
            url: '/public-api/v1/tenant',
        });
    }

    /**
     * Returns current tenant profile.
     *
     * @returns TenantProfileSchema tenant response
     * @throws ApiError
     */
    public static getTenantProfile(): CancelablePromise<TenantProfileSchema> {
        return __request(OpenAPI, {
            method: 'GET',
            url: '/api/v1/tenant/profile',
        });
    }

    /**
     * Updates a tenant profile in the store.
     * @param requestBody Tenant profile to update to the store
     * @returns TenantProfileSchema tenant response
     * @throws ApiError
     */
    public static updateTenantProfile(
        requestBody: TenantProfileSchema,
    ): CancelablePromise<TenantProfileSchema> {
        return __request(OpenAPI, {
            method: 'PUT',
            url: '/api/v1/tenant/profile',
            body: requestBody,
            mediaType: 'application/json',
        });
    }

    /**
     * Returns all roles from the system that the user has access to
     *
     * @param page page number
     * @param pageSize maximum number of results to return
     * @param sortBy field to sort by
     * @param order sort order
     * @param q starts with
     * @returns Role role response
     * @throws ApiError
     */
    public static listRoles(
        page?: number,
        pageSize?: number,
        sortBy?: string,
        order?: 'asc' | 'desc',
        q?: string,
    ): CancelablePromise<Array<Role>> {
        return __request(OpenAPI, {
            method: 'GET',
            url: '/api/v1/roles',
            query: {
                'page': page,
                'pageSize': pageSize,
                'sortBy': sortBy,
                'order': order,
                'q': q,
            },
        });
    }

    /**
     * Creates a new role in the store. Duplicates are allowed
     * @param requestBody Role to add to the store
     * @returns Role role response
     * @throws ApiError
     */
    public static addRole(
        requestBody: NewRole,
    ): CancelablePromise<Role> {
        return __request(OpenAPI, {
            method: 'POST',
            url: '/api/v1/roles',
            body: requestBody,
            mediaType: 'application/json',
        });
    }

    /**
     * Returns a user based on a single ID, if the user does not have access to the role
     * @param id ID of role to fetch
     * @returns Role role response
     * @throws ApiError
     */
    public static getRoleById(
        id: string,
    ): CancelablePromise<Role> {
        return __request(OpenAPI, {
            method: 'GET',
            url: '/api/v1/roles/{id}',
            path: {
                'id': id,
            },
        });
    }

    /**
     * Updates a new role in the store. Duplicates are allowed
     * @param id ID of role to fetch
     * @param requestBody Role to update to the store
     * @returns void
     * @throws ApiError
     */
    public static updateRole(
        id: string,
        requestBody: Role,
    ): CancelablePromise<void> {
        return __request(OpenAPI, {
            method: 'PUT',
            url: '/api/v1/roles/{id}',
            path: {
                'id': id,
            },
            body: requestBody,
            mediaType: 'application/json',
        });
    }

    /**
     * deletes a single role based on the ID supplied
     * @param id ID of role to delete
     * @returns void
     * @throws ApiError
     */
    public static deleteRole(
        id: string,
    ): CancelablePromise<void> {
        return __request(OpenAPI, {
            method: 'DELETE',
            url: '/api/v1/roles/{id}',
            path: {
                'id': id,
            },
        });
    }

    /**
     * Returns all tenants from the system that the user has access to
     *
     * @param page page number
     * @param pageSize maximum number of results to return
     * @param sortBy field to sort by
     * @param order sort order
     * @param q starts with
     * @returns Tenant tenant response
     * @throws ApiError
     */
    public static listTenants(
        page?: number,
        pageSize?: number,
        sortBy?: string,
        order?: 'asc' | 'desc',
        q?: string,
    ): CancelablePromise<Array<Tenant>> {
        return __request(OpenAPI, {
            method: 'GET',
            url: '/superadmin-api/v1/tenants',
            query: {
                'page': page,
                'pageSize': pageSize,
                'sortBy': sortBy,
                'order': order,
                'q': q,
            },
        });
    }

    /**
     * Creates a new tenant in the store. Duplicates are allowed
     * @param requestBody Tenant to add to the store
     * @returns Tenant tenant response
     * @throws ApiError
     */
    public static addTenant(
        requestBody: NewTenant,
    ): CancelablePromise<Tenant> {
        return __request(OpenAPI, {
            method: 'POST',
            url: '/superadmin-api/v1/tenants',
            body: requestBody,
            mediaType: 'application/json',
        });
    }

    /**
     * Returns a tenant based on a single ID, if the user does not have access to the tenant
     * @param tenantid ID of tenant to fetch
     * @returns Tenant tenant response
     * @throws ApiError
     */
    public static getTenantById(
        tenantid: string,
    ): CancelablePromise<Tenant> {
        return __request(OpenAPI, {
            method: 'GET',
            url: '/superadmin-api/v1/tenants/{tenantid}',
            path: {
                'tenantid': tenantid,
            },
        });
    }

    /**
     * Updates a tenant in the store.
     * @param tenantid ID of tenant to fetch
     * @param requestBody Tenant to add to the store
     * @returns Tenant tenant response
     * @throws ApiError
     */
    public static updateTenant(
        tenantid: string,
        requestBody: Tenant,
    ): CancelablePromise<Tenant> {
        return __request(OpenAPI, {
            method: 'PUT',
            url: '/superadmin-api/v1/tenants/{tenantid}',
            path: {
                'tenantid': tenantid,
            },
            body: requestBody,
            mediaType: 'application/json',
        });
    }

    /**
     * deletes a single tenant based on the ID supplied
     * @param tenantid ID of tenant to delete
     * @returns void
     * @throws ApiError
     */
    public static deleteTenant(
        tenantid: string,
    ): CancelablePromise<void> {
        return __request(OpenAPI, {
            method: 'DELETE',
            url: '/superadmin-api/v1/tenants/{tenantid}',
            path: {
                'tenantid': tenantid,
            },
        });
    }

    /**
     * Returns tenant features.
     * @param tenantid ID of tenant to fetch
     * @returns TenantFeatures tenant response
     * @throws ApiError
     */
    public static getTenantFeatures(
        tenantid: string,
    ): CancelablePromise<TenantFeatures> {
        return __request(OpenAPI, {
            method: 'GET',
            url: '/superadmin-api/v1/tenant/{tenantid}/features',
            path: {
                'tenantid': tenantid,
            },
        });
    }

    /**
     * Updates a tenant in the store.
     * @param tenantid ID of tenant to fetch
     * @param requestBody Tenant to add to the store
     * @returns TenantFeatures tenant response
     * @throws ApiError
     */
    public static updateTenantFeatures(
        tenantid: string,
        requestBody: TenantFeatures,
    ): CancelablePromise<TenantFeatures> {
        return __request(OpenAPI, {
            method: 'PUT',
            url: '/superadmin-api/v1/tenant/{tenantid}/features',
            path: {
                'tenantid': tenantid,
            },
            body: requestBody,
            mediaType: 'application/json',
        });
    }

    /**
     * Returns all client applications the user has access to
     * @param page page number
     * @param pageSize maximum number of results to return
     * @param sortBy field to sort by
     * @param order sort order
     * @param q search query
     * @param includeInactive include inactive applications
     * @returns ClientApplication client applications response
     * @throws ApiError
     */
    public static listClientApplications(
        page?: number,
        pageSize?: number,
        sortBy?: string,
        order?: 'asc' | 'desc',
        q?: string,
        includeInactive?: boolean,
    ): CancelablePromise<Array<ClientApplication>> {
        return __request(OpenAPI, {
            method: 'GET',
            url: '/superadmin-api/v1/client-applications',
            query: {
                'page': page,
                'pageSize': pageSize,
                'sortBy': sortBy,
                'order': order,
                'q': q,
                'includeInactive': includeInactive,
            },
        });
    }

    /**
     * Creates a new client application
     * @param requestBody Client application to create
     * @returns ClientApplication client application created
     * @throws ApiError
     */
    public static createClientApplication(
        requestBody: NewClientApplication,
    ): CancelablePromise<ClientApplication> {
        return __request(OpenAPI, {
            method: 'POST',
            url: '/superadmin-api/v1/client-applications',
            body: requestBody,
            mediaType: 'application/json',
        });
    }

    /**
     * Returns a client application by ID
     * @param id ID of client application to fetch
     * @returns ClientApplication client application response
     * @throws ApiError
     */
    public static getClientApplicationById(
        id: string,
    ): CancelablePromise<ClientApplication> {
        return __request(OpenAPI, {
            method: 'GET',
            url: '/superadmin-api/v1/client-applications/{id}',
            path: {
                'id': id,
            },
        });
    }

    /**
     * Updates a client application
     * @param id ID of client application to update
     * @param requestBody Client application to update
     * @returns ClientApplication client application updated
     * @throws ApiError
     */
    public static updateClientApplication(
        id: string,
        requestBody: NewClientApplication,
    ): CancelablePromise<ClientApplication> {
        return __request(OpenAPI, {
            method: 'PUT',
            url: '/superadmin-api/v1/client-applications/{id}',
            path: {
                'id': id,
            },
            body: requestBody,
            mediaType: 'application/json',
        });
    }

    /**
     * Deletes a client application
     * @param id ID of client application to delete
     * @returns void
     * @throws ApiError
     */
    public static deleteClientApplication(
        id: string,
    ): CancelablePromise<void> {
        return __request(OpenAPI, {
            method: 'DELETE',
            url: '/superadmin-api/v1/client-applications/{id}',
            path: {
                'id': id,
            },
        });
    }

    /**
     * Deactivates a client application
     * @param id ID of client application to deactivate
     * @returns void
     * @throws ApiError
     */
    public static deactivateClientApplication(
        id: string,
    ): CancelablePromise<void> {
        return __request(OpenAPI, {
            method: 'PATCH',
            url: '/superadmin-api/v1/client-applications/{id}/deactivate',
            path: {
                'id': id,
            },
        });
    }

    /**
     * Returns all API tokens for a client application
     * @param id ID of client application
     * @param page page number
     * @param pageSize maximum number of results to return
     * @param sortBy field to sort by
     * @param order sort order
     * @param includeRevoked include revoked tokens
     * @param includeExpired include expired tokens
     * @returns APIToken API tokens response
     * @throws ApiError
     */
    public static listApiTokens(
        id: string,
        page?: number,
        pageSize?: number,
        sortBy?: string,
        order?: 'asc' | 'desc',
        includeRevoked?: boolean,
        includeExpired?: boolean,
    ): CancelablePromise<Array<APIToken>> {
        return __request(OpenAPI, {
            method: 'GET',
            url: '/superadmin-api/v1/client-applications/{id}/tokens',
            path: {
                'id': id,
            },
            query: {
                'page': page,
                'pageSize': pageSize,
                'sortBy': sortBy,
                'order': order,
                'includeRevoked': includeRevoked,
                'includeExpired': includeExpired,
            },
        });
    }

    /**
     * Creates a new API token for a client application
     * @param id ID of client application
     * @param requestBody API token to create
     * @returns APITokenCreated API token created
     * @throws ApiError
     */
    public static createApiToken(
        id: string,
        requestBody: NewAPIToken,
    ): CancelablePromise<APITokenCreated> {
        return __request(OpenAPI, {
            method: 'POST',
            url: '/superadmin-api/v1/client-applications/{id}/tokens',
            path: {
                'id': id,
            },
            body: requestBody,
            mediaType: 'application/json',
        });
    }

    /**
     * Returns an API token by ID
     * @param id ID of client application
     * @param tokenId ID of API token
     * @returns APIToken API token response
     * @throws ApiError
     */
    public static getApiTokenById(
        id: string,
        tokenId: string,
    ): CancelablePromise<APIToken> {
        return __request(OpenAPI, {
            method: 'GET',
            url: '/superadmin-api/v1/client-applications/{id}/tokens/{tokenId}',
            path: {
                'id': id,
                'tokenId': tokenId,
            },
        });
    }

    /**
     * Deletes an API token
     * @param id ID of client application
     * @param tokenId ID of API token
     * @returns void
     * @throws ApiError
     */
    public static deleteApiToken(
        id: string,
        tokenId: string,
    ): CancelablePromise<void> {
        return __request(OpenAPI, {
            method: 'DELETE',
            url: '/superadmin-api/v1/client-applications/{id}/tokens/{tokenId}',
            path: {
                'id': id,
                'tokenId': tokenId,
            },
        });
    }

    /**
     * Revokes an API token
     * @param id ID of client application
     * @param tokenId ID of API token
     * @param requestBody Revocation details
     * @returns APIToken API token revoked
     * @throws ApiError
     */
    public static revokeApiToken(
        id: string,
        tokenId: string,
        requestBody?: APITokenRevoke,
    ): CancelablePromise<APIToken> {
        return __request(OpenAPI, {
            method: 'PATCH',
            url: '/superadmin-api/v1/client-applications/{id}/tokens/{tokenId}/revoke',
            path: {
                'id': id,
                'tokenId': tokenId,
            },
            body: requestBody,
            mediaType: 'application/json',
        });
    }

    /**
     * Returns audit logs for an API token
     * @param id ID of client application
     * @param tokenId ID of API token
     * @param page page number
     * @param pageSize maximum number of results to return
     * @returns APITokenAuditLog API token audit logs response
     * @throws ApiError
     */
    public static getApiTokenAuditLogs(
        id: string,
        tokenId: string,
        page?: number,
        pageSize?: number,
    ): CancelablePromise<Array<APITokenAuditLog>> {
        return __request(OpenAPI, {
            method: 'GET',
            url: '/superadmin-api/v1/client-applications/{id}/tokens/{tokenId}/audit',
            path: {
                'id': id,
                'tokenId': tokenId,
            },
            query: {
                'page': page,
                'pageSize': pageSize,
            },
        });
    }

}
