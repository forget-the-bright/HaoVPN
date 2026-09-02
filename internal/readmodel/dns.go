package readmodel

// DNSServerView 托管 DNS 列表行（WebUI / API）。
//
// ReadonlyIP：config（YAML seed）禁改 IP/删除；CanEditExcludes：含「全部」成员时可配排除名单。
// 与托管路由共用「应用生效」；备注变更不经 pending_apply。
type DNSServerView struct {
	ID              int64   `json:"id"`
	DNSIP           string  `json:"dns_ip"`
	Remark          string  `json:"remark"`
	Source          string  `json:"source"` // config | manual
	Scope           string  `json:"scope"`  // all | user
	MemberUserIDs   []int64 `json:"member_user_ids"`
	MemberNames     string  `json:"member_names,omitempty"`
	ExcludeUserIDs  []int64 `json:"exclude_user_ids"`
	ExcludeNames    string  `json:"exclude_names,omitempty"`
	ReadonlyIP      bool    `json:"readonly_ip"`      // config 行禁改 IP/删
	CanEditExcludes bool    `json:"can_edit_excludes"` // 含 all 时可配排除
}
