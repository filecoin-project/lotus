package conf

type Config struct {
	ID    string `json:"id"` //miner-id or worker-id
	Redis *Redis `json:"redis"`
}

type Redis struct {
	Addr string `json:"addr"`
	Pwd  string `json:"pwd"`
}
