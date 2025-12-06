package models

type LoginRequest struct {
	Email      string `json:"email"`
	Password   string `json:"password"`
	RememberMe bool   `json:"rememberMe"`
}

type LoginResponse struct {
	Token string `json:"token"`
	User  User   `json:"user"`
}
type OAuthCallbackRequest struct {
	Code     string `json:"code"`
	State    string `json:"state"`
	Provider string `json:"provider"`
}
