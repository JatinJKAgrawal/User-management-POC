// Package httpx contains shared HTTP helpers: a consistent JSON envelope,
// error mapping, request binding and pagination.
package httpx

import (
	"errors"
	"log/slog"
	"net/http"
	"reflect"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/gin-gonic/gin/binding"
	"github.com/go-playground/validator/v10"
	"github.com/google/uuid"

	"github.com/jatinjkagrawal/user-management-poc/internal/apperror"
	"github.com/jatinjkagrawal/user-management-poc/internal/domain"
)

type ErrorBody struct {
	Code      string `json:"code"`
	Message   string `json:"message"`
	Details   any    `json:"details,omitempty"`
	RequestID string `json:"request_id,omitempty"`
}

type Meta struct {
	Total  int `json:"total"`
	Limit  int `json:"limit"`
	Offset int `json:"offset"`
}

func OK(c *gin.Context, status int, data any) {
	c.JSON(status, gin.H{"data": data})
}

func List(c *gin.Context, data any, total int, p domain.Page) {
	c.JSON(http.StatusOK, gin.H{"data": data, "meta": Meta{Total: total, Limit: p.Limit, Offset: p.Offset}})
}

func NoContent(c *gin.Context) { c.Status(http.StatusNoContent) }

// Error writes err as a JSON error response. Internal errors are logged with
// their cause and returned to the client as a generic message.
func Error(c *gin.Context, err error) {
	ae := apperror.As(err)
	status := ae.HTTPStatus()
	if status >= 500 {
		slog.Default().ErrorContext(c.Request.Context(), "request failed",
			"error", err, "path", c.FullPath(), "request_id", c.GetString(RequestIDKey))
	}
	c.AbortWithStatusJSON(status, gin.H{"error": ErrorBody{
		Code: ae.Code, Message: ae.Message, Details: ae.Details, RequestID: c.GetString(RequestIDKey),
	}})
}

const RequestIDKey = "request_id"

func init() {
	// Report validation errors using JSON field names instead of Go names.
	if v, ok := binding.Validator.Engine().(*validator.Validate); ok {
		v.RegisterTagNameFunc(func(f reflect.StructField) string {
			name, _, _ := strings.Cut(f.Tag.Get("json"), ",")
			if name == "-" {
				return ""
			}
			if name == "" {
				return f.Name
			}
			return name
		})
	}
}

// BindJSON decodes and validates the request body, returning a 422 with
// per-field messages on failure.
func BindJSON(c *gin.Context, dst any) bool {
	if err := c.ShouldBindJSON(dst); err != nil {
		var ve validator.ValidationErrors
		if errors.As(err, &ve) {
			fields := map[string]string{}
			for _, fe := range ve {
				fields[jsonField(fe)] = describe(fe)
			}
			Error(c, apperror.Validation("validation_failed", "request validation failed").WithDetails(fields))
			return false
		}
		Error(c, apperror.Validation("invalid_body", "request body is not valid JSON"))
		return false
	}
	return true
}

// jsonField returns the dotted JSON path of the field, without the root struct.
func jsonField(fe validator.FieldError) string {
	ns := fe.Namespace()
	if i := strings.Index(ns, "."); i >= 0 {
		ns = ns[i+1:]
	}
	return ns
}

func describe(fe validator.FieldError) string {
	switch fe.Tag() {
	case "required":
		return "is required"
	case "email":
		return "must be a valid email address"
	case "min":
		return "must be at least " + fe.Param()
	case "max":
		return "must be at most " + fe.Param()
	case "oneof":
		return "must be one of: " + fe.Param()
	case "uuid":
		return "must be a valid UUID"
	case "eqfield":
		return "must match " + strings.ToLower(fe.Param())
	default:
		return "is invalid (" + fe.Tag() + ")"
	}
}

// UUIDParam parses a path parameter as a UUID, writing a 404 if malformed
// (a malformed ID can never match a resource).
func UUIDParam(c *gin.Context, name string) (uuid.UUID, bool) {
	id, err := uuid.Parse(c.Param(name))
	if err != nil {
		Error(c, apperror.NotFound("not_found", "resource not found"))
		return uuid.Nil, false
	}
	return id, true
}

// Pagination reads ?limit=&offset= with sane bounds.
func Pagination(c *gin.Context) domain.Page {
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
	offset, _ := strconv.Atoi(c.DefaultQuery("offset", "0"))
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}
	if offset < 0 {
		offset = 0
	}
	return domain.Page{Limit: limit, Offset: offset}
}
