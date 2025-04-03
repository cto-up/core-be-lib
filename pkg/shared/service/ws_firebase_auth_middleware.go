package service

import (
	"github.com/gin-gonic/gin"
)

// FirebaseAuthMiddleware is middleware for Firebase Authentication
type WSFirebaseAuthMiddleware struct {
	firebaseAuthMiddleware *FirebaseAuthMiddleware
}

// New is constructor of the middleware
func NewWSAuthMiddleware(FirebaseAuthMiddleware *FirebaseAuthMiddleware) *WSFirebaseAuthMiddleware {
	return &WSFirebaseAuthMiddleware{
		firebaseAuthMiddleware: FirebaseAuthMiddleware,
	}
}

// MiddlewareFunc is function to verify token
func (fam *WSFirebaseAuthMiddleware) MiddlewareFunc() gin.HandlerFunc {
	return func(c *gin.Context) {
		fam.firebaseAuthMiddleware.verifyToken(c)
		c.Next()
	}
}
