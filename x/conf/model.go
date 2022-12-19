package conf

type Config struct {
	Redis *Redis `json:"redis"`
}

type Redis struct {
	Addr string `json:"addr"`
	Pwd  string `json:"pwd"`
}
