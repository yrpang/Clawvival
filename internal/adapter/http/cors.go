package httpadapter

import (
	"context"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol/consts"
)

const corsAllowMethods = "GET,POST,OPTIONS"
const corsAllowHeaders = "Content-Type,X-Agent-ID,X-Agent-Key"

func applyCORSHeaders(ctx *app.RequestContext) {
	ctx.Response.Header.Set("Access-Control-Allow-Origin", "*")
	ctx.Response.Header.Set("Access-Control-Allow-Methods", corsAllowMethods)
	ctx.Response.Header.Set("Access-Control-Allow-Headers", corsAllowHeaders)
	ctx.Response.Header.Set("Access-Control-Max-Age", "600")
}

func corsMiddleware() app.HandlerFunc {
	return func(c context.Context, ctx *app.RequestContext) {
		applyCORSHeaders(ctx)
		if string(ctx.Method()) == consts.MethodOptions {
			ctx.AbortWithStatus(consts.StatusNoContent)
			return
		}
		ctx.Next(c)
	}
}
