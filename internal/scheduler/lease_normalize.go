package scheduler

import (
	"fmt"
	"strings"
	"time"
)

func normalizeTaskType(taskType TaskType) (TaskType, error) {
	switch taskType {
	case "":
		return TaskTypeSingleton, nil
	case TaskTypeSingleton, TaskTypePartitioned, TaskTypeParallel:
		return taskType, nil
	default:
		return "", fmt.Errorf("%w: unsupported task type %q", ErrInvalidLease, taskType)
	}
}

func normalizeLeaseScope(taskType TaskType, scope string) (string, error) {
	scope = strings.TrimSpace(scope)
	switch taskType {
	case TaskTypeSingleton:
		if scope == "" {
			return LeaseScopeGlobal, nil
		}
	case TaskTypePartitioned:
		if scope == "" {
			return "", fmt.Errorf("%w: partitioned task scope is required", ErrInvalidLease)
		}
	case TaskTypeParallel:
		return scope, nil
	default:
		return "", fmt.Errorf("%w: unsupported task type %q", ErrInvalidLease, taskType)
	}
	return scope, nil
}

func normalizeLeaseDurations(ttl, heartbeatInterval time.Duration) (time.Duration, time.Duration) {
	if ttl <= 0 {
		ttl = defaultLeaseTTL
	}
	if heartbeatInterval <= 0 || heartbeatInterval >= ttl {
		heartbeatInterval = ttl / 2
	}
	if heartbeatInterval <= 0 {
		heartbeatInterval = ttl
	}
	return ttl, heartbeatInterval
}

func (runtime *Runtime) normalizeLeaseOwner(owner string) string {
	owner = strings.TrimSpace(owner)
	if owner == "" && runtime != nil {
		owner = strings.TrimSpace(runtime.leaseOwner)
	}
	if owner == "" {
		owner = defaultLeaseOwner()
	}
	return owner
}
