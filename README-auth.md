# Authentication Middleware

## Overview

The `AuthMiddleware` provides a robust authentication system for the Core Application, combining API token and Firebase authentication methods.

## Authentication Methods

### 1. API Token Authentication

Used primarily for service-to-service communication and automated processes.

- **Header**: `X-Api-Key`
- **Format**: UUID-based token
- **Storage**: Tokens are hashed in the database (SHA-256)
- **Verification**: Performed by `ClientApplicationService.VerifyAPIToken()`

### 2. Firebase Authentication

Used primarily for user authentication in web and mobile applications.

- **Header**: `Authorization: Bearer <token>`
- **Format**: Firebase JWT token
- **Verification**: Performed by `FirebaseAuthMiddleware.verifyToken()`

## Path-Based Rules

The middleware applies different authentication rules based on the request path:

| Path Pattern        | Authentication Method      | Required Role                                           |
| ------------------- | -------------------------- | ------------------------------------------------------- |
| `/public/*`         | None (Public)              | None                                                    |
| `/api/v1/users/*`   | Firebase Auth only         | ADMIN, CUSTOMER_ADMIN, SUPER_ADMIN for write operations |
| `/admin-api/*`      | Firebase Auth only         | ADMIN, SUPER_ADMIN for all operations                   |
| `/superadmin-api/*` | Firebase Auth only         | SUPER_ADMIN for all operations                          |
| All other paths     | API Token OR Firebase Auth | None                                                    |

## Response Status Codes

- **401 Unauthorized**: No authentication credentials provided
- **403 Forbidden**: Invalid credentials or insufficient permissions

## Context Values

When authentication succeeds, the middleware sets the following values in the Gin context:

### API Token Authentication

- `api_token`: The complete token record
- `api_token_scopes`: Array of permission scopes
- `auth_user_id`: The user ID who created the token

### Firebase Authentication

- `auth_email`: User's email address
- `auth_user_id`: Firebase user ID
- `auth_claims`: All Firebase claims (including custom roles)

## Usage Example

```go
// Create the combined auth middleware
authMiddleware := access.NewAuthMiddleware(
    firebaseAuthMiddleWare,
    clientAppService,
)

// Apply middleware to routes
router.Use(authMiddleware.MiddlewareFunc())
```

## Security Considerations

1. **Token Storage**: API tokens are never stored in plain text
2. **Token Rotation**: Implement regular token rotation for security
3. **Scoped Access**: API tokens can be limited to specific operations
4. **Role-Based Access**: Firebase claims enforce role-based permissions
5. **Tenant Isolation**: Combined with TenantMiddleware for multi-tenant security

## Implementation Details

The middleware is implemented in `pkg/shared/service/auth_middleware.go` and follows these steps:

1. Check if the path is public (`/public/*`)
2. For non-user and non-admin paths:
   - Check for API token in X-Api-Key header
   - If valid, set context values and continue
   - If invalid, return 403 Forbidden
   - If not present, try Firebase authentication
3. For user and admin paths:
   - Require Firebase authentication
   - Check for required roles based on path and method
   - Return 403 if role requirements not met
