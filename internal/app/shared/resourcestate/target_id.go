package resourcestate

import (
	"fmt"
	"strings"
)

func ParseResourceTargetID(targetID string) (x int, y int, resource string, ok bool) {
	var prefix string
	if _, err := fmt.Sscanf(strings.TrimSpace(targetID), "%3s_%d_%d_%s", &prefix, &x, &y, &resource); err != nil {
		return 0, 0, "", false
	}
	if prefix != "res" {
		return 0, 0, "", false
	}
	resource = strings.ToLower(strings.TrimSpace(resource))
	if resource == "" {
		return 0, 0, "", false
	}
	return x, y, resource, true
}

func BuildResourceTargetID(x, y int, resource string) string {
	return fmt.Sprintf("res_%d_%d_%s", x, y, strings.ToLower(strings.TrimSpace(resource)))
}
