package api

import (
	"net/http"

	"haovpn/internal/readmodel"
	"haovpn/internal/timeutil"
)

// handleLANRegistry GET /api/v1/lan-registry 只读列表。
func (s *Server) handleLANRegistry(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodGet) {
		return
	}
	viaFilter := parseQueryInt64(r, "user_id")
	rows, err := s.store.ListClientLANRegistry(viaFilter)
	if err != nil {
		writeInternalError(w, err)
		return
	}
	byID, err := s.userDirMap()
	if err != nil {
		writeInternalError(w, err)
		return
	}
	items := make([]readmodel.LANRegistryView, 0, len(rows))
	for _, row := range rows {
		v := readmodel.LANRegistryView{
			UserID: row.UserID, DestCIDR: row.DestCIDR, VPNIP: row.VPNIP,
			HostID: row.HostID, UpdatedAt: timeutil.FormatRFC3339(row.UpdatedAt),
		}
		if u, ok := byID[row.UserID]; ok {
			v.Username = u.Username
		}
		items = append(items, v)
	}
	writeItems(w, items)
}
