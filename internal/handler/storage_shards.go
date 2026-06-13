package handler

import "net/http"

func (s *Service) handleStorageShardRoute(w http.ResponseWriter, parts []string) bool {
	if !isStorageShardRoute(parts) {
		return false
	}
	s.writeLocalDataPlaneRemoved(w)
	return true
}

func isStorageShardRoute(parts []string) bool {
	return len(parts) == 6 && parts[0] == "_internal" && parts[1] == "storage" && parts[2] == "shards"
}
