package user

type LoginReq struct {
	UserName string `json:"user_name" binding:"required"`
	Password string `json:"password"  binding:"required"`
}

type RegisterReq struct {
	UserName string `json:"user_name" binding:"required,min=3,max=32"`
	Password string `json:"password"  binding:"required,min=8"`
	NickName string `json:"nick_name"      binding:"required"`
	Email    string `json:"email"     binding:"required,email"`
	Phone    string `json:"phone"`
}

// ── 响应 ──────────────────────────────────────

type LoginResp struct {
	Token    string   `json:"token"`
	UserID   string   `json:"user_id"`
	UserName string   `json:"user_name"`
	NickName string   `json:"nick_name"`
	GroupID  int      `json:"group_id"`
	Powers   []string `json:"powers"`
	Space    int64    `json:"space"`
}

type UserResp struct {
	ID        string `json:"id"`
	NickName  string `json:"nick_name"`
	UserName  string `json:"user_name"`
	Email     string `json:"email"`
	Phone     string `json:"phone"`
	GroupID   int    `json:"group_id"`
	Space     int64  `json:"space"`
	FreeSpace int64  `json:"free_space"`
	State     int    `json:"state"`
}
