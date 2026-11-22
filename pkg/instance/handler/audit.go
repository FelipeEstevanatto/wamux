package instance_handler

import (
	instance_model "github.com/felipeestevanatto/wamux/pkg/instance/model"
	"github.com/gin-gonic/gin"
)

// auditActor identifies who performed an administrative action. Admin-key routes
// have no instance in context, so they are recorded as "admin"; routes scoped to
// an instance use its id, never its token.
func auditActor(ctx *gin.Context) string {
	if v, ok := ctx.Get("instance"); ok {
		if inst, ok := v.(*instance_model.Instance); ok && inst != nil && inst.Id != "" {
			return inst.Id
		}
	}
	return "admin"
}
