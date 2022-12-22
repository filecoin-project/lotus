package conf

type Config struct {
	ID  string
	Url string

	Net   string `json:"net"`
	Proto string `json:"proto"`
	Token string `json:"token"`
	Redis *Redis `json:"redis"`
}

type Redis struct {
	Addr string `json:"addr"`
	Pwd  string `json:"pwd"`
}
