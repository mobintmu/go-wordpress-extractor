package apikey

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// HeaderName is the request header that carries the API key.
const HeaderName = "X-API-Key"

// ContextKeyID is the gin context key holding the verified key id.
const ContextKeyID = "api_key_id"

// Middleware rejects requests without a valid API key. Missing, unknown,
// revoked, and expired keys all get the same 401 response.
func Middleware(s *Store, onReject func()) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, ok := s.Verify(c.GetHeader(HeaderName))
		if !ok {
			if onReject != nil {
				onReject()
			}
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"code":    "UNAUTHORIZED",
				"message": "Invalid API key.",
			})
			return
		}
		c.Set(ContextKeyID, id)
		c.Next()
	}
}
