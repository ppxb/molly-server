package response

import "github.com/gin-gonic/gin"

const requestIDContextKey = "request_id"

type ErrorBody struct {
	Code      string `json:"code"`
	Message   string `json:"message"`
	RequestID string `json:"request_id,omitempty"`
}

func Error(c *gin.Context, statusCode int, code, message string) {
	requestID, _ := c.Get(requestIDContextKey)

	body := ErrorBody{
		Code:    code,
		Message: message,
	}
	if id, ok := requestID.(string); ok {
		body.RequestID = id
	}

	c.AbortWithStatusJSON(statusCode, body)
}

func JSON(c *gin.Context, statusCode int, payload any) {
	c.JSON(statusCode, payload)
}
