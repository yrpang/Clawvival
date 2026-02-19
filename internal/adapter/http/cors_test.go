package httpadapter

import (
	"testing"

	"github.com/cloudwego/hertz/pkg/app"
)

func TestApplyCORSHeaders(t *testing.T) {
	ctx := &app.RequestContext{}
	applyCORSHeaders(ctx)

	if got, want := string(ctx.Response.Header.Peek("Access-Control-Allow-Origin")), "*"; got != want {
		t.Fatalf("allow-origin mismatch: got=%q want=%q", got, want)
	}
	if got, want := string(ctx.Response.Header.Peek("Access-Control-Allow-Methods")), corsAllowMethods; got != want {
		t.Fatalf("allow-methods mismatch: got=%q want=%q", got, want)
	}
	if got, want := string(ctx.Response.Header.Peek("Access-Control-Allow-Headers")), corsAllowHeaders; got != want {
		t.Fatalf("allow-headers mismatch: got=%q want=%q", got, want)
	}
}
