package lib

// Response 基础数据
type CommonResponse struct {
	Errcode int    `json:"errcode,omitempty"`
	Errmsg  string `json:"errmsg,omitempty"`
}

type AccessToken struct {
	CommonResponse
	AccessToken string `json:"access_token"`
	ExpiresIn   int    `json:"expires_in"`
}
